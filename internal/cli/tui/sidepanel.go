package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nolouch/gocode/pkg/sdk"
)

const (
	defaultSidePanelWidth   = 40
	sidePanelMinWidth       = 28
	sidePanelMaxWidth       = 72
	sidePanelMinMainWidth   = 56
	sidePanelResizeStep     = 4
	sidePanelCollapseWidth  = 20
	sidePanelTreeMaxDepth   = 3
	sidePanelTreeMaxNodes   = 280
	sidePanelPreviewLines   = 8
	sidePanelPreviewMaxCols = 88
)

type sidePanelTab string

const (
	sidePanelTabFiles   sidePanelTab = "files"
	sidePanelTabReview  sidePanelTab = "review"
	sidePanelTabContext sidePanelTab = "context"
)

type fileTreeNode struct {
	Path  string
	Name  string
	Depth int
	IsDir bool
}

type reviewItem struct {
	Path      string
	Tool      string
	State     string
	Summary   string
	Preview   string
	UpdatedAt time.Time
}

func (m *Model) handleSidePanelKey(msg tea.KeyMsg) bool {
	switch strings.ToLower(msg.String()) {
	case "ctrl+1":
		m.sidePanelOpen = true
		m.sidePanelTab = sidePanelTabFiles
		m.relayout()
		return true
	case "ctrl+2":
		m.sidePanelOpen = true
		m.sidePanelTab = sidePanelTabReview
		m.relayout()
		return true
	case "ctrl+3":
		m.sidePanelOpen = true
		m.sidePanelTab = sidePanelTabContext
		m.relayout()
		return true
	case "ctrl+o":
		m.sidePanelOpen = !m.sidePanelOpen
		m.relayout()
		return true
	case "alt+,", "ctrl+left":
		if !m.sidePanelOpen {
			return true
		}
		m.sidePanelWidth -= sidePanelResizeStep
		if m.sidePanelWidth < sidePanelCollapseWidth {
			m.sidePanelOpen = false
		}
		m.relayout()
		return true
	case "alt+.", "ctrl+right":
		if !m.sidePanelOpen {
			m.sidePanelOpen = true
			if m.sidePanelWidth < sidePanelMinWidth {
				m.sidePanelWidth = sidePanelMinWidth
			}
		} else {
			m.sidePanelWidth += sidePanelResizeStep
		}
		m.relayout()
		return true
	default:
		return false
	}
}

func (m Model) effectiveSidePanelWidth() int {
	if !m.sidePanelOpen || m.width < 1 {
		return 0
	}
	w := m.sidePanelWidth
	if w < sidePanelMinWidth {
		w = sidePanelMinWidth
	}
	if w > sidePanelMaxWidth {
		w = sidePanelMaxWidth
	}
	maxPanel := m.width - sidePanelMinMainWidth
	if maxPanel < sidePanelMinWidth {
		return 0
	}
	if w > maxPanel {
		w = maxPanel
	}
	return w
}

func (m Model) mainContentWidth() int {
	w := m.width - m.effectiveSidePanelWidth()
	if w < 1 {
		return 1
	}
	return w
}

func (m Model) renderSidePanel(height int) string {
	panelW := m.effectiveSidePanelWidth()
	if panelW == 0 || height < 1 {
		return ""
	}

	innerW := panelW - 1
	if innerW < 1 {
		innerW = 1
	}
	tabRow := m.renderSidePanelTabs(innerW)
	tabH := lipgloss.Height(tabRow)
	bodyH := height - tabH
	if bodyH < 1 {
		bodyH = 1
	}

	var body string
	switch m.sidePanelTab {
	case sidePanelTabReview:
		body = m.renderReviewPanel(innerW, bodyH)
	case sidePanelTabContext:
		body = m.renderContextPanel(innerW, bodyH)
	default:
		body = m.renderFilesPanel(innerW, bodyH)
	}

	panel := lipgloss.JoinVertical(lipgloss.Left, tabRow, body)
	return StyleSidePanelBorder.Width(panelW).Height(height).Render(panel)
}

func (m Model) renderSidePanelTabs(width int) string {
	reviewCount := len(m.reviewByPath)
	total := totalTokenUsage(m.contextUsage)

	files := m.renderTab(sidePanelTabFiles, "Files")
	review := m.renderTab(sidePanelTabReview, fmt.Sprintf("Review %d", reviewCount))
	context := m.renderTab(sidePanelTabContext, fmt.Sprintf("Context %d", total))

	row := lipgloss.JoinHorizontal(lipgloss.Left, files, " ", review, " ", context)
	return StyleSidePanelTabs.Width(width).Render(truncatePanelLine(row, width))
}

