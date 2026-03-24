package main

import (
	"context"
	"fmt"

	"github.com/isaacphi/mcp-language-server/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *mcpServer) registerTools() error {
	coreLogger.Debug("Registering MCP tools")

	applyTextEditTool := mcp.NewTool("edit_file",
		mcp.WithDescription("Apply multiple text edits to a file."),
		mcp.WithArray("edits",
			mcp.Required(),
			mcp.Description("List of edits to apply"),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"startLine": map[string]any{
						"type":        "number",
						"description": "Start line to replace, inclusive, one-indexed",
					},
					"endLine": map[string]any{
						"type":        "number",
						"description": "End line to replace, inclusive, one-indexed",
					},
					"newText": map[string]any{
						"type":        "string",
						"description": "Replacement text. Replace with the new text. Leave blank to remove lines.",
					},
				},
				"required": []string{"startLine", "endLine"},
			}),
		),
		mcp.WithString("filePath",
			mcp.Required(),
			mcp.Description("Path to the file to edit"),
		),
	)

	s.mcpServer.AddTool(applyTextEditTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract arguments
		filePath, ok := request.Params.Arguments["filePath"].(string)
		if !ok {
			return mcp.NewToolResultError("filePath must be a string"), nil
		}

		// Extract edits array
		editsArg, ok := request.Params.Arguments["edits"]
		if !ok {
			return mcp.NewToolResultError("edits is required"), nil
		}

		// Type assert and convert the edits
		editsArray, ok := editsArg.([]any)
		if !ok {
			return mcp.NewToolResultError("edits must be an array"), nil
		}

		var edits []tools.TextEdit
		for _, editItem := range editsArray {
			editMap, ok := editItem.(map[string]any)
			if !ok {
				return mcp.NewToolResultError("each edit must be an object"), nil
			}

			startLine, ok := editMap["startLine"].(float64)
			if !ok {
				return mcp.NewToolResultError("startLine must be a number"), nil
			}

			endLine, ok := editMap["endLine"].(float64)
			if !ok {
				return mcp.NewToolResultError("endLine must be a number"), nil
			}

			newText, _ := editMap["newText"].(string) // newText can be empty

			edits = append(edits, tools.TextEdit{
				StartLine: int(startLine),
				EndLine:   int(endLine),
				NewText:   newText,
			})
		}

		coreLogger.Debug("Executing edit_file for file: %s", filePath)
		response, err := tools.ApplyTextEdits(s.ctx, s.lspClient, filePath, edits)
		if err != nil {
			coreLogger.Error("Failed to apply edits: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to apply edits: %v", err)), nil
		}
		return mcp.NewToolResultText(response), nil
	})

	readDefinitionTool := mcp.NewTool("definition",
		mcp.WithDescription("Read the source code definition of a symbol (function, type, constant, etc.) from the codebase. Returns the complete implementation code where the symbol is defined."),
		mcp.WithString("symbolName",
			mcp.Required(),
			mcp.Description("The name of the symbol whose definition you want to find (e.g. 'mypackage.MyFunction', 'MyType.MyMethod')"),
		),
	)

	s.mcpServer.AddTool(readDefinitionTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract arguments
		symbolName, ok := request.Params.Arguments["symbolName"].(string)
		if !ok {
			return mcp.NewToolResultError("symbolName must be a string"), nil
		}

		coreLogger.Debug("Executing definition for symbol: %s", symbolName)
		text, err := tools.ReadDefinition(s.ctx, s.lspClient, symbolName)
		if err != nil {
			coreLogger.Error("Failed to get definition: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to get definition: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	})

	findReferencesTool := mcp.NewTool("references",
		mcp.WithDescription("Find all usages and references of a symbol throughout the codebase. Returns a list of all files and locations where the symbol appears."),
		mcp.WithString("symbolName",
			mcp.Required(),
			mcp.Description("The name of the symbol to search for (e.g. 'mypackage.MyFunction', 'MyType')"),
		),
	)

	s.mcpServer.AddTool(findReferencesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract arguments
		symbolName, ok := request.Params.Arguments["symbolName"].(string)
		if !ok {
			return mcp.NewToolResultError("symbolName must be a string"), nil
		}

		coreLogger.Debug("Executing references for symbol: %s", symbolName)
		text, err := tools.FindReferences(s.ctx, s.lspClient, symbolName)
		if err != nil {
			coreLogger.Error("Failed to find references: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to find references: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	})

	getDiagnosticsTool := mcp.NewTool("diagnostics",
		mcp.WithDescription("Get diagnostic information for a specific file from the language server."),
		mcp.WithString("filePath",
			mcp.Required(),
			mcp.Description("The path to the file to get diagnostics for"),
		),
		mcp.WithBoolean("contextLines",
			mcp.Description("Lines to include around each diagnostic."),
			mcp.DefaultBool(false),
		),
		mcp.WithBoolean("showLineNumbers",
			mcp.Description("If true, adds line numbers to the output"),
			mcp.DefaultBool(true),
		),
	)

	s.mcpServer.AddTool(getDiagnosticsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract arguments
		filePath, ok := request.Params.Arguments["filePath"].(string)
		if !ok {
			return mcp.NewToolResultError("filePath must be a string"), nil
		}

		contextLines := 5 // default value
		if contextLinesArg, ok := request.Params.Arguments["contextLines"].(int); ok {
			contextLines = contextLinesArg
		}

		showLineNumbers := true // default value
		if showLineNumbersArg, ok := request.Params.Arguments["showLineNumbers"].(bool); ok {
			showLineNumbers = showLineNumbersArg
		}

		coreLogger.Debug("Executing diagnostics for file: %s", filePath)
		text, err := tools.GetDiagnosticsForFile(s.ctx, s.lspClient, filePath, contextLines, showLineNumbers)
		if err != nil {
			coreLogger.Error("Failed to get diagnostics: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to get diagnostics: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	})

	getCodeLensTool := mcp.NewTool("get_codelens",
		mcp.WithDescription("Get code lens hints for a given file from the language server."),
		mcp.WithString("filePath",
			mcp.Required(),
			mcp.Description("The path to the file to get code lens information for"),
		),
	)

	s.mcpServer.AddTool(getCodeLensTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, ok := request.Params.Arguments["filePath"].(string)
		if !ok {
			return mcp.NewToolResultError("filePath must be a string"), nil
		}

		coreLogger.Debug("Executing get_codelens for file: %s", filePath)
		text, err := tools.GetCodeLens(s.ctx, s.lspClient, filePath)
		if err != nil {
			coreLogger.Error("Failed to get code lens: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to get code lens: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	})

	executeCodeLensTool := mcp.NewTool("execute_codelens",
		mcp.WithDescription("Execute a code lens command for a given file and lens index."),
		mcp.WithString("filePath",
			mcp.Required(),
			mcp.Description("The path to the file containing the code lens to execute"),
		),
		mcp.WithNumber("index",
			mcp.Required(),
			mcp.Description("The index of the code lens to execute (from get_codelens output), 1 indexed"),
		),
	)

	s.mcpServer.AddTool(executeCodeLensTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, ok := request.Params.Arguments["filePath"].(string)
		if !ok {
			return mcp.NewToolResultError("filePath must be a string"), nil
		}

		var index int
		switch v := request.Params.Arguments["index"].(type) {
		case float64:
			index = int(v)
		case int:
			index = v
		default:
			return mcp.NewToolResultError("index must be a number"), nil
		}

		coreLogger.Debug("Executing execute_codelens for file: %s index: %d", filePath, index)
		text, err := tools.ExecuteCodeLens(s.ctx, s.lspClient, filePath, index)
		if err != nil {
			coreLogger.Error("Failed to execute code lens: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to execute code lens: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	})

	codeActionsTool := mcp.NewTool("code_actions",
		mcp.WithDescription("List available code actions (refactorings, quick fixes) at the specified position or range in a file."),
		mcp.WithString("filePath",
			mcp.Required(),
			mcp.Description("The path to the file"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("The start line number (1-indexed)"),
		),
		mcp.WithNumber("column",
			mcp.Required(),
			mcp.Description("The start column number (1-indexed)"),
		),
		mcp.WithNumber("endLine",
			mcp.Description("The end line number (1-indexed). Defaults to line."),
		),
		mcp.WithNumber("endColumn",
			mcp.Description("The end column number (1-indexed). Defaults to column."),
		),
	)

	s.mcpServer.AddTool(codeActionsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, ok := request.Params.Arguments["filePath"].(string)
		if !ok {
			return mcp.NewToolResultError("filePath must be a string"), nil
		}

		var line, column int
		switch v := request.Params.Arguments["line"].(type) {
		case float64:
			line = int(v)
		case int:
			line = v
		default:
			return mcp.NewToolResultError("line must be a number"), nil
		}

		switch v := request.Params.Arguments["column"].(type) {
		case float64:
			column = int(v)
		case int:
			column = v
		default:
			return mcp.NewToolResultError("column must be a number"), nil
		}

		endLine := line
		if v, ok := request.Params.Arguments["endLine"]; ok && v != nil {
			switch val := v.(type) {
			case float64:
				endLine = int(val)
			case int:
				endLine = val
			}
		}

		endColumn := column
		if v, ok := request.Params.Arguments["endColumn"]; ok && v != nil {
			switch val := v.(type) {
			case float64:
				endColumn = int(val)
			case int:
				endColumn = val
			}
		}

		coreLogger.Debug("Executing code_actions for file: %s L%d:C%d-L%d:C%d", filePath, line, column, endLine, endColumn)
		text, err := tools.GetCodeActions(s.ctx, s.lspClient, filePath, line, column, endLine, endColumn)
		if err != nil {
			coreLogger.Error("Failed to get code actions: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to get code actions: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	})

	executeCodeActionTool := mcp.NewTool("execute_code_action",
		mcp.WithDescription("Execute a code action (refactoring, quick fix) by title at the specified position or range."),
		mcp.WithString("filePath",
			mcp.Required(),
			mcp.Description("The path to the file"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("The start line number (1-indexed)"),
		),
		mcp.WithNumber("column",
			mcp.Required(),
			mcp.Description("The start column number (1-indexed)"),
		),
		mcp.WithString("actionTitle",
			mcp.Required(),
			mcp.Description("The title of the code action to execute (exact or substring match, case-insensitive)"),
		),
		mcp.WithNumber("endLine",
			mcp.Description("The end line number (1-indexed). Defaults to line."),
		),
		mcp.WithNumber("endColumn",
			mcp.Description("The end column number (1-indexed). Defaults to column."),
		),
	)

	s.mcpServer.AddTool(executeCodeActionTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, ok := request.Params.Arguments["filePath"].(string)
		if !ok {
			return mcp.NewToolResultError("filePath must be a string"), nil
		}

		actionTitle, ok := request.Params.Arguments["actionTitle"].(string)
		if !ok {
			return mcp.NewToolResultError("actionTitle must be a string"), nil
		}

		var line, column int
		switch v := request.Params.Arguments["line"].(type) {
		case float64:
			line = int(v)
		case int:
			line = v
		default:
			return mcp.NewToolResultError("line must be a number"), nil
		}

		switch v := request.Params.Arguments["column"].(type) {
		case float64:
			column = int(v)
		case int:
			column = v
		default:
			return mcp.NewToolResultError("column must be a number"), nil
		}

		endLine := line
		if v, ok := request.Params.Arguments["endLine"]; ok && v != nil {
			switch val := v.(type) {
			case float64:
				endLine = int(val)
			case int:
				endLine = val
			}
		}

		endColumn := column
		if v, ok := request.Params.Arguments["endColumn"]; ok && v != nil {
			switch val := v.(type) {
			case float64:
				endColumn = int(val)
			case int:
				endColumn = val
			}
		}

		coreLogger.Debug("Executing execute_code_action for file: %s action: %s", filePath, actionTitle)
		text, err := tools.ExecuteCodeAction(s.ctx, s.lspClient, filePath, line, column, endLine, endColumn, actionTitle)
		if err != nil {
			coreLogger.Error("Failed to execute code action: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to execute code action: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	})

	executeCommandTool := mcp.NewTool("execute_command",
		mcp.WithDescription("Execute a workspace command on the language server (e.g. clojure-lsp-server-info, clean-ns)."),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("The command identifier to execute"),
		),
		mcp.WithString("arguments",
			mcp.Description("JSON array of arguments for the command (e.g. '[\"file:///path/to/file.clj\", 1, 2]')"),
		),
	)

	s.mcpServer.AddTool(executeCommandTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		command, ok := request.Params.Arguments["command"].(string)
		if !ok {
			return mcp.NewToolResultError("command must be a string"), nil
		}

		arguments := ""
		if v, ok := request.Params.Arguments["arguments"].(string); ok {
			arguments = v
		}

		coreLogger.Debug("Executing execute_command: %s", command)
		text, err := tools.ExecuteWorkspaceCommand(s.ctx, s.lspClient, command, arguments)
		if err != nil {
			coreLogger.Error("Failed to execute command: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to execute command: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	})

	hoverTool := mcp.NewTool("hover",
		mcp.WithDescription("Get hover information (type, documentation) for a symbol at the specified position."),
		mcp.WithString("filePath",
			mcp.Required(),
			mcp.Description("The path to the file to get hover information for"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("The line number where the hover is requested (1-indexed)"),
		),
		mcp.WithNumber("column",
			mcp.Required(),
			mcp.Description("The column number where the hover is requested (1-indexed)"),
		),
	)

	s.mcpServer.AddTool(hoverTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract arguments
		filePath, ok := request.Params.Arguments["filePath"].(string)
		if !ok {
			return mcp.NewToolResultError("filePath must be a string"), nil
		}

		// Handle both float64 and int for line and column due to JSON parsing
		var line, column int
		switch v := request.Params.Arguments["line"].(type) {
		case float64:
			line = int(v)
		case int:
			line = v
		default:
			return mcp.NewToolResultError("line must be a number"), nil
		}

		switch v := request.Params.Arguments["column"].(type) {
		case float64:
			column = int(v)
		case int:
			column = v
		default:
			return mcp.NewToolResultError("column must be a number"), nil
		}

		coreLogger.Debug("Executing hover for file: %s line: %d column: %d", filePath, line, column)
		text, err := tools.GetHoverInfo(s.ctx, s.lspClient, filePath, line, column)
		if err != nil {
			coreLogger.Error("Failed to get hover information: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to get hover information: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	})

	renameSymbolTool := mcp.NewTool("rename_symbol",
		mcp.WithDescription("Rename a symbol (variable, function, class, etc.) at the specified position and update all references throughout the codebase."),
		mcp.WithString("filePath",
			mcp.Required(),
			mcp.Description("The path to the file containing the symbol to rename"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("The line number where the symbol is located (1-indexed)"),
		),
		mcp.WithNumber("column",
			mcp.Required(),
			mcp.Description("The column number where the symbol is located (1-indexed)"),
		),
		mcp.WithString("newName",
			mcp.Required(),
			mcp.Description("The new name for the symbol"),
		),
	)

	s.mcpServer.AddTool(renameSymbolTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract arguments
		filePath, ok := request.Params.Arguments["filePath"].(string)
		if !ok {
			return mcp.NewToolResultError("filePath must be a string"), nil
		}

		newName, ok := request.Params.Arguments["newName"].(string)
		if !ok {
			return mcp.NewToolResultError("newName must be a string"), nil
		}

		// Handle both float64 and int for line and column due to JSON parsing
		var line, column int
		switch v := request.Params.Arguments["line"].(type) {
		case float64:
			line = int(v)
		case int:
			line = v
		default:
			return mcp.NewToolResultError("line must be a number"), nil
		}

		switch v := request.Params.Arguments["column"].(type) {
		case float64:
			column = int(v)
		case int:
			column = v
		default:
			return mcp.NewToolResultError("column must be a number"), nil
		}

		coreLogger.Debug("Executing rename_symbol for file: %s line: %d column: %d newName: %s", filePath, line, column, newName)
		text, err := tools.RenameSymbol(s.ctx, s.lspClient, filePath, line, column, newName)
		if err != nil {
			coreLogger.Error("Failed to rename symbol: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to rename symbol: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	})

	coreLogger.Info("Successfully registered all MCP tools")
	return nil
}
