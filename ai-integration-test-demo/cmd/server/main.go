package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/example/ai-integration-test-demo/ai/agent"
	"github.com/example/ai-integration-test-demo/ai/session"
	"github.com/example/ai-integration-test-demo/internal/breakpoint"
	"github.com/example/ai-integration-test-demo/internal/event"
	"github.com/example/ai-integration-test-demo/internal/player"
	gameserver "github.com/example/ai-integration-test-demo/internal/server"
	"github.com/gorilla/websocket"
)

func main() {
	port := flag.Int("port", 5400, "server port")
	mode := flag.String("mode", "server", "run mode: server or test")
	apiKey := flag.String("api-key", os.Getenv("API_KEY"), "API key")
	model := flag.String("model", "glm-5.1", "model name")
	baseURL := flag.String("base-url", "https://open.bigmodel.cn/api/paas/v4", "API base URL")
	scenario := flag.String("scenario", "basic", "test scenario: basic, cross-module, edge-case")
	flag.Parse()

	bus := event.NewBus()
	pm := player.NewManager(bus)
	pm.CreatePlayer(10001)

	bp := breakpoint.NewController(bus)
	srv := gameserver.New(pm, bus, bp)

	http.HandleFunc("/ws", srv.HandleWS)

	httpServer := &http.Server{Addr: fmt.Sprintf(":%d", *port)}

	if *mode == "test" {
		go func() {
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("server error: %v", err)
			}
		}()
		log.Printf("game server started on :%d", *port)

		time.Sleep(500 * time.Millisecond)

		conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/ws", *port), nil)
		if err != nil {
			log.Fatalf("dial error: %v", err)
		}
		defer conn.Close()

		sess := session.New(conn)
		ag := agent.New(*apiKey, *model, *baseURL, sess)

		taskDesc := buildScenario(*scenario)
		log.Printf("running AI test scenario: %s", *scenario)

		result, err := ag.Run(context.Background(), taskDesc)
		if err != nil {
			log.Fatalf("agent error: %v", err)
		}

		fmt.Println("\n========== TEST REPORT ==========")
		fmt.Println(result)
		fmt.Println("=================================")

		httpServer.Close()
		return
	}

	log.Printf("game server started on :%d (mode=server)", *port)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
}

func buildScenario(name string) string {
	switch name {
	case "basic":
		return `Run a basic integration test on player 10001:
1. Check the player's bag, tasks, and achievements initial state
2. Add item 2001 x5, step through with "next" to see logs
3. Verify task 3001 progress changed and achievement 4001 is unlocked
4. Report findings`
	case "cross-module":
		return `Run a cross-module integration test on player 10001:
1. Check initial state of all modules
2. Add item 2001 x1, step through, verify task 3001 completion → achievement 4001 unlock
3. Add item 2002 x2, step through, verify task 3002 completion → achievement 4002 unlock
4. Check if achievement 4003 (collector_100) is unlocked (requires 2+ unlocked achievements)
5. Report findings with special attention to cross-module trigger chains`
	case "edge-case":
		return `Run edge-case integration tests on player 10001:
1. Test adding item with count=0 (should be rejected)
2. Test removing item that doesn't exist (should fail gracefully)
3. Test removing more items than available (should fail)
4. Test adding negative count (should be rejected)
5. After each operation, use "next" and check logs for proper error handling
6. Report any bugs or unexpected behaviors found`
	default:
		return fmt.Sprintf(`Run integration tests on player 10001 using scenario: %s. Check all modules, perform operations, step through execution, and report findings.`, name)
	}
}
