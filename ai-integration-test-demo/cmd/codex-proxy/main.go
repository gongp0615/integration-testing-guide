// codex-proxy: A local OpenAI-compatible proxy server that routes
// chat completion requests through Codex CLI's authentication.
//
// It reads the access token from ~/.codex/auth.json and forwards
// requests to the OpenAI API with proper Bearer authentication.
//
// The key insight: Codex CLI uses wss://chatgpt.com/backend-api/codex/responses
// which is not standard OpenAI API. However, if the user has API credits
// (even $0 free tier), the access_token CAN be used as a Bearer token
// for the standard /v1/chat/completions endpoint.
//
// For ChatGPT-only subscriptions (no API credits), this proxy provides
// a fallback mode that wraps codex exec as a single-turn completion engine.
//
// Usage:
//
//	go run ./cmd/codex-proxy -port 8090
//	# Then point DSMB-Agent at: --base-url http://127.0.0.1:8090/v1
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// codexAuth holds the parsed auth.json structure
type codexAuth struct {
	AuthMode string `json:"auth_mode"`
	APIKey   string `json:"OPENAI_API_KEY"`
	Tokens   struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
}

// proxy holds the proxy server state
type proxy struct {
	mu       sync.RWMutex
	auth     codexAuth
	authPath string
	port     int
}

func main() {
	port := 8090
	if len(os.Args) > 2 && os.Args[1] == "-port" {
		fmt.Sscanf(os.Args[2], "%d", &port)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("cannot find home directory:", err)
	}
	authPath := filepath.Join(homeDir, ".codex", "auth.json")

	p := &proxy{
		authPath: authPath,
		port:     port,
	}

	if err := p.loadAuth(); err != nil {
		log.Fatal("cannot load codex auth:", err)
	}

	http.HandleFunc("/v1/chat/completions", p.handleChatCompletions)
	http.HandleFunc("/v1/models", p.handleModels)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	log.Printf("codex-proxy listening on %s", addr)
	log.Printf("  Use: --base-url http://127.0.0.1:%d/v1 --api-key codex", port)
	log.Printf("  Auth mode: %s", p.auth.AuthMode)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func (p *proxy) loadAuth() error {
	data, err := os.ReadFile(p.authPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", p.authPath, err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return json.Unmarshal(data, &p.auth)
}

// handleChatCompletions proxies a chat completion request.
// It serializes the messages into a single prompt and uses codex exec.
func (p *proxy) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req chatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("proxy request: model=%s messages=%d tools=%d",
		req.Model, len(req.Messages), len(req.Tools))

	// Build prompt from messages and tools
	prompt := buildPromptFromMessages(req)

	// Call codex exec
	response, err := callCodexExec(r.Context(), prompt)
	if err != nil {
		log.Printf("codex exec error: %v", err)
		writeErrorResponse(w, "codex exec failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Parse codex response and check if it contains tool calls
	chatResp := buildChatResponse(req.Model, response)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatResp)
}

func (p *proxy) handleModels(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"object": "list",
		"data": []map[string]interface{}{
			{
				"id":       "gpt-5.4",
				"object":   "model",
				"owned_by": "openai",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ── Chat Completion types ─────────────────────────────────────

type chatCompletionRequest struct {
	Model    string          `json:"model"`
	Messages []message       `json:"messages"`
	Tools    []toolDef       `json:"tools,omitempty"`
	Stream   bool            `json:"stream,omitempty"`
}

type message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolDef struct {
	Type     string      `json:"type"`
	Function toolFuncDef `json:"function"`
}

type toolFuncDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type chatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   usage    `json:"usage"`
}

type choice struct {
	Index        int     `json:"index"`
	Message      message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ── Prompt building ───────────────────────────────────────────

func buildPromptFromMessages(req chatCompletionRequest) string {
	var sb strings.Builder

	// Instruction header for codex exec
	sb.WriteString("You are acting as a transparent LLM proxy. ")
	sb.WriteString("You must respond EXACTLY as the AI assistant would in the conversation below. ")
	sb.WriteString("Do NOT add any commentary, explanation, or meta-text. ")
	sb.WriteString("If the conversation expects a tool call, respond with a JSON block starting with ```tool_calls.\n\n")

	// Add tool definitions if present
	if len(req.Tools) > 0 {
		sb.WriteString("## Available Tools\n\n")
		sb.WriteString("You have the following tools available. To call a tool, respond with EXACTLY this format:\n")
		sb.WriteString("```tool_calls\n")
		sb.WriteString("[{\"id\": \"call_xxx\", \"type\": \"function\", \"function\": {\"name\": \"tool_name\", \"arguments\": \"{...}\"}}]\n")
		sb.WriteString("```\n\n")
		for _, t := range req.Tools {
			paramJSON, _ := json.Marshal(t.Function.Parameters)
			sb.WriteString(fmt.Sprintf("### %s\n%s\nParameters: %s\n\n",
				t.Function.Name, t.Function.Description, string(paramJSON)))
		}
	}

	// Add conversation
	sb.WriteString("## Conversation\n\n")
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			sb.WriteString(fmt.Sprintf("[SYSTEM]: %s\n\n", m.Content))
		case "user":
			sb.WriteString(fmt.Sprintf("[USER]: %s\n\n", m.Content))
		case "assistant":
			if len(m.ToolCalls) > 0 {
				tcJSON, _ := json.Marshal(m.ToolCalls)
				sb.WriteString(fmt.Sprintf("[ASSISTANT tool_calls]: %s\n\n", string(tcJSON)))
			} else {
				sb.WriteString(fmt.Sprintf("[ASSISTANT]: %s\n\n", m.Content))
			}
		case "tool":
			sb.WriteString(fmt.Sprintf("[TOOL RESULT %s]: %s\n\n", m.ToolCallID, m.Content))
		}
	}

	sb.WriteString("[ASSISTANT]: ")
	return sb.String()
}

// ── Codex exec wrapper ───────────────────────────────────────

func callCodexExec(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "codex", "exec", "--ephemeral", "--json", "-")
	cmd.Stdin = strings.NewReader(prompt)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("codex exec: %w (stderr: %s)", err, stderr.String())
	}

	// Parse JSONL output from codex exec
	var lastText string
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event["type"] == "item.completed" {
			if item, ok := event["item"].(map[string]interface{}); ok {
				if text, ok := item["text"].(string); ok {
					lastText = text
				}
			}
		}
		if event["type"] == "error" {
			if msg, ok := event["message"].(string); ok {
				return "", fmt.Errorf("codex error: %s", msg)
			}
		}
	}

	if lastText == "" {
		return "", fmt.Errorf("no response text from codex exec")
	}

	return lastText, nil
}

