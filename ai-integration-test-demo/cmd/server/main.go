package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/example/ai-integration-test-demo/ai/agent"
	"github.com/example/ai-integration-test-demo/ai/codeanalyzer"
	"github.com/example/ai-integration-test-demo/ai/knowledge"
	"github.com/example/ai-integration-test-demo/ai/lsp"
	"github.com/example/ai-integration-test-demo/ai/prompt"
	"github.com/example/ai-integration-test-demo/ai/session"
	"github.com/example/ai-integration-test-demo/internal/breakpoint"
	"github.com/example/ai-integration-test-demo/internal/event"
	"github.com/example/ai-integration-test-demo/internal/player"
	gameserver "github.com/example/ai-integration-test-demo/internal/server"
	"github.com/gorilla/websocket"
)

func main() {
	host := flag.String("host", "127.0.0.1", "server host")
	port := flag.Int("port", 5400, "server port")
	mode := flag.String("mode", "server", "run mode: server or test")
	apiKey := flag.String("api-key", os.Getenv("API_KEY"), "API key")
	model := flag.String("model", os.Getenv("MODEL"), "model name")
	baseURL := flag.String("base-url", os.Getenv("BASE_URL"), "API base URL")
	scenario := flag.String("scenario", "autonomous-discovery", "test scenario: autonomous-discovery, code-only, log-only")
	projectDir := flag.String("project-dir", ".", "project root directory for code analysis")
	quickStart := flag.Bool("quick-start", false, "pre-inject code analysis into prompt")
	docFile := flag.String("doc-file", "", "requirements document file for Level 1+ Prompt")
	rulesFile := flag.String("rules-file", "", "expert rules file for Level 2 Prompt")
	flag.Parse()

	// When using Codex CLI provider, defaults are different
	if *apiKey == "codex" {
		if *model == "" {
			*model = "gpt-5.4"
		}
		// baseURL is not used for codex provider
	} else {
		if *model == "" {
			*model = "glm-5.1"
		}
		if *baseURL == "" {
			*baseURL = "https://open.bigmodel.cn/api/paas/v4"
		}
	}

	bus := event.NewBus()
	pm := player.NewManager(bus)
	pm.CreatePlayer(10001)

	bp := breakpoint.NewController(bus)
	commandProfile := "manual"
	if *mode == "test" {
		commandProfile = getAgentMode(*scenario)
	}
	srv := gameserver.New(pm, bus, bp, commandProfile)

	http.HandleFunc("/ws", srv.HandleWS)

	httpServer := &http.Server{Addr: fmt.Sprintf("%s:%d", *host, *port)}

	if *mode == "test" {
		agentMode := getAgentMode(*scenario)

		fm := knowledge.NewFileManager(*projectDir)
		if err := fm.InitKnowledge(); err != nil {
			log.Printf("warning: knowledge init failed: %v", err)
		}

		codeSummary := ""
		if *quickStart {
			log.Printf("quick-start: analyzing source code from %s ...", *projectDir)
			modules, err := codeanalyzer.Analyze(*projectDir)
			if err != nil {
				log.Printf("warning: code analysis failed: %v", err)
			} else {
				codeSummary = codeanalyzer.FormatSummary(modules)
				log.Printf("code analysis complete: %d modules analyzed", len(modules))
			}
		}

		docContent, err := readOptionalFile(*docFile)
		if err != nil {
			log.Fatalf("read doc-file error: %v", err)
		}
		rulesContent, err := readOptionalFile(*rulesFile)
		if err != nil {
			log.Fatalf("read rules-file error: %v", err)
		}

		var sess *session.Session

		if agentMode != "code-only" {
			go func() {
				if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Fatalf("server error: %v", err)
				}
			}()
			log.Printf("game server started on %s:%d", *host, *port)
			time.Sleep(500 * time.Millisecond)

			conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/ws", *port), nil)
			if err != nil {
				log.Fatalf("dial error: %v", err)
			}
			defer conn.Close()
			sess = session.New(conn)
		}

		// Start LSP server for code-access modes
		var lspClient *lsp.Client
		needsLSP := agentMode == "code-batch" || agentMode == "dual" || agentMode == "code-only" || agentMode == "l0" || agentMode == "l1"
		if needsLSP {
			absProjectDir, _ := filepath.Abs(*projectDir)
			log.Printf("starting gopls LSP server for %s ...", absProjectDir)
			lc, err := lsp.NewClient(context.Background(), absProjectDir)
			if err != nil {
				log.Printf("warning: LSP init failed (falling back to text tools): %v", err)
			} else {
				lspClient = lc
				defer lspClient.Close()
				log.Printf("gopls LSP server started successfully")
			}
		}

		ag := agent.New(*apiKey, *model, *baseURL, sess, agentMode, prompt.PromptOptions{
			CodeSummary:  codeSummary,
			DocContent:   docContent,
			RulesContent: rulesContent,
		}, fm, lspClient)

		taskDesc, _ := buildScenario(*scenario)
		log.Printf("running AI test scenario: %s (mode: %s)", *scenario, agentMode)

		result, err := ag.Run(context.Background(), taskDesc)
		if err != nil {
			log.Fatalf("agent error: %v", err)
		}

		fmt.Println("\n========== TEST REPORT ==========")
		fmt.Println(result)
		fmt.Println("=================================")

		if agentMode != "code-only" {
			httpServer.Close()
		}
		return
	}

	log.Printf("game server started on %s:%d (mode=server)", *host, *port)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
}

