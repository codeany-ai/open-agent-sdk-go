package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

const globMaxResults = 100

// GlobTool finds files matching glob patterns.
type GlobTool struct{}

func NewGlobTool() *GlobTool { return &GlobTool{} }

func (t *GlobTool) Name() string { return "Glob" }

func (t *GlobTool) Description() string {
	return `Fast file pattern matching tool. Supports glob patterns like "**/*.js" or "src/**/*.ts".
Returns matching file paths sorted by modification time. Use this to find files by name patterns.`
}

func (t *GlobTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "The glob pattern to match files against",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The directory to search in (defaults to working directory)",
			},
		},
		Required: []string{"pattern"},
	}
}

func (t *GlobTool) IsConcurrencySafe(input map[string]interface{}) bool { return true }
func (t *GlobTool) IsReadOnly(input map[string]interface{}) bool        { return true }

type fileWithTime struct {
	path    string
	modTime time.Time
}

func (t *GlobTool) Call(ctx context.Context, input map[string]interface{}, tCtx *types.ToolUseContext) (*types.ToolResult, error) {
	pattern, _ := input["pattern"].(string)
	if pattern == "" {
		return &types.ToolResult{IsError: true, Error: "pattern is required"}, nil
	}

	searchPath, _ := input["path"].(string)
	if searchPath == "" && tCtx != nil {
		searchPath = tCtx.WorkingDir
	}
	if searchPath == "" {
		searchPath = "."
	}

	start := time.Now()
	var matches []fileWithTime

	err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip hidden directories (except the root)
		name := info.Name()
		if info.IsDir() && strings.HasPrefix(name, ".") && path != searchPath {
			return filepath.SkipDir
		}
		// Skip node_modules
		if info.IsDir() && name == "node_modules" {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		// Match against pattern
		relPath, _ := filepath.Rel(searchPath, path)
		matched, _ := filepath.Match(pattern, relPath)
		if !matched {
			// Try matching just the filename
			matched, _ = filepath.Match(pattern, name)
		}
		if !matched && strings.Contains(pattern, "**") {
			// Simple doublestar matching
			matched = simpleDoublestarMatch(pattern, relPath)
		}

		if matched {
			matches = append(matches, fileWithTime{path: relPath, modTime: info.ModTime()})
		}
		return nil
	})

	if err != nil && err != context.Canceled {
		return &types.ToolResult{IsError: true, Error: err.Error()}, nil
	}

	// Sort by modification time (newest first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime.After(matches[j].modTime)
	})

	truncated := false
	if len(matches) > globMaxResults {
		matches = matches[:globMaxResults]
		truncated = true
	}

	filenames := make([]string, len(matches))
	for i, m := range matches {
		filenames[i] = m.path
	}

	duration := time.Since(start).Milliseconds()

	text := strings.Join(filenames, "\n")
	if truncated {
		text += fmt.Sprintf("\n\n(showing %d of many matches, results truncated)", globMaxResults)
	}
	if text == "" {
		text = "No files found matching pattern: " + pattern
	}

	return &types.ToolResult{
		Data: map[string]interface{}{
			"filenames":  filenames,
			"durationMs": duration,
			"numFiles":   len(filenames),
			"truncated":  truncated,
		},
		Content: []types.ContentBlock{{
			Type: types.ContentBlockText,
			Text: text,
		}},
	}, nil
}

// simpleDoublestarMatch handles basic ** glob patterns.
func simpleDoublestarMatch(pattern, path string) bool {
	// Convert ** pattern to a simple prefix/suffix match
	parts := strings.SplitN(pattern, "**", 2)
	if len(parts) != 2 {
		return false
	}

	prefix := parts[0]
	suffix := parts[1]

	// Remove leading/trailing separators
	suffix = strings.TrimPrefix(suffix, "/")
	suffix = strings.TrimPrefix(suffix, string(filepath.Separator))

	if prefix != "" && !strings.HasPrefix(path, prefix) {
		return false
	}

	if suffix == "" {
		return true
	}

	// Match suffix against the path (could be nested)
	pathWithoutPrefix := path
	if prefix != "" {
		pathWithoutPrefix = strings.TrimPrefix(path, prefix)
	}

	// Check if any part of the remaining path matches the suffix pattern
	matched, _ := filepath.Match(suffix, filepath.Base(pathWithoutPrefix))
	return matched
}
