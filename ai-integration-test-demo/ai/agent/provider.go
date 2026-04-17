package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	oai "github.com/sashabaranov/go-openai"
)

// Provider abstracts the LLM backend so the agent loop is backend-agnostic.
type Provider interface {
	// ChatCompletion sends a chat request and returns the response.
	ChatCompletion(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error)
	// Name returns a human-readable provider name for logging.
	Name() string
}

// ── OpenAI Provider (existing behavior) ──────────────────────

// OpenAIProvider uses the go-openai client to call any OpenAI-compatible API.
type OpenAIProvider struct {
	client *oai.Client
}

func NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider {
	cfg := oai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	return &OpenAIProvider{client: oai.NewClientWithConfig(cfg)}
}

func (p *OpenAIProvider) ChatCompletion(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
	return p.client.CreateChatCompletion(ctx, req)
}

func (p *OpenAIProvider) Name() string { return "openai" }

// ── Codex CLI Provider ───────────────────────────────────────

// CodexProvider routes LLM calls through `codex exec --json`.
// It serializes the message history into a single prompt, invokes codex exec,
// and parses the text response back into the OpenAI response format.
//
// Tool calls are communicated via a ```tool_calls``` fenced block convention:
// the model outputs tool calls as JSON inside such a block, and this provider
// parses them back into oai.ToolCall structs.
type CodexProvider struct {
	model string
}

func NewCodexProvider(model string) *CodexProvider {
	if model == "" {
		model = "gpt-5.4"
	}
	return &CodexProvider{model: model}
}

func (p *CodexProvider) Name() string { return "codex" }

func (p *CodexProvider) ChatCompletion(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
	prompt := p.buildPrompt(req)

	text, err := p.callCodexExec(ctx, prompt)
	if err != nil {
		return oai.ChatCompletionResponse{}, fmt.Errorf("codex exec: %w", err)
	}

	return p.parseResponse(req.Model, text, req.Tools), nil
}

func (p *CodexProvider) buildPrompt(req oai.ChatCompletionRequest) string {
	var sb strings.Builder

	// Meta-instruction: force codex exec to act as a transparent LLM
	sb.WriteString(`CRITICAL INSTRUCTION: You are NOT acting as a coding agent. You are acting as a raw LLM completion engine. Do NOT read files, do NOT run commands, do NOT use any of your built-in tools. Simply read the conversation below and produce the next assistant response.

If you want to call one of the CUSTOM tools listed below, output EXACTLY this format and NOTHING else:
` + "```" + `tool_calls
[{"id":"call_1","type":"function","function":{"name":"TOOL_NAME","arguments":"{...}"}}]
` + "```" + `

If you want to produce a final text response (no tool call), just output the text directly.

`)

	// Tool definitions
	if len(req.Tools) > 0 {
		sb.WriteString("CUSTOM TOOLS AVAILABLE:\n\n")
		for _, t := range req.Tools {
			paramJSON, _ := json.Marshal(t.Function.Parameters)
			sb.WriteString(fmt.Sprintf("- %s: %s\n  Parameters: %s\n\n",
				t.Function.Name, t.Function.Description, string(paramJSON)))
		}
	}

	sb.WriteString("---\nCONVERSATION:\n\n")

	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			sb.WriteString(fmt.Sprintf("SYSTEM: %s\n\n", m.Content))
		case "user":
			sb.WriteString(fmt.Sprintf("USER: %s\n\n", m.Content))
		case "assistant":
			if len(m.ToolCalls) > 0 {
				tcJSON, _ := json.Marshal(m.ToolCalls)
				sb.WriteString(fmt.Sprintf("ASSISTANT [tool_calls]: %s\n\n", string(tcJSON)))
			} else if m.Content != "" {
				sb.WriteString(fmt.Sprintf("ASSISTANT: %s\n\n", m.Content))
			}
		case "tool":
			sb.WriteString(fmt.Sprintf("TOOL_RESULT (call_id=%s): %s\n\n", m.ToolCallID, m.Content))
		}
	}

	sb.WriteString("---\nNow produce the next ASSISTANT response. Remember: output tool_calls block OR plain text, nothing else.\n\nASSISTANT: ")
	return sb.String()
}