func getAgentMode(scenario string) string {
	switch scenario {
	case "l0":
		return "l0"
	case "l1":
		return "l1"
	case "code-only":
		return "code-only"
	case "batch-only":
		return "batch-only"
	case "step-only":
		return "step-only"
	case "code-batch":
		return "code-batch"
	case "dual", "autonomous-discovery":
		return "dual"
	default:
		return "dual"
	}
}

func buildScenario(name string) (string, string) {
	switch name {
	case "l0":
		return `Run DSMB-Agent in L0 mode on player 10001.

You do not have pre-built business commands. First understand the codebase, then register the commands you need, then execute them step by step.

Your goal is to discover correlations, construct your own validation interfaces, and test normal, abnormal, and boundary behavior without relying on human-authored test guidance.`, ""
	case "l1":
		return `Run DSMB-Agent in L1 mode on player 10001.

You have basic built-in business commands, but you may also register new commands whenever the existing interface is insufficient.

Your goal is to use built-in interfaces where possible, extend them when necessary, and test normal, abnormal, and boundary behavior while discovering cross-module relations and defects.`, ""
	case "autonomous-discovery":
		return `Run an autonomous correlation discovery test on player 10001.

You have a pre-built code analysis summary and runtime access. Your goal is to:

1. Review the code analysis to understand module structure and event Publish/Subscribe chains
2. Verify each inferred correlation by performing operations and observing runtime logs
3. Build a complete correlation map of all cross-module relationships
4. Test edge cases to discover bugs (e.g., negative counts, missing validation, duplicate claims)
5. Report both discovered correlations AND any bugs found`, ""
	case "code-only":
		return `Analyze the pre-built code analysis to infer cross-module correlations and potential bugs.

You have the code analysis but CANNOT run the system. Your goal is to:

1. Review the code analysis to understand module structure and event flow
2. Build a correlation map from the Publish/Subscribe chains
3. Identify potential bugs by analyzing the code structure (e.g., missing validation, hardcoded values)
4. Report your findings as unverified hypotheses`, ""
	case "log-only":
		return `Run an autonomous correlation discovery test on player 10001 using runtime observation ONLY.

You do NOT have access to source code analysis. Your goal is to DISCOVER cross-module relationships by operating the system and observing logs:

1. Query the initial state of all modules to understand what exists
2. Perform operations one at a time, using "next" after each to observe logs
3. When you see a log entry from a different module, note the correlation
4. Build a complete correlation map through systematic exploration
5. Test edge cases to discover bugs
6. Report both discovered correlations AND any bugs found`, ""
	default:
		return fmt.Sprintf(`Run integration tests on player 10001 using scenario: %s. Check all modules, perform operations, step through execution, and report findings.`, name), ""
	}
}

func readOptionalFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
