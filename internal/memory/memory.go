// Package memory handles GEMINI.md discovery and updates.
package memory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/gitignore"
	"github.com/puremachinery/gemini-cli-go/internal/patterns"
	"github.com/puremachinery/gemini-cli-go/internal/storage"
)

const (
	// DefaultContextFilename is the standard GEMINI.md filename.
	DefaultContextFilename = "GEMINI.md"
	// MemorySectionHeader is where memories are appended.
	MemorySectionHeader = "## Gemini Added Memories"
)

// State tracks the current loaded memory content.
type State struct {
	WorkspaceRoot string
	FileName      string
	Content       string
	FilePaths     []string
}

// Snapshot is a read-only memory view.
type Snapshot struct {
	Content   string
	FilePaths []string
}

// NewState initializes memory state for a workspace.
func NewState(workspaceRoot string) *State {
	return &State{
		WorkspaceRoot: workspaceRoot,
		FileName:      DefaultContextFilename,
	}
}

// Refresh reloads hierarchical memory.
func (s *State) Refresh() error {
	if s == nil {
		return errors.New("memory state is nil")
	}
	name := strings.TrimSpace(s.FileName)
	if name == "" {
		name = DefaultContextFilename
	}
	snapshot, err := LoadHierarchy(s.WorkspaceRoot, name)
	if err != nil {
		return err
	}
	s.Content = snapshot.Content
	s.FilePaths = snapshot.FilePaths
	s.FileName = name
	return nil
}

// GlobalPath returns the global GEMINI.md path.
func (s *State) GlobalPath() string {
	name := DefaultContextFilename
	if s != nil {
		if trimmed := strings.TrimSpace(s.FileName); trimmed != "" {
			name = trimmed
		}
	}
	return filepath.Join(storage.GlobalGeminiDir(), name)
}

type fileEntry struct {
	path    string
	content string
}

// LoadHierarchy discovers GEMINI.md files and concatenates their content.
func LoadHierarchy(workspaceRoot, fileName string) (Snapshot, error) {
	name := strings.TrimSpace(fileName)
	if name == "" {
		name = DefaultContextFilename
	}

	var entries []fileEntry
	seen := map[string]struct{}{}
	addEntry := func(path, content string) {
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		entries = append(entries, fileEntry{path: path, content: content})
	}

	globalPath := filepath.Join(storage.GlobalGeminiDir(), name)
	if content, ok, err := readOptionalFile(globalPath); err != nil {
		return Snapshot{}, err
	} else if ok {
		addEntry(globalPath, content)
	}

	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return Snapshot{
			Content:   concatenate(entries, workspaceRoot),
			FilePaths: collectPaths(entries),
		}, nil
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}

	stopDir := findGitRoot(rootAbs)
	if stopDir == "" {
		stopDir = rootAbs
	}
	upwardDirs := collectUpwardDirs(rootAbs, stopDir)
	for _, dir := range upwardDirs {
		path := filepath.Join(dir, name)
		if content, ok, err := readOptionalFile(path); err != nil {
			return Snapshot{}, err
		} else if ok {
			addEntry(path, content)
		}
	}

	downwardPaths, err := findDownwardFiles(rootAbs, name)
	if err != nil {
		return Snapshot{}, err
	}
	for _, path := range downwardPaths {
		if content, ok, err := readOptionalFile(path); err != nil {
			return Snapshot{}, err
		} else if ok {
			addEntry(path, content)
		}
	}

	return Snapshot{
		Content:   concatenate(entries, rootAbs),
		FilePaths: collectPaths(entries),
	}, nil
}

func collectPaths(entries []fileEntry) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.path)
	}
	return paths
}

func readOptionalFile(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return string(data), true, nil
	}
	if os.IsNotExist(err) {
		return "", false, nil
	}
	return "", false, err
}