func (m Model) renderTab(tab sidePanelTab, label string) string {
	if m.sidePanelTab == tab {
		return StyleSidePanelTabActive.Render(label)
	}
	return StyleSidePanelTabInactive.Render(label)
}

func (m Model) renderFilesPanel(width, height int) string {
	lines := []string{
		StyleSidePanelTitle.Render("Workspace"),
		StyleSidePanelMuted.Render(m.workDir),
		"",
	}
	if m.fileTreeErr != "" {
		lines = append(lines, StyleSidePanelError.Render(m.fileTreeErr))
		return renderPanelLines(lines, width, height)
	}
	if len(m.fileTreeNodes) == 0 {
		lines = append(lines, StyleSidePanelMuted.Render("No files found."))
		return renderPanelLines(lines, width, height)
	}

	for _, n := range m.fileTreeNodes {
		prefix := strings.Repeat("  ", n.Depth)
		name := n.Name
		if n.IsDir {
			name += "/"
		}
		marker := " "
		if _, ok := m.reviewByPath[n.Path]; ok && !n.IsDir {
			marker = "M"
		}
		line := fmt.Sprintf("%s %s%s", marker, prefix, name)
		if marker == "M" {
			line = StyleSidePanelChanged.Render(line)
		}
		lines = append(lines, line)
	}
	return renderPanelLines(lines, width, height)
}

