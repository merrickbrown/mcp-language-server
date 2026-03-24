package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/isaacphi/mcp-language-server/internal/lsp"
	"github.com/isaacphi/mcp-language-server/internal/protocol"
)

// ExecuteWorkspaceCommand executes a workspace/executeCommand request.
// The arguments parameter is a JSON array string that gets parsed to []json.RawMessage.
func ExecuteWorkspaceCommand(ctx context.Context, client *lsp.Client, command string, arguments string) (string, error) {
	params := protocol.ExecuteCommandParams{
		Command: command,
	}

	// Parse arguments JSON array if provided
	if arguments != "" {
		var args []json.RawMessage
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("failed to parse arguments as JSON array: %v", err)
		}
		params.Arguments = args
	}

	result, err := client.ExecuteCommand(ctx, params)
	if err != nil {
		return "", fmt.Errorf("failed to execute command %q: %v", command, err)
	}

	// Format the result
	if result == nil {
		return fmt.Sprintf("Command %q executed successfully.", command), nil
	}

	resultJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf("Command %q executed. Result: %v", command, result), nil
	}

	return fmt.Sprintf("Command %q result:\n%s", command, string(resultJSON)), nil
}
