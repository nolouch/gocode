package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GlobTool finds files matching a glob pattern, sorted by modification time.
type GlobTool struct{}

func (t *GlobTool) ID() string { return "glob" }
func (t *GlobTool) Description() string {
	return "Find files matching a glob pattern (e.g. '**/*.go'). Returns paths sorted by modification time (newest first)."
}
func (t *GlobTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob pattern, e.g. '**/*.go' or 'src/*.ts'"},
			"path":    map[string]any{"type": "string", "description": "Base directory to search from (defaults to working directory)"},
		},
		"required": []string{"pattern"},
	}
}

func (t *GlobTool) Execute(ctx Context, args map[string]any) (Result, error) {
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return Result{IsError: true, Output: "glob requires a 'pattern'"}, nil
	}

	base, _ := args["path"].(string)
	if base == "" {
		base = ctx.WorkDir
	} else if !filepath.IsAbs(base) {
		base = filepath.Join(ctx.WorkDir, base)
	}

	// Walk and collect matches
	type fileEntry struct {
		path    string
		modTime int64
	}
	var matches []fileEntry

	err := filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if d.IsDir() {
			// Skip hidden dirs (except base itself)
			if p != base && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		matched, err := filepath.Match(pattern, rel)
		if err != nil {
			return err
		}
		// Also try matching just the filename for simple patterns
		if !matched {
			matched, _ = filepath.Match(pattern, d.Name())
		}
		// Handle ** patterns by checking each path segment
		if !matched && strings.Contains(pattern, "**") {
			matched = matchDoubleGlob(pattern, rel)
		}
		if matched {
			info, _ := d.Info()
			var mt int64
			if info != nil {
				mt = info.ModTime().UnixNano()
			}
			matches = append(matches, fileEntry{path: rel, modTime: mt})
		}
		return nil
	})
	if err != nil {
		return Result{IsError: true, Output: err.Error()}, nil
	}

	// Sort by modification time, newest first
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime > matches[j].modTime
	})

	if len(matches) == 0 {
		return Result{Output: "No files found matching: " + pattern, Title: pattern}, nil
	}

	var sb strings.Builder
	for _, m := range matches {
		sb.WriteString(m.path)
		sb.WriteByte('\n')
	}
	out := strings.TrimRight(sb.String(), "\n")
	return Result{
		Output: out,
		Title:  fmt.Sprintf("%s (%d files)", pattern, len(matches)),
	}, nil
}

// matchDoubleGlob handles patterns with ** by splitting on ** and checking prefix/suffix.
func matchDoubleGlob(pattern, path string) bool {
	parts := strings.SplitN(pattern, "**", 2)
	if len(parts) != 2 {
		return false
	}
	prefix := filepath.ToSlash(parts[0])
	suffix := filepath.ToSlash(parts[1])
	p := filepath.ToSlash(path)

	if prefix != "" && !strings.HasPrefix(p, prefix) {
		return false
	}
	if suffix != "" {
		// suffix may start with /, trim it
		suffix = strings.TrimPrefix(suffix, "/")
		matched, _ := filepath.Match(suffix, filepath.Base(p))
		if !matched {
			// try full suffix match
			matched = strings.HasSuffix(p, strings.TrimPrefix(suffix, "*"))
		}
		return matched
	}
	return true
}
