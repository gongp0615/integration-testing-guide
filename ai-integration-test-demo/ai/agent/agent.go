package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/example/ai-integration-test-demo/ai/prompt"
	"github.com/example/ai-integration-test-demo/ai/session"
	"github.com/example/ai-integration-test-demo/ai/tools"
	oai "github.com/sashabaranov/go-openai"
)

type Agent struct {
	client      *oai.Client
	session     *session.Session
	model       string
	mode        string
	codeSummary string
}

func New(apiKey, model, baseURL string, sess *session.Session, mode string, codeSummary string) *Agent {
	cfg := oai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	return &Agent{
		client:      oai.NewClientWithConfig(cfg),
		session:     sess,
		model:       model,
		mode:        mode,
		codeSummary: codeSummary,
	}
}

func (a *Agent) Run(ctx context.Context, taskDesc string) (string, error) {
	sysPrompt := prompt.BuildPrompt(a.mode, prompt.PromptOptions{
		DocContent: a.codeSummary,
	})

	messages := []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: sysPrompt},
		{Role: oai.ChatMessageRoleUser, Content: taskDesc},
	}

	toolDefs := tools.Definitions(a.mode)

	maxIter := 80
	if a.mode == "code-only" {
		maxIter = 15
	}

	warnAt := maxIter - 8
	forcedAt := maxIter - 3

	for i := 0; i < maxIter; i++ {
		if i == warnAt {
			messages = append(messages, oai.ChatCompletionMessage{
				Role:    oai.ChatMessageRoleUser,
				Content: "[SYSTEM] You are approaching the iteration limit. Start wrapping up now. Produce your final Correlation Map and Defect Report. Do NOT make more tool calls.",
			})
		}

		req := oai.ChatCompletionRequest{
			Model:    a.model,
			Messages: messages,
		}

		if i < forcedAt && len(toolDefs) > 0 {
			req.Tools = toolDefs
		}

		resp, err := a.client.CreateChatCompletion(ctx, req)
		if err != nil {
			return "", fmt.Errorf("openai api error: %w", err)
		}

		choice := resp.Choices[0]
		messages = append(messages, choice.Message)

		if choice.FinishReason == oai.FinishReasonStop {
			return choice.Message.Content, nil
		}

		if choice.FinishReason == oai.FinishReasonToolCalls {
			if i >= forcedAt {
				messages = append(messages, oai.ChatCompletionMessage{
					Role:       oai.ChatMessageRoleTool,
					Content:    "[SYSTEM] Iteration limit reached. You MUST produce your final report now without any more tool calls.",
					ToolCallID: choice.Message.ToolCalls[0].ID,
				})
				continue
			}
			for _, tc := range choice.Message.ToolCalls {
				result, err := a.handleToolCall(tc)
				if err != nil {
					result = fmt.Sprintf("error: %v", err)
				}
				messages = append(messages, oai.ChatCompletionMessage{
					Role:       oai.ChatMessageRoleTool,
					Content:    result,
					ToolCallID: tc.ID,
				})
			}
			continue
		}

		log.Printf("unexpected finish reason: %s", choice.FinishReason)
	}

	return "", fmt.Errorf("max iterations reached")
}

func (a *Agent) handleToolCall(tc oai.ToolCall) (string, error) {
	if tc.Function.Name != "send_command" {
		return "", fmt.Errorf("unknown tool: %s", tc.Function.Name)
	}

	var params tools.SendCommandParams
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	log.Printf("AI → %s %+v", params.Cmd, params)

	result, err := a.session.SendCommand(params)
	if err != nil {
		return "", err
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return string(out), nil
}
