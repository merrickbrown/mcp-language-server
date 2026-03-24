package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/isaacphi/mcp-language-server/internal/lsp"
	"github.com/isaacphi/mcp-language-server/internal/protocol"
	"github.com/isaacphi/mcp-language-server/internal/utilities"
)

// GetCodeActions retrieves available code actions for a given file and position range.
func GetCodeActions(ctx context.Context, client *lsp.Client, filePath string, line, column, endLine, endColumn int) (string, error) {
	err := client.SyncFile(ctx, filePath)
	if err != nil {
		return "", fmt.Errorf("could not open file: %v", err)
	}

	uri := protocol.DocumentUri("file://" + filePath)

	// Convert 1-indexed to 0-indexed
	startPos := protocol.Position{
		Line:      uint32(line - 1),
		Character: uint32(column - 1),
	}
	endPos := protocol.Position{
		Line:      uint32(endLine - 1),
		Character: uint32(endColumn - 1),
	}

	params := protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range: protocol.Range{
			Start: startPos,
			End:   endPos,
		},
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{},
		},
	}

	results, err := client.CodeAction(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to get code actions: %v", err)
	}

	if len(results) == 0 {
		return "No code actions available at this position.", nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Code actions for %s (L%d:C%d – L%d:C%d):\n\n", filePath, line, column, endLine, endColumn))

	for i, item := range results {
		switch action := item.Value.(type) {
		case protocol.CodeAction:
			preferred := ""
			if action.IsPreferred {
				preferred = " (preferred)"
			}
			disabled := ""
			if action.Disabled != nil {
				disabled = fmt.Sprintf(" [disabled: %s]", action.Disabled.Reason)
			}
			kind := ""
			if action.Kind != "" {
				kind = fmt.Sprintf(" [%s]", action.Kind)
			}
			output.WriteString(fmt.Sprintf("%d. %s%s%s%s\n", i+1, action.Title, kind, preferred, disabled))
		case protocol.Command:
			output.WriteString(fmt.Sprintf("%d. %s [command: %s]\n", i+1, action.Title, action.Command))
		default:
			output.WriteString(fmt.Sprintf("%d. (unknown type)\n", i+1))
		}
	}

	output.WriteString(fmt.Sprintf("\nFound %d code action(s). Use execute_code_action with the title to apply one.\n", len(results)))
	return output.String(), nil
}

