package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	oai "github.com/sashabaranov/go-openai"
)

// AnthropicProvider calls the Anthropic Messages API (or compatible endpoints
// like Zhipu's /api/anthropic). It translates between the OpenAI types used
// by the agent loop and the Anthropic wire format.
type AnthropicProvider struct {
	apiKey  string
	baseURL string // e.g. "https://open.bigmodel.cn/api/anthropic"
	model   string
	client  *http.Client
}

func NewAnthropicProvider(apiKey, baseURL, model string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 300 * time.Second},
	}
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) ChatCompletion(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
	anthReq := p.buildRequest(req)

	body, err := json.Marshal(anthReq)
	if err != nil {
		return oai.ChatCompletionResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	url := p.baseURL + "/v1/messages"

	const maxRetries = 5
	backoff := 10 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return oai.ChatCompletionResponse{}, fmt.Errorf("create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", p.apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")

		resp, err := p.client.Do(httpReq)
		if err != nil {
			// Retry on transient network errors (EOF, timeout)
			if attempt < maxRetries {
				log.Printf("anthropic request error (attempt %d/%d): %v, retrying in %v", attempt, maxRetries, err, backoff)
				select {
				case <-ctx.Done():
					return oai.ChatCompletionResponse{}, ctx.Err()
				case <-time.After(backoff):
				}
				backoff = backoff * 2
				if backoff > 120*time.Second {
					backoff = 120 * time.Second
				}
				continue
			}
			return oai.ChatCompletionResponse{}, fmt.Errorf("http request: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if attempt < maxRetries {
				log.Printf("anthropic read error (attempt %d/%d): %v, retrying in %v", attempt, maxRetries, err, backoff)
				select {
				case <-ctx.Done():
					return oai.ChatCompletionResponse{}, ctx.Err()
				case <-time.After(backoff):
				}
				backoff = backoff * 2
				if backoff > 120*time.Second {
					backoff = 120 * time.Second
				}
				continue
			}
			return oai.ChatCompletionResponse{}, fmt.Errorf("read response: %w", err)
		}

		// Retry on 429 (rate limit) and 529/503 (overloaded)
		if resp.StatusCode == 429 || resp.StatusCode == 529 || resp.StatusCode == 503 {
			if attempt < maxRetries {
				log.Printf("anthropic %d (attempt %d/%d): %s, retrying in %v", resp.StatusCode, attempt, maxRetries, string(respBody), backoff)
				select {
				case <-ctx.Done():
					return oai.ChatCompletionResponse{}, ctx.Err()
				case <-time.After(backoff):
				}
				backoff = backoff * 2
				if backoff > 120*time.Second {
					backoff = 120 * time.Second
				}
				continue
			}
		}

		if resp.StatusCode != 200 {
			return oai.ChatCompletionResponse{}, fmt.Errorf("anthropic api error %d: %s", resp.StatusCode, string(respBody))
		}

		var anthResp anthropicResponse
		if err := json.Unmarshal(respBody, &anthResp); err != nil {
			return oai.ChatCompletionResponse{}, fmt.Errorf("unmarshal response: %w (body: %s)", err, string(respBody))
		}

		return p.convertResponse(anthResp), nil
	}

	return oai.ChatCompletionResponse{}, fmt.Errorf("exhausted %d retries", maxRetries)
}

// ── Anthropic request types ──────────────────────────────────

type anthropicRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	System    string            `json:"system,omitempty"`
	Messages  []anthropicMsg    `json:"messages"`
	Tools     []anthropicTool   `json:"tools,omitempty"`
}

type anthropicMsg struct {
	Role    string        `json:"role"`
	Content interface{}   `json:"content"` // string or []contentBlock
}