// ── Response building ─────────────────────────────────────────

func buildChatResponse(model, responseText string) chatCompletionResponse {
	msg := message{
		Role:    "assistant",
		Content: responseText,
	}
	finishReason := "stop"

	// Check if the response contains tool calls
	if strings.Contains(responseText, "```tool_calls") {
		toolCalls := extractToolCalls(responseText)
		if len(toolCalls) > 0 {
			msg.Content = ""
			msg.ToolCalls = toolCalls
			finishReason = "tool_calls"
		}
	}

	return chatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-codex-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []choice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
		Usage: usage{
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
	}
}

func extractToolCalls(text string) []toolCall {
	start := strings.Index(text, "```tool_calls\n")
	if start == -1 {
		start = strings.Index(text, "```tool_calls")
		if start == -1 {
			return nil
		}
		start += len("```tool_calls")
	} else {
		start += len("```tool_calls\n")
	}

	end := strings.Index(text[start:], "```")
	if end == -1 {
		end = len(text) - start
	}

	jsonStr := strings.TrimSpace(text[start : start+end])

	var calls []toolCall
	if err := json.Unmarshal([]byte(jsonStr), &calls); err != nil {
		log.Printf("failed to parse tool calls: %v (json: %s)", err, jsonStr)
		return nil
	}
	return calls
}

func writeErrorResponse(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": msg,
			"type":    "proxy_error",
		},
	})
}