// ExecuteCodeAction finds and executes a code action by title match.
func ExecuteCodeAction(ctx context.Context, client *lsp.Client, filePath string, line, column, endLine, endColumn int, actionTitle string) (string, error) {
	err := client.SyncFile(ctx, filePath)
	if err != nil {
		return "", fmt.Errorf("could not open file: %v", err)
	}

	uri := protocol.DocumentUri("file://" + filePath)

	// Convert 1-indexed to 0-indexed
	startPos := protocol.Position{
		Line:      uint32(line - 1),
		Character: uint32(column - 1),
	}
	endPos := protocol.Position{
		Line:      uint32(endLine - 1),
		Character: uint32(endColumn - 1),
	}

	params := protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range: protocol.Range{
			Start: startPos,
			End:   endPos,
		},
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{},
		},
	}

	results, err := client.CodeAction(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to get code actions: %v", err)
	}

	// Find matching action — exact match first, then case-insensitive substring
	var matched *protocol.CodeAction
	var matchedCommand *protocol.Command
	titleLower := strings.ToLower(actionTitle)

	// Pass 1: exact match (case-insensitive)
	for _, item := range results {
		switch action := item.Value.(type) {
		case protocol.CodeAction:
			if strings.EqualFold(action.Title, actionTitle) {
				a := action
				matched = &a
				break
			}
		case protocol.Command:
			if strings.EqualFold(action.Title, actionTitle) {
				c := action
				matchedCommand = &c
				break
			}
		}
		if matched != nil || matchedCommand != nil {
			break
		}
	}

	// Pass 2: substring match (case-insensitive)
	if matched == nil && matchedCommand == nil {
		for _, item := range results {
			switch action := item.Value.(type) {
			case protocol.CodeAction:
				if strings.Contains(strings.ToLower(action.Title), titleLower) {
					a := action
					matched = &a
					break
				}
			case protocol.Command:
				if strings.Contains(strings.ToLower(action.Title), titleLower) {
					c := action
					matchedCommand = &c
					break
				}
			}
			if matched != nil || matchedCommand != nil {
				break
			}
		}
	}

	if matched == nil && matchedCommand == nil {
		// Build list of available actions for the error message
		var available []string
		for _, item := range results {
			switch action := item.Value.(type) {
			case protocol.CodeAction:
				available = append(available, action.Title)
			case protocol.Command:
				available = append(available, action.Title)
			}
		}
		return "", fmt.Errorf("no code action matching %q found. Available: %s", actionTitle, strings.Join(available, ", "))
	}

	// Handle bare Command (not wrapped in CodeAction)
	if matchedCommand != nil {
		_, err := client.ExecuteCommand(ctx, protocol.ExecuteCommandParams{
			Command:   matchedCommand.Command,
			Arguments: matchedCommand.Arguments,
		})
		if err != nil {
			return "", fmt.Errorf("failed to execute command %q: %v", matchedCommand.Command, err)
		}
		return fmt.Sprintf("Successfully executed command: %s", matchedCommand.Title), nil
	}

	// Debug: log which fields are set on the matched action
	toolsLogger.Debug("Code action %q: Edit=%v, Command=%v, Data=%v",
		matched.Title, matched.Edit != nil, matched.Command != nil, matched.Data != nil)

	// Lazy resolution: if Edit is nil but Data is present, resolve the action
	if matched.Edit == nil && matched.Data != nil {
		toolsLogger.Debug("Resolving code action %q (has Data, no Edit)", matched.Title)
		resolved, err := client.ResolveCodeAction(ctx, *matched)
		if err != nil {
			return "", fmt.Errorf("failed to resolve code action %q: %v", matched.Title, err)
		}
		matched = &resolved
		toolsLogger.Debug("After resolve: Edit=%v, Command=%v", matched.Edit != nil, matched.Command != nil)
	}

	// Track affected files for NotifyChange
	affectedFiles := make(map[string]bool)

	// Apply Edit first (per LSP spec)
	if matched.Edit != nil {
		if err := utilities.ApplyWorkspaceEdit(*matched.Edit); err != nil {
			return "", fmt.Errorf("failed to apply workspace edit: %v", err)
		}

		// Collect affected files from Changes
		for editURI := range matched.Edit.Changes {
			path := strings.TrimPrefix(string(editURI), "file://")
			affectedFiles[path] = true
		}
		// Collect affected files from DocumentChanges
		for _, dc := range matched.Edit.DocumentChanges {
			if dc.TextDocumentEdit != nil {
				path := strings.TrimPrefix(string(dc.TextDocumentEdit.TextDocument.URI), "file://")
				affectedFiles[path] = true
			}
			if dc.CreateFile != nil {
				path := strings.TrimPrefix(string(dc.CreateFile.URI), "file://")
				affectedFiles[path] = true
			}
			if dc.RenameFile != nil {
				oldPath := strings.TrimPrefix(string(dc.RenameFile.OldURI), "file://")
				newPath := strings.TrimPrefix(string(dc.RenameFile.NewURI), "file://")
				affectedFiles[oldPath] = true
				affectedFiles[newPath] = true
			}
		}
	}

	// Then execute Command (per LSP spec: edit first, then command)
	if matched.Command != nil {
		_, err := client.ExecuteCommand(ctx, protocol.ExecuteCommandParams{
			Command:   matched.Command.Command,
			Arguments: matched.Command.Arguments,
		})
		if err != nil {
			return "", fmt.Errorf("failed to execute command %q: %v", matched.Command.Command, err)
		}
	}

	// Notify the LSP about changed files (headless mode has no file watcher)
	for path := range affectedFiles {
		if client.IsFileOpen(path) {
			if err := client.NotifyChange(ctx, path); err != nil {
				toolsLogger.Warn("Failed to notify change for %s: %v", path, err)
			}
		}
	}

	// Build summary
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Successfully executed code action: %s\n", matched.Title))
	if len(affectedFiles) > 0 {
		summary.WriteString("Modified files:\n")
		for path := range affectedFiles {
			summary.WriteString(fmt.Sprintf("  - %s\n", path))
		}
	}

	return summary.String(), nil
}