type contentBlock struct {
	Type      string      `json:"type"`
	Text      string      `json:"text,omitempty"`
	ID        string      `json:"id,omitempty"`
	Name      string      `json:"name,omitempty"`
	Input     interface{} `json:"input,omitempty"`
	ToolUseID string      `json:"tool_use_id,omitempty"`
	Content   string      `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

// ── Anthropic response types ─────────────────────────────────

type anthropicResponse struct {
	ID         string              `json:"id"`
	Type       string              `json:"type"`
	Role       string              `json:"role"`
	Model      string              `json:"model"`
	Content    []anthropicContent  `json:"content"`
	StopReason string              `json:"stop_reason"` // "end_turn", "tool_use", "max_tokens"
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicContent struct {
	Type  string          `json:"type"` // "text" or "tool_use"
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// ── Format conversion ────────────────────────────────────────

func (p *AnthropicProvider) buildRequest(req oai.ChatCompletionRequest) anthropicRequest {
	anthReq := anthropicRequest{
		Model:     p.model,
		MaxTokens: 4096,
	}

	// Extract system message and convert the rest
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			anthReq.System = msg.Content

		case "user":
			anthReq.Messages = append(anthReq.Messages, anthropicMsg{
				Role:    "user",
				Content: msg.Content,
			})

		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Convert tool calls to Anthropic content blocks
				var blocks []contentBlock
				if msg.Content != "" {
					blocks = append(blocks, contentBlock{Type: "text", Text: msg.Content})
				}
				for _, tc := range msg.ToolCalls {
					var input interface{}
					json.Unmarshal([]byte(tc.Function.Arguments), &input)
					blocks = append(blocks, contentBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: input,
					})
				}
				anthReq.Messages = append(anthReq.Messages, anthropicMsg{
					Role:    "assistant",
					Content: blocks,
				})
			} else {
				anthReq.Messages = append(anthReq.Messages, anthropicMsg{
					Role:    "assistant",
					Content: msg.Content,
				})
			}

		case "tool":
			// Anthropic expects tool results as user messages with tool_result blocks
			// We need to merge consecutive tool results into one user message
			block := contentBlock{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   msg.Content,
			}
			// Check if the last message is already a user with tool_result blocks
			if len(anthReq.Messages) > 0 {
				last := &anthReq.Messages[len(anthReq.Messages)-1]
				if last.Role == "user" {
					if blocks, ok := last.Content.([]contentBlock); ok {
						last.Content = append(blocks, block)
						continue
					}
				}
			}
			anthReq.Messages = append(anthReq.Messages, anthropicMsg{
				Role:    "user",
				Content: []contentBlock{block},
			})
		}
	}

	// Ensure messages alternate user/assistant (Anthropic requirement)
	anthReq.Messages = ensureAlternating(anthReq.Messages)

	// Convert tools
	for _, t := range req.Tools {
		if t.Function == nil {
			continue
		}
		anthReq.Tools = append(anthReq.Tools, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	return anthReq
}

// ensureAlternating merges consecutive same-role messages.
// Anthropic requires strict user/assistant alternation.
func ensureAlternating(msgs []anthropicMsg) []anthropicMsg {
	if len(msgs) == 0 {
		return msgs
	}

	var result []anthropicMsg
	for _, msg := range msgs {
		if len(result) > 0 && result[len(result)-1].Role == msg.Role {
			// Merge into previous message
			prev := &result[len(result)-1]
			prevStr, prevIsStr := prev.Content.(string)
			curStr, curIsStr := msg.Content.(string)
			if prevIsStr && curIsStr {
				prev.Content = prevStr + "\n\n" + curStr
			} else {
				// Convert both to block arrays and merge
				prevBlocks := toBlocks(prev.Content)
				curBlocks := toBlocks(msg.Content)
				prev.Content = append(prevBlocks, curBlocks...)
			}
		} else {
			result = append(result, msg)
		}
	}

	// Anthropic requires first message to be user
	if len(result) > 0 && result[0].Role != "user" {
		result = append([]anthropicMsg{{Role: "user", Content: "Begin."}}, result...)
	}

	return result
}

func toBlocks(content interface{}) []contentBlock {
	switch v := content.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []contentBlock{{Type: "text", Text: v}}
	case []contentBlock:
		return v
	default:
		return nil
	}
}

func (p *AnthropicProvider) convertResponse(resp anthropicResponse) oai.ChatCompletionResponse {
	msg := oai.ChatCompletionMessage{
		Role: oai.ChatMessageRoleAssistant,
	}
	finishReason := oai.FinishReasonStop

	var textParts []string
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			argsBytes, _ := json.Marshal(block.Input)
			// Input is already the parsed object; for OpenAI format we need it as a JSON string
			msg.ToolCalls = append(msg.ToolCalls, oai.ToolCall{
				ID:   block.ID,
				Type: oai.ToolTypeFunction,
				Function: oai.FunctionCall{
					Name:      block.Name,
					Arguments: string(argsBytes),
				},
			})
		}
	}

	if len(msg.ToolCalls) > 0 {
		finishReason = oai.FinishReasonToolCalls
	}
	msg.Content = joinNonEmpty(textParts, "\n")

	return oai.ChatCompletionResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []oai.ChatCompletionChoice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
		Usage: oai.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

func joinNonEmpty(parts []string, sep string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) == 0 {
		return ""
	}
	result := nonEmpty[0]
	for _, p := range nonEmpty[1:] {
		result += sep + p
	}
	return result
}
