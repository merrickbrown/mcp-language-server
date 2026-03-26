package tools

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/isaacphi/mcp-language-server/internal/lsp"
	"github.com/isaacphi/mcp-language-server/internal/protocol"
)

// symbolMatches checks if a workspace symbol result matches the query.
// Handles Clojure conventions (ns/name) and general languages (Type.Method).
func symbolMatches(symbolName, query string) bool {
	if strings.EqualFold(symbolName, query) {
		return true
	}

	// For Clojure: query "ns/name" should match symbol "ns/name" or just "name"
	if strings.Contains(query, "/") {
		parts := strings.SplitN(query, "/", 2)
		unqualified := parts[len(parts)-1]
		if strings.EqualFold(symbolName, unqualified) {
			return true
		}
	}

	// For general languages: query "Type.Method" should match symbol "Method"
	if strings.Contains(query, ".") {
		parts := strings.Split(query, ".")
		methodName := parts[len(parts)-1]
		if strings.EqualFold(symbolName, methodName) {
			return true
		}
	}

	// Substring match: symbol "collage.core/subsystems" contains query "subsystems"
	if strings.Contains(strings.ToLower(symbolName), strings.ToLower(query)) {
		return true
	}

	return false
}

func FindReferences(ctx context.Context, client *lsp.Client, symbolName string) (string, error) {
	// Get context lines from environment variable
	contextLines := 5
	if envLines := os.Getenv("LSP_CONTEXT_LINES"); envLines != "" {
		if val, err := strconv.Atoi(envLines); err == nil && val >= 0 {
			contextLines = val
		}
	}

	// First get the symbol location via workspace/symbol
	symbolResult, err := client.Symbol(ctx, protocol.WorkspaceSymbolParams{
		Query: symbolName,
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch symbol: %v", err)
	}

	results, err := symbolResult.Results()
	if err != nil {
		return "", fmt.Errorf("failed to parse results: %v", err)
	}

	var allReferences []string
	for _, symbol := range results {
		if !symbolMatches(symbol.GetName(), symbolName) {
			continue
		}

		// Get the location of the symbol
		loc := symbol.GetLocation()

		err := client.SyncFile(ctx, loc.URI.Path())
		if err != nil {
			toolsLogger.Error("Error opening file: %v", err)
			continue
		}

		// Use LSP references request
		refsParams := protocol.ReferenceParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: loc.URI,
				},
				Position: loc.Range.Start,
			},
			Context: protocol.ReferenceContext{
				IncludeDeclaration: false,
			},
		}
		refs, err := client.References(ctx, refsParams)
		if err != nil {
			return "", fmt.Errorf("failed to get references: %v", err)
		}

		// Group references by file
		refsByFile := make(map[protocol.DocumentUri][]protocol.Location)
		for _, ref := range refs {
			refsByFile[ref.URI] = append(refsByFile[ref.URI], ref)
		}

		// Get sorted list of URIs
		uris := make([]string, 0, len(refsByFile))
		for uri := range refsByFile {
			uris = append(uris, string(uri))
		}
		sort.Strings(uris)

		// Process each file's references in sorted order
		for _, uriStr := range uris {
			uri := protocol.DocumentUri(uriStr)
			fileRefs := refsByFile[uri]
			filePath := strings.TrimPrefix(uriStr, "file://")

			// Format file header
			fileInfo := fmt.Sprintf("---\n\n%s\nReferences in File: %d\n",
				filePath,
				len(fileRefs),
			)

			// Format locations with context
			fileContent, err := os.ReadFile(filePath)
			if err != nil {
				allReferences = append(allReferences, fileInfo+"\nError reading file: "+err.Error())
				continue
			}

			lines := strings.Split(string(fileContent), "\n")

			// Track reference locations for header display
			var locStrings []string
			for _, ref := range fileRefs {
				locStr := fmt.Sprintf("L%d:C%d",
					ref.Range.Start.Line+1,
					ref.Range.Start.Character+1)
				locStrings = append(locStrings, locStr)
			}

			// Collect lines to display using the utility function
			linesToShow, err := GetLineRangesToDisplay(ctx, client, fileRefs, len(lines), contextLines)
			if err != nil {
				continue
			}

			// Convert to line ranges using the utility function
			lineRanges := ConvertLinesToRanges(linesToShow, len(lines))

			// Format with locations in header
			formattedOutput := fileInfo
			if len(locStrings) > 0 {
				formattedOutput += "At: " + strings.Join(locStrings, ", ") + "\n"
			}

			// Format the content with ranges
			formattedOutput += "\n" + FormatLinesWithRanges(lines, lineRanges)
			allReferences = append(allReferences, formattedOutput)
		}
	}

	if len(allReferences) == 0 {
		return fmt.Sprintf("No references found for symbol: %s", symbolName), nil
	}

	return strings.Join(allReferences, "\n"), nil
}