func findGitRoot(start string) string {
	if start == "" {
		return ""
	}
	dir := filepath.Clean(start)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func collectUpwardDirs(start, stop string) []string {
	if start == "" {
		return nil
	}
	stop = filepath.Clean(stop)
	dirs := []string{}
	dir := filepath.Clean(start)
	for {
		dirs = append(dirs, dir)
		if dir == stop || dir == filepath.Dir(dir) {
			break
		}
		dir = filepath.Dir(dir)
	}
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs
}

func findDownwardFiles(root, fileName string) ([]string, error) {
	var files []string
	ignoreRules, hasNegations, err := loadGeminiIgnoreRules(root)
	if err != nil {
		return nil, err
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if isDefaultExcludedDir(d.Name()) {
				return filepath.SkipDir
			}
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if isIgnored(rel, d.IsDir(), ignoreRules) {
			if d.IsDir() {
				if !hasNegations {
					return filepath.SkipDir
				}
				return nil
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == fileName {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	files = filterGitIgnored(root, files, ignoreRules)
	sortStrings(files)
	return files, nil
}

func isDefaultExcludedDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "bin", ".upstream":
		return true
	default:
		return false
	}
}

func filterGitIgnored(rootAbs string, files []string, ignoreRules []ignoreRule) []string {
	if len(files) == 0 {
		return files
	}
	ignored, ok := gitignore.IgnoredSet(rootAbs, files)
	out := make([]string, 0, len(files))
	for _, path := range files {
		rel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			out = append(out, path)
			continue
		}
		rel = filepath.ToSlash(rel)
		baseIgnored := false
		if ok {
			if _, skip := ignored[rel]; skip {
				baseIgnored = true
			}
		}
		if applyIgnoreRules(baseIgnored, rel, false, ignoreRules) {
			continue
		}
		out = append(out, path)
	}
	return out
}

func concatenate(entries []fileEntry, workspaceRoot string) string {
	var blocks []string
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry.content)
		if trimmed == "" {
			continue
		}
		displayPath := entry.path
		if filepath.IsAbs(displayPath) && workspaceRoot != "" {
			if rel, err := filepath.Rel(workspaceRoot, displayPath); err == nil {
				displayPath = rel
			}
		}
		displayPath = filepath.ToSlash(displayPath)
		blocks = append(blocks, fmt.Sprintf(
			"--- Context from: %s ---\n%s\n--- End of Context from: %s ---",
			displayPath,
			trimmed,
			displayPath,
		))
	}
	return strings.Join(blocks, "\n\n")
}

func loadGeminiIgnoreRules(root string) ([]ignoreRule, bool, error) {
	path := filepath.Join(root, ".geminiignore")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	rawRules := parseIgnoreRules(string(data))
	if len(rawRules) == 0 {
		return nil, false, nil
	}
	return compileRules(rawRules)
}

type rawIgnoreRule struct {
	negate  bool
	pattern string
}

func parseIgnoreRules(content string) []rawIgnoreRule {
	lines := strings.Split(content, "\n")
	rules := make([]rawIgnoreRule, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = strings.TrimSpace(strings.TrimPrefix(line, "!"))
			if line == "" {
				continue
			}
		}
		line = filepath.ToSlash(line)
		anchored := strings.HasPrefix(line, "/")
		if anchored {
			line = strings.TrimPrefix(line, "/")
		}
		dirOnly := strings.HasSuffix(line, "/")
		if dirOnly {
			line = strings.TrimSuffix(line, "/")
			if line == "" {
				continue
			}
		}
		if !anchored && !strings.Contains(line, "/") {
			line = "**/" + line
		}
		if dirOnly {
			line += "/**"
		}
		rules = append(rules, rawIgnoreRule{
			negate:  negate,
			pattern: line,
		})
	}
	return rules
}

type ignoreRule struct {
	negate  bool
	matcher patterns.Matcher
}

func compileRules(raw []rawIgnoreRule) ([]ignoreRule, bool, error) {
	rules := make([]ignoreRule, 0, len(raw))
	hasNegations := false
	for _, rule := range raw {
		if rule.pattern == "" {
			continue
		}
		regex, err := patterns.GlobToRegex(rule.pattern)
		if err != nil {
			return nil, false, err
		}
		rules = append(rules, ignoreRule{negate: rule.negate, matcher: patterns.Matcher{Pattern: rule.pattern, Regex: regex}})
		if rule.negate {
			hasNegations = true
		}
	}
	return rules, hasNegations, nil
}

func isIgnored(path string, isDir bool, rules []ignoreRule) bool {
	return applyIgnoreRules(false, path, isDir, rules)
}