func (m Model) renderReviewPanel(width, height int) string {
	lines := []string{
		StyleSidePanelTitle.Render("Review"),
		StyleSidePanelMuted.Render("Captured file changes from tool calls"),
		"",
	}
	items := m.sortedReviewItems()
	if len(items) == 0 {
		lines = append(lines, StyleSidePanelMuted.Render("No file changes yet."))
		return renderPanelLines(lines, width, height)
	}

	for _, item := range items {
		state := strings.ToUpper(item.State)
		if state == "" {
			state = "UNKNOWN"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s", state, item.Path))
		meta := strings.TrimSpace(item.Tool)
		if !item.UpdatedAt.IsZero() {
			if meta != "" {
				meta += " · "
			}
			meta += item.UpdatedAt.Format("15:04:05")
		}
		if meta != "" {
			lines = append(lines, StyleSidePanelMuted.Render("  "+meta))
		}
		if item.Summary != "" {
			lines = append(lines, "  "+item.Summary)
		}
		if item.Preview != "" {
			for _, line := range strings.Split(item.Preview, "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				lines = append(lines, StyleSidePanelMuted.Render("  "+line))
			}
		}
		lines = append(lines, "")
	}
	return renderPanelLines(lines, width, height)
}

func (m Model) renderContextPanel(width, height int) string {
	u := m.contextUsage
	total := totalTokenUsage(u)
	barWidth := width - 20
	if barWidth < 6 {
		barWidth = 6
	}

	lines := []string{
		StyleSidePanelTitle.Render("Context Usage"),
		StyleSidePanelMuted.Render(fmt.Sprintf("Assistant turns: %d", m.contextTurns)),
		"",
		fmt.Sprintf("Total      %d", total),
		"",
		fmt.Sprintf("Input      %d", u.Input),
		"  " + tokenBar(u.Input, total, barWidth),
		fmt.Sprintf("Output     %d", u.Output),
		"  " + tokenBar(u.Output, total, barWidth),
		fmt.Sprintf("Reasoning  %d", u.Reasoning),
		"  " + tokenBar(u.Reasoning, total, barWidth),
		fmt.Sprintf("CacheRead  %d", u.CacheRead),
		fmt.Sprintf("CacheWrite %d", u.CacheWrite),
	}
	return renderPanelLines(lines, width, height)
}

func (m *Model) rebuildSidePanel(msgs []*sdk.Message) {
	m.reviewByPath = make(map[string]reviewItem)
	m.contextUsage = sdk.TokenUsage{}
	m.contextTurns = 0
	m.stepFinishSeen = make(map[string]struct{})

	for _, msg := range msgs {
		if msg == nil || msg.Role != sdk.RoleAssistant {
			continue
		}
		m.contextTurns++
		addTokenUsage(&m.contextUsage, msg.Tokens)

		for _, part := range msg.Parts {
			if part.Type == sdk.PartTypeTool {
				m.applyToolPartToReview(part)
			}
			if part.Type == sdk.PartTypeStepFinish {
				m.stepFinishSeen[part.ID] = struct{}{}
			}
		}
	}
	m.refreshFileTree()
}

func (m *Model) applyStepFinishUsage(part sdk.Part) {
	if part.StepFinish == nil {
		return
	}
	if _, ok := m.stepFinishSeen[part.ID]; ok {
		return
	}
	m.stepFinishSeen[part.ID] = struct{}{}
	addTokenUsage(&m.contextUsage, part.StepFinish.Tokens)
}

func (m *Model) applyToolPartToReview(part sdk.Part) {
	if part.Tool == nil {
		return
	}
	if !isMutatingTool(part.Tool.Tool) {
		return
	}

	paths := extractToolPaths(m.workDir, part.Tool.Tool, part.Tool.Input)
	if len(paths) == 0 {
		paths = []string{"(workspace)"}
	}

	updatedAt := time.Now()
	if !part.Tool.EndAt.IsZero() {
		updatedAt = part.Tool.EndAt
	} else if !part.Tool.StartAt.IsZero() {
		updatedAt = part.Tool.StartAt
	}

	for _, p := range paths {
		item := m.reviewByPath[p]
		item.Path = p
		item.Tool = part.Tool.Tool
		item.State = string(part.Tool.State)
		item.Summary = summarizeToolPart(part.Tool)
		item.UpdatedAt = updatedAt
		if (part.Tool.State == sdk.ToolStateCompleted || part.Tool.State == sdk.ToolStateError) && p != "(workspace)" {
			item.Preview = m.gitDiffPreview(p)
		}
		m.reviewByPath[p] = item
	}
}

func (m *Model) refreshFileTree() {
	nodes, err := buildFileTreeNodes(m.workDir, sidePanelTreeMaxDepth, sidePanelTreeMaxNodes)
	if err != nil {
		m.fileTreeErr = err.Error()
		m.fileTreeNodes = nil
		return
	}
	m.fileTreeErr = ""
	m.fileTreeNodes = nodes
}

func (m Model) sortedReviewItems() []reviewItem {
	out := make([]reviewItem, 0, len(m.reviewByPath))
	for _, item := range m.reviewByPath {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Path < out[j].Path
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (m Model) gitDiffPreview(path string) string {
	if path == "" || path == "(workspace)" {
		return ""
	}
	cmd := exec.Command("git", "-C", m.workDir, "diff", "--", path)
	out, err := cmd.Output()
	if err != nil {
		// git not installed, not a repo, or path not tracked
		if len(out) > 0 {
			fmt.Fprintf(os.Stderr, "sidepanel: git diff %s: %v\n", path, err)
		}
		return ""
	}
	diff := strings.TrimSpace(string(out))
	if diff == "" {
		return ""
	}
	lines := strings.Split(diff, "\n")
	if len(lines) > sidePanelPreviewLines {
		lines = lines[:sidePanelPreviewLines]
	}
	for i := range lines {
		lines[i] = truncatePanelLine(lines[i], sidePanelPreviewMaxCols)
	}
	return strings.Join(lines, "\n")
}

func buildFileTreeNodes(root string, maxDepth, maxNodes int) ([]fileTreeNode, error) {
	root = filepath.Clean(root)
	if root == "" {
		return nil, nil
	}
	var nodes []fileTreeNode
	var walk func(absDir, relDir string, depth int) error

	walk = func(absDir, relDir string, depth int) error {
		if len(nodes) >= maxNodes || depth >= maxDepth {
			return nil
		}
		entries, err := os.ReadDir(absDir)
		if err != nil {
			// Log permission errors or other filesystem issues
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "sidepanel: read dir %s: %v\n", absDir, err)
			}
			return nil
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir() != entries[j].IsDir() {
				return entries[i].IsDir()
			}
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		})
		for _, entry := range entries {
			if shouldSkipTreeEntry(entry.Name()) {
				continue
			}
			rel := entry.Name()
			if relDir != "" {
				rel = filepath.Join(relDir, rel)
			}
			rel = filepath.ToSlash(rel)
			nodes = append(nodes, fileTreeNode{
				Path:  rel,
				Name:  entry.Name(),
				Depth: depth,
				IsDir: entry.IsDir(),
			})
			if len(nodes) >= maxNodes {
				return nil
			}
			if entry.IsDir() && depth < maxDepth {
				if err := walk(filepath.Join(absDir, entry.Name()), rel, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := walk(root, "", 0); err != nil {
		return nil, err
	}
	return nodes, nil
}

func shouldSkipTreeEntry(name string) bool {
	switch name {
	case ".git", ".gocode", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func isMutatingTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "write", "write_file", "edit", "apply_patch", "todo_write":
		return true
	default:
		return false
	}
}

func extractToolPaths(workDir, toolName string, input map[string]any) []string {
	if len(input) == 0 {
		return nil
	}
	var out []string
	toolName = strings.ToLower(strings.TrimSpace(toolName))

	if p := mapString(input, "path", "file"); p != "" {
		out = append(out, normalizeReviewPath(workDir, p))
	}

	if paths, ok := input["paths"].([]any); ok {
		for _, p := range paths {
			if s, ok := p.(string); ok {
				out = append(out, normalizeReviewPath(workDir, s))
			}
		}
	}

	if toolName == "apply_patch" {
		if patch, ok := input["patch"].(string); ok {
			for _, p := range parsePatchPaths(patch) {
				out = append(out, normalizeReviewPath(workDir, p))
			}
		}
	}

	seen := make(map[string]struct{})
	dedup := make([]string, 0, len(out))
	for _, p := range out {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		dedup = append(dedup, p)
	}
	return dedup
}

func parsePatchPaths(patch string) []string {
	var out []string
	lines := strings.Split(patch, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "*** Update File:"):
			out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "*** Update File:")))
		case strings.HasPrefix(line, "*** Add File:"):
			out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "*** Add File:")))
		case strings.HasPrefix(line, "*** Delete File:"):
			out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File:")))
		case strings.HasPrefix(line, "+++ b/"):
			out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "+++ b/")))
		case strings.HasPrefix(line, "--- a/"):
			out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "--- a/")))
		}
	}
	return out
}

