package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/isaacphi/mcp-language-server/internal/lsp"
	"github.com/isaacphi/mcp-language-server/internal/protocol"
)

// hoverResult is a flexible representation of the LSP Hover response.
// The LSP spec allows contents to be MarkupContent | MarkedString | MarkedString[],
// but the generated Go types only handle MarkupContent. clojure-lsp returns
// MarkedString[], so we parse contents manually.
type hoverResult struct {
	Contents json.RawMessage `json:"contents"`
	Range    *protocol.Range `json:"range,omitempty"`
}

// markedString represents a MarkedString in the LSP spec.
// It can be a plain string or {language, value}.
type markedString struct {
	Language string `json:"language"`
	Value    string `json:"value"`
}

// extractHoverContents parses the flexible "contents" field into a string.
func extractHoverContents(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try MarkupContent: {"kind": "...", "value": "..."}
	var markup protocol.MarkupContent
	if err := json.Unmarshal(raw, &markup); err == nil && markup.Value != "" {
		return markup.Value
	}

	// Try MarkedString[]: [{"language": "...", "value": "..."}, ...] or ["...", ...]
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		var parts []string
		for _, item := range arr {
			// Try as {language, value}
			var ms markedString
			if err := json.Unmarshal(item, &ms); err == nil && ms.Value != "" {
				if ms.Language != "" {
					parts = append(parts, fmt.Sprintf("```%s\n%s\n```", ms.Language, ms.Value))
				} else {
					parts = append(parts, ms.Value)
				}
				continue
			}
			// Try as plain string
			var s string
			if err := json.Unmarshal(item, &s); err == nil {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n\n")
	}

	// Try single MarkedString: {"language": "...", "value": "..."}
	var ms markedString
	if err := json.Unmarshal(raw, &ms); err == nil && ms.Value != "" {
		if ms.Language != "" {
			return fmt.Sprintf("```%s\n%s\n```", ms.Language, ms.Value)
		}
		return ms.Value
	}

	// Try plain string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	return string(raw)
}

// GetHoverInfo retrieves hover information (type, documentation) for a symbol at the specified position
func GetHoverInfo(ctx context.Context, client *lsp.Client, filePath string, line, column int) (string, error) {
	err := client.SyncFile(ctx, filePath)
	if err != nil {
		return "", fmt.Errorf("could not open file: %v", err)
	}

	params := protocol.HoverParams{}
	position := protocol.Position{
		Line:      uint32(line - 1),
		Character: uint32(column - 1),
	}
	uri := protocol.DocumentUri("file://" + filePath)
	params.TextDocument = protocol.TextDocumentIdentifier{URI: uri}
	params.Position = position

	// Use raw JSON call to avoid the MarkupContent unmarshalling issue
	var rawResult json.RawMessage
	err = client.Call(ctx, "textDocument/hover", params, &rawResult)
	if err != nil {
		return "", fmt.Errorf("failed to get hover information: %v", err)
	}

	if rawResult == nil || string(rawResult) == "null" {
		lineText, err := ExtractTextFromLocation(protocol.Location{
			URI: uri,
			Range: protocol.Range{
				Start: protocol.Position{Line: position.Line, Character: 0},
				End:   protocol.Position{Line: position.Line + 1, Character: 0},
			},
		})
		if err != nil {
			toolsLogger.Warn("failed to extract line at position: %v", err)
		}
		return fmt.Sprintf("No hover information available for this position on the following line:\n%s", lineText), nil
	}

	var hover hoverResult
	if err := json.Unmarshal(rawResult, &hover); err != nil {
		return "", fmt.Errorf("failed to parse hover result: %v", err)
	}

	content := extractHoverContents(hover.Contents)
	if content == "" {
		return "No hover information available at this position.", nil
	}

	return content, nil
}