func applyIgnoreRules(baseIgnored bool, path string, isDir bool, rules []ignoreRule) bool {
	if len(rules) == 0 {
		return baseIgnored
	}
	normalized := strings.Trim(path, "/")
	if normalized == "" {
		return baseIgnored
	}
	parts := strings.Split(normalized, "/")
	ignored := baseIgnored
	for _, rule := range rules {
		if !ruleApplies(rule, normalized, isDir, parts) {
			continue
		}
		if rule.negate {
			ignored = false
		} else {
			ignored = true
		}
	}
	return ignored
}

func ruleMatches(rule ignoreRule, path string, isDir bool) bool {
	if rule.matcher.Regex.MatchString(path) {
		return true
	}
	if isDir {
		return rule.matcher.Regex.MatchString(path + "/")
	}
	return false
}

func ruleApplies(rule ignoreRule, path string, isDir bool, parts []string) bool {
	if ruleMatches(rule, path, isDir) {
		return true
	}
	if rule.negate {
		return false
	}
	if len(parts) < 2 {
		return false
	}
	for i := 1; i < len(parts); i++ {
		parent := strings.Join(parts[:i], "/")
		if ruleMatches(rule, parent, true) {
			return true
		}
	}
	return false
}

func sortStrings(items []string) {
	if len(items) < 2 {
		return
	}
	sort.Slice(items, func(i, j int) bool {
		return filepath.ToSlash(items[i]) < filepath.ToSlash(items[j])
	})
}

// AddMemoryEntry appends a memory entry to the global GEMINI.md file.
func AddMemoryEntry(fact, filePath string) error {
	fact = strings.TrimSpace(fact)
	if fact == "" {
		return errors.New("memory fact is empty")
	}
	return storage.WithFileLock(filePath, func() error {
		current := ""
		if data, err := os.ReadFile(filePath); err == nil {
			current = string(data)
		} else if !os.IsNotExist(err) {
			return err
		}
		newContent := computeNewContent(current, fact)
		if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
			return err
		}
		perm := os.FileMode(0o644)
		if storage.IsGlobalGeminiPath(filePath) {
			perm = 0o600
		}
		return storage.WriteFileAtomic(filePath, []byte(newContent), perm)
	})
}

func computeNewContent(currentContent, fact string) string {
	processedText := normalizeFact(fact)
	newMemoryItem := fmt.Sprintf("- %s", processedText)

	headerIndex := strings.Index(currentContent, MemorySectionHeader)
	if headerIndex == -1 {
		separator := ensureNewlineSeparation(currentContent)
		return currentContent + separator + MemorySectionHeader + "\n" + newMemoryItem + "\n"
	}

	startOfSectionContent := headerIndex + len(MemorySectionHeader)
	endOfSectionIndex := strings.Index(currentContent[startOfSectionContent:], "\n## ")
	if endOfSectionIndex != -1 {
		endOfSectionIndex += startOfSectionContent
	} else {
		endOfSectionIndex = len(currentContent)
	}

	beforeSection := strings.TrimRight(currentContent[:startOfSectionContent], "\r\n")
	sectionContent := strings.TrimRight(currentContent[startOfSectionContent:endOfSectionIndex], "\r\n")
	afterSection := currentContent[endOfSectionIndex:]

	sectionContent = sectionContent + "\n" + newMemoryItem
	updated := fmt.Sprintf("%s\n%s\n%s", beforeSection, strings.TrimLeft(sectionContent, "\r\n"), afterSection)
	updated = strings.TrimRight(updated, "\r\n")
	return updated + "\n"
}

func normalizeFact(fact string) string {
	processed := strings.TrimSpace(fact)
	processed = strings.TrimLeft(processed, "- \t")
	return strings.TrimSpace(processed)
}

func ensureNewlineSeparation(currentContent string) string {
	if currentContent == "" {
		return ""
	}
	if strings.HasSuffix(currentContent, "\n\n") || strings.HasSuffix(currentContent, "\r\n\r\n") {
		return ""
	}
	if strings.HasSuffix(currentContent, "\n") || strings.HasSuffix(currentContent, "\r\n") {
		return "\n"
	}
	return "\n\n"
}