func mapString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func normalizeReviewPath(workDir, path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" || path == "/dev/null" {
		return ""
	}
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(workDir, path); err == nil && !strings.HasPrefix(rel, "..") {
			path = rel
		}
	}
	path = filepath.ToSlash(filepath.Clean(path))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}

func summarizeToolPart(tp *sdk.ToolPart) string {
	if tp == nil {
		return ""
	}
	if tp.State == sdk.ToolStateError && strings.TrimSpace(tp.Error) != "" {
		return truncatePanelLine(strings.TrimSpace(firstLine(tp.Error)), 88)
	}
	if strings.TrimSpace(tp.Output) != "" {
		return truncatePanelLine(strings.TrimSpace(firstLine(tp.Output)), 88)
	}
	return ""
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func addTokenUsage(dst *sdk.TokenUsage, inc sdk.TokenUsage) {
	dst.Input += inc.Input
	dst.Output += inc.Output
	dst.Reasoning += inc.Reasoning
	dst.CacheRead += inc.CacheRead
	dst.CacheWrite += inc.CacheWrite
}

func totalTokenUsage(u sdk.TokenUsage) int {
	return u.Input + u.Output + u.Reasoning + u.CacheRead + u.CacheWrite
}

func tokenBar(v, total, width int) string {
	if width < 1 {
		return ""
	}
	if total <= 0 || v <= 0 {
		return strings.Repeat("-", width)
	}
	fill := int(float64(v) / float64(total) * float64(width))
	if fill < 0 {
		fill = 0
	}
	if fill > width {
		fill = width
	}
	return strings.Repeat("=", fill) + strings.Repeat("-", width-fill)
}

func renderPanelLines(lines []string, width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		rendered = append(rendered, truncatePanelLine(line, width))
	}
	if len(rendered) > height {
		more := len(rendered) - height + 1
		rendered = rendered[:height-1]
		rendered = append(rendered, truncatePanelLine(fmt.Sprintf("... %d more", more), width))
	}
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Render(strings.Join(rendered, "\n"))
}

// ansiEscapeRegex matches ANSI escape sequences
var ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func truncatePanelLine(s string, width int) string {
	if width < 1 {
		return ""
	}
	// Strip ANSI codes first for accurate width calculation and truncation
	plain := ansiEscapeRegex.ReplaceAllString(s, "")
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 3 {
		r := []rune(plain)
		if len(r) <= width {
			return s
		}
		return string(r[:width])
	}
	runes := []rune(plain)
	cut := width - 3
	if cut < 0 {
		cut = 0
	}
	if len(runes) > cut {
		runes = runes[:cut]
	}
	return string(runes) + "..."
}
