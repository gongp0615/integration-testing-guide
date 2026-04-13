package tools

import (
	oai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

type ToolName string

const (
	ToolSendCommand ToolName = "send_command"
)

type SendCommandParams struct {
	Cmd      string `json:"cmd"`
	PlayerID int    `json:"playerId,omitempty"`
	Sub      string `json:"sub,omitempty"`
	ItemID   int    `json:"itemId,omitempty"`
	Count    int    `json:"count,omitempty"`
	TaskID   int    `json:"taskId,omitempty"`
	AchID    int    `json:"achId,omitempty"`
}

func Definitions() []oai.Tool {
	return []oai.Tool{
		{
			Type: oai.ToolTypeFunction,
			Function: &oai.FunctionDefinition{
				Name:        string(ToolSendCommand),
				Description: "Send a command to the game server via WebSocket and return the response. Use this to query game state, enqueue operations, or step through execution.",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"cmd": {
							Type:        jsonschema.String,
							Description: "Command to send: playermgr, additem, removeitem, next, help",
						},
						"playerId": {
							Type:        jsonschema.Number,
							Description: "Player ID (required for playermgr, additem, removeitem)",
						},
						"sub": {
							Type:        jsonschema.String,
							Description: "Sub-module to query: bag, task, achievement (used with playermgr)",
						},
						"itemId": {
							Type:        jsonschema.Number,
							Description: "Item ID (used with additem, removeitem, or playermgr+sub=bag)",
						},
						"count": {
							Type:        jsonschema.Number,
							Description: "Item count (used with additem, removeitem)",
						},
						"taskId": {
							Type:        jsonschema.Number,
							Description: "Task ID (used with playermgr+sub=task)",
						},
						"achId": {
							Type:        jsonschema.Number,
							Description: "Achievement ID (used with playermgr+sub=achievement)",
						},
					},
					Required: []string{"cmd"},
				},
			},
		},
	}
}