func (p *CodexProvider) callCodexExec(ctx context.Context, prompt string) (string, error) {
	execCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "codex", "exec",
		"--ephemeral",
		"--json",
		"--sandbox", "read-only",
		"-m", p.model,
		"-")
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w (stderr: %s)", err, stderr.String())
	}

	// Parse JSONL output
	var lastText string
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(scanner.Text()), &event); err != nil {
			continue
		}
		switch event["type"] {
		case "item.completed":
			if item, ok := event["item"].(map[string]interface{}); ok {
				if text, ok := item["text"].(string); ok {
					lastText = text
				}
			}
		case "error":
			if msg, ok := event["message"].(string); ok {
				return "", fmt.Errorf("codex error: %s", msg)
			}
		}
	}

	if lastText == "" {
		return "", fmt.Errorf("no response from codex exec (stdout: %s)", stdout.String())
	}

	return lastText, nil
}

func (p *CodexProvider) parseResponse(model, text string, requestTools []oai.Tool) oai.ChatCompletionResponse {
	msg := oai.ChatCompletionMessage{
		Role:    oai.ChatMessageRoleAssistant,
		Content: text,
	}
	finishReason := oai.FinishReasonStop

	// Try to extract tool calls
	if toolCalls := extractToolCallsFromText(text); len(toolCalls) > 0 {
		msg.Content = ""
		msg.ToolCalls = toolCalls
		finishReason = oai.FinishReasonToolCalls
	}

	return oai.ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-codex-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []oai.ChatCompletionChoice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
	}
}

// extractToolCallsFromText looks for tool call JSON in various formats the LLM might produce.
func extractToolCallsFromText(text string) []oai.ToolCall {
	// 1. Try fenced block: ```tool_calls ... ```
	markers := []string{"```tool_calls\n", "```tool_calls\r\n", "```tool_calls "}
	for _, marker := range markers {
		if idx := strings.Index(text, marker); idx >= 0 {
			start := idx + len(marker)
			end := strings.Index(text[start:], "```")
			if end == -1 {
				end = len(text) - start
			}
			jsonStr := strings.TrimSpace(text[start : start+end])
			if calls := parseToolCallsJSON(jsonStr); len(calls) > 0 {
				return calls
			}
		}
	}

	// 2. Try unfenced: "tool_calls\n[...]" (no backticks)
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "tool_calls") {
		afterPrefix := strings.TrimPrefix(trimmed, "tool_calls")
		afterPrefix = strings.TrimSpace(afterPrefix)
		if strings.HasPrefix(afterPrefix, "[") {
			if calls := parseToolCallsJSON(afterPrefix); len(calls) > 0 {
				return calls
			}
		}
	}

	// 3. Try: entire response is a JSON array of tool calls
	if strings.HasPrefix(trimmed, "[{") && strings.HasSuffix(trimmed, "}]") {
		if calls := parseToolCallsJSON(trimmed); len(calls) > 0 {
			return calls
		}
	}

	// 4. Try: find first JSON array in the text that looks like tool calls
	if idx := strings.Index(text, "[{\"id\""); idx >= 0 {
		// Find matching closing bracket
		depth := 0
		for i := idx; i < len(text); i++ {
			if text[i] == '[' {
				depth++
			} else if text[i] == ']' {
				depth--
				if depth == 0 {
					jsonStr := text[idx : i+1]
					if calls := parseToolCallsJSON(jsonStr); len(calls) > 0 {
						return calls
					}
					break
				}
			}
		}
	}

	return nil
}

type rawToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func parseToolCallsJSON(jsonStr string) []oai.ToolCall {
	var raw []rawToolCall
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		log.Printf("codex provider: failed to parse tool calls JSON: %v", err)
		return nil
	}

	var result []oai.ToolCall
	for i, r := range raw {
		id := r.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		result = append(result, oai.ToolCall{
			ID:   id,
			Type: oai.ToolTypeFunction,
			Function: oai.FunctionCall{
				Name:      r.Function.Name,
				Arguments: r.Function.Arguments,
			},
		})
	}
	return result
}
