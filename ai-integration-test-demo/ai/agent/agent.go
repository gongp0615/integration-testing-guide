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
	client  *oai.Client
	session *session.Session
	model   string
}

func New(apiKey, model, baseURL string, sess *session.Session) *Agent {
	cfg := oai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	return &Agent{
		client:  oai.NewClientWithConfig(cfg),
		session: sess,
		model:   model,
	}
}

func (a *Agent) Run(ctx context.Context, taskDesc string) (string, error) {
	messages := []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: prompt.SystemPrompt},
		{Role: oai.ChatMessageRoleUser, Content: taskDesc},
	}

	toolDefs := tools.Definitions()

	for i := 0; i < 30; i++ {
		resp, err := a.client.CreateChatCompletion(ctx, oai.ChatCompletionRequest{
			Model:    a.model,
			Messages: messages,
			Tools:    toolDefs,
		})
		if err != nil {
			return "", fmt.Errorf("openai api error: %w", err)
		}

		choice := resp.Choices[0]
		messages = append(messages, choice.Message)

		if choice.FinishReason == oai.FinishReasonStop {
			return choice.Message.Content, nil
		}

		if choice.FinishReason == oai.FinishReasonToolCalls {
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
