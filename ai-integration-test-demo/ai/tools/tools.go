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
	Name     string `json:"name,omitempty"`
	Target   string `json:"target,omitempty"`
	Action   string `json:"action,omitempty"`
	Desc     string `json:"description,omitempty"`
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

type RegisterCmdParams struct {
	Name        string `json:"name"`
	Target      string `json:"target"`
	Action      string `json:"action"`
	Description string `json:"description,omitempty"`
}

type LSPReferencesParams struct {
	File   string `json:"file"`
	Symbol string `json:"symbol"`
}

type LSPDefinitionParams struct {
	File   string `json:"file"`
	Symbol string `json:"symbol"`
}

type LSPSymbolsParams struct {
	Query string `json:"query"`
}

// HasCodeAccess returns true if the mode supports source code reading.
func HasCodeAccess(mode string) bool {
	return mode == "code-batch" || mode == "dual" || mode == "code-only" || mode == "l0" || mode == "l1"
}

func hasStepMode(mode string) bool {
	return mode == "step-only" || mode == "dual" || mode == "code-only" || mode == "l0" || mode == "l1"
}

func hasLSP(mode string, lspAvailable bool) bool {
	return lspAvailable && HasCodeAccess(mode)
}

func hasRuntime(mode string) bool {
	return mode != "code-only"
}

func hasRegisterCmd(mode string) bool {
	return mode == "l0" || mode == "l1"
}

func Definitions(mode string, lspAvailable ...bool) []oai.Tool {
	lspOK := len(lspAvailable) > 0 && lspAvailable[0]
	var toolList []oai.Tool
	if hasRuntime(mode) {
		toolList = append(toolList, sendCommandTool(mode))
	}
	if hasRegisterCmd(mode) {
		toolList = append(toolList, registerCmdTool())
	}
	if HasCodeAccess(mode) {
		toolList = append(toolList, readFileTool())
		toolList = append(toolList, searchCodeTool())
		toolList = append(toolList, updateKnowledgeTool())
		if hasLSP(mode, lspOK) {
			toolList = append(toolList, lspReferencesTool())
			toolList = append(toolList, lspDefinitionTool())
			toolList = append(toolList, lspSymbolsTool())
		}
	}
	return toolList
}

func sendCommandTool(mode string) oai.Tool {
	cmdDesc := "Command: "
	switch mode {
	case "l0":
		cmdDesc += "playermgr, next, batch, help, listcmd, or any registered custom command"
	case "l1":
		cmdDesc += "playermgr, additem, removeitem, checkin, claimreward, equip, unequip, claimmail, next, batch, help, listcmd, or any registered custom command"
	case "step-only", "dual", "code-only":
		cmdDesc += "playermgr, additem, removeitem, checkin, claimreward, equip, unequip, claimmail, next, help"
	default:
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
		"itemId": {Type: jsonschema.Number, Description: "Item ID"},
		"count":  {Type: jsonschema.Number, Description: "Item count"},
		"taskId": {Type: jsonschema.Number, Description: "Task ID"},
		"achId":  {Type: jsonschema.Number, Description: "Achievement ID"},
		"day":    {Type: jsonschema.Number, Description: "Sign-in day number 1-7"},
		"mailId": {Type: jsonschema.Number, Description: "Mail ID"},
		"slot":   {Type: jsonschema.String, Description: "Equipment slot: weapon or armor"},
	}
	if hasStepMode(mode) {
		props["sub"] = jsonschema.Definition{Type: jsonschema.String, Description: "Sub-module: bag, task, achievement, equipment, signin, mail"}
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

func registerCmdTool() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "register_cmd",
			Description: "Register a new named test command that maps to a whitelisted raw business action. Use this to create purpose-specific validation commands such as test_remove_negative.",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"name":        {Type: jsonschema.String, Description: "Unique command name to register"},
					"target":      {Type: jsonschema.String, Description: "Target module: bag, signin, equipment, mail"},
					"action":      {Type: jsonschema.String, Description: "Whitelisted raw action on the target module"},
					"description": {Type: jsonschema.String, Description: "Optional human-readable purpose of the command"},
				},
				Required: []string{"name", "target", "action"},
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

func lspReferencesTool() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "lsp_references",
			Description: "Find all references to a symbol across the project using semantic code analysis (via LSP/gopls). More accurate than text search — finds actual usages, not string matches.",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"file":   {Type: jsonschema.String, Description: "Relative file path containing the symbol, e.g. internal/event/bus.go"},
					"symbol": {Type: jsonschema.String, Description: "Symbol name to find references for, e.g. Publish or Subscribe"},
				},
				Required: []string{"file", "symbol"},
			},
		},
	}
}

func lspDefinitionTool() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "lsp_definition",
			Description: "Go to the definition of a symbol. Returns the file and line where the symbol is defined.",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"file":   {Type: jsonschema.String, Description: "Relative file path where the symbol is used, e.g. internal/task/task.go"},
					"symbol": {Type: jsonschema.String, Description: "Symbol name to find the definition of, e.g. AddItem"},
				},
				Required: []string{"file", "symbol"},
			},
		},
	}
}

func lspSymbolsTool() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "lsp_symbols",
			Description: "Search for symbols (functions, structs, methods, variables) across the workspace. Returns symbol name, kind, file, and line number.",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"query": {Type: jsonschema.String, Description: "Search query for symbol names, e.g. Subscribe or Remove. Use empty string for all symbols."},
				},
				Required: []string{"query"},
			},
		},
	}
}
