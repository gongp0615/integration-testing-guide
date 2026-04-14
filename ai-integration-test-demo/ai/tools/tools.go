package tools

import (
	oai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

type SendCommandParams struct {
	Cmd      string `json:"cmd"`
	PlayerID int    `json:"playerId,omitempty"`
	Sub      string `json:"sub,omitempty"`
	ItemID   int    `json:"itemId,omitempty"`
	Count    int    `json:"count,omitempty"`
	TaskID   int    `json:"taskId,omitempty"`
	AchID    int    `json:"achId,omitempty"`
	Day      int    `json:"day,omitempty"`
	MailID   int    `json:"mailId,omitempty"`
	Slot     string `json:"slot,omitempty"`
}

type ReadFileParams struct {
	Path string `json:"path"`
}

type SearchCodeParams struct {
	Directory string `json:"directory"`
	Pattern   string `json:"pattern"`
}

type UpdateKnowledgeParams struct {
	Content string `json:"content"`
}

func hasCodeAccess(mode string) bool {
	return mode == "code-batch" || mode == "dual" || mode == "code-only"
}

func hasStepMode(mode string) bool {
	return mode == "step-only" || mode == "dual" || mode == "code-only"
}

func hasRuntime(mode string) bool {
	return mode != "code-only"
}

func Definitions(mode string) []oai.Tool {
	var toolList []oai.Tool
	if hasRuntime(mode) {
		toolList = append(toolList, sendCommandTool(mode))
	}
	if hasCodeAccess(mode) {
		toolList = append(toolList, readFileTool())
		toolList = append(toolList, searchCodeTool())
		toolList = append(toolList, updateKnowledgeTool())
	}
	return toolList
}

func sendCommandTool(mode string) oai.Tool {
	cmdDesc := "Command: "
	if hasStepMode(mode) {
		cmdDesc += "playermgr, additem, removeitem, checkin, claimreward, equip, unequip, claimmail, next, help"
	} else {
		cmdDesc += "additem, removeitem, checkin, claimreward, equip, unequip, claimmail, batch, help"
	}
	props := map[string]jsonschema.Definition{
		"cmd": {
			Type:        jsonschema.String,
			Description: cmdDesc,
		},
		"playerId": {
			Type:        jsonschema.Number,
			Description: "Player ID (required for most commands)",
		},
	}
	if hasStepMode(mode) {
		props["sub"] = jsonschema.Definition{Type: jsonschema.String, Description: "Sub-module: bag, task, achievement, equipment, signin, mail"}
		props["itemId"] = jsonschema.Definition{Type: jsonschema.Number, Description: "Item ID"}
		props["count"] = jsonschema.Definition{Type: jsonschema.Number, Description: "Item count"}
		props["taskId"] = jsonschema.Definition{Type: jsonschema.Number, Description: "Task ID"}
		props["achId"] = jsonschema.Definition{Type: jsonschema.Number, Description: "Achievement ID"}
		props["day"] = jsonschema.Definition{Type: jsonschema.Number, Description: "Sign-in day number 1-7"}
		props["mailId"] = jsonschema.Definition{Type: jsonschema.Number, Description: "Mail ID"}
		props["slot"] = jsonschema.Definition{Type: jsonschema.String, Description: "Equipment slot: weapon or armor"}
	}
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "send_command",
			Description: "Send a command to the game server and return the response.",
			Parameters: jsonschema.Definition{
				Type:       jsonschema.Object,
				Properties: props,
				Required:   []string{"cmd"},
			},
		},
	}
}

func readFileTool() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "read_file",
			Description: "Read a source code file from the project. Path must be relative to project root under internal/ or ai/ directories.",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"path": {Type: jsonschema.String, Description: "Relative file path, e.g. internal/bag/bag.go"},
				},
				Required: []string{"path"},
			},
		},
	}
}

func searchCodeTool() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "search_code",
			Description: "Search for a keyword in Go source files under a directory. Returns matching lines with file and line number.",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"directory": {Type: jsonschema.String, Description: "Directory to search, e.g. internal/"},
					"pattern":   {Type: jsonschema.String, Description: "Substring to search for"},
				},
				Required: []string{"directory", "pattern"},
			},
		},
	}
}

func updateKnowledgeTool() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "update_knowledge",
			Description: "Update the knowledge.md file. Provide FULL content (overwrites existing).",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"content": {Type: jsonschema.String, Description: "Full content for knowledge.md"},
				},
				Required: []string{"content"},
			},
		},
	}
}
