package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeNewContentAddsHeader(t *testing.T) {
	got := computeNewContent("", "remember this")
	want := "## Gemini Added Memories\n- remember this\n"
	if got != want {
		t.Fatalf("unexpected content:\n%q\nwant:\n%q", got, want)
	}
}

func TestComputeNewContentInsertsIntoSection(t *testing.T) {
	current := "Intro\n\n## Gemini Added Memories\n- first\n\n## Other\nother"
	got := computeNewContent(current, "second")
	want := "Intro\n\n## Gemini Added Memories\n- first\n- second\n\n## Other\nother\n"
	if got != want {
		t.Fatalf("unexpected content:\n%q\nwant:\n%q", got, want)
	}
}

func TestLoadHierarchyOrdersFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	globalDir := filepath.Join(tmp, ".gemini")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global: %v", err)
	}
	globalPath := filepath.Join(globalDir, DefaultContextFilename)
	if err := os.WriteFile(globalPath, []byte("Global"), 0o644); err != nil {
		t.Fatalf("write global: %v", err)
	}

	repoRoot := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, DefaultContextFilename), []byte("Project"), 0o644); err != nil {
		t.Fatalf("write root: %v", err)
	}

	subDir := filepath.Join(repoRoot, "sub")
	childDir := filepath.Join(subDir, "child")
	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatalf("mkdir child: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, DefaultContextFilename), []byte("Sub"), 0o644); err != nil {
		t.Fatalf("write sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(childDir, DefaultContextFilename), []byte("Child"), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}

	snapshot, err := LoadHierarchy(subDir, DefaultContextFilename)
	if err != nil {
		t.Fatalf("LoadHierarchy: %v", err)
	}
	if len(snapshot.FilePaths) != 4 {
		t.Fatalf("expected 4 file paths, got %d", len(snapshot.FilePaths))
	}
	if snapshot.FilePaths[0] != globalPath {
		t.Fatalf("expected global path first, got %q", snapshot.FilePaths[0])
	}
	if snapshot.FilePaths[1] != filepath.Join(repoRoot, DefaultContextFilename) {
		t.Fatalf("expected root path second, got %q", snapshot.FilePaths[1])
	}
	if snapshot.FilePaths[2] != filepath.Join(subDir, DefaultContextFilename) {
		t.Fatalf("expected sub path third, got %q", snapshot.FilePaths[2])
	}
	if snapshot.FilePaths[3] != filepath.Join(childDir, DefaultContextFilename) {
		t.Fatalf("expected child path fourth, got %q", snapshot.FilePaths[3])
	}

	globalRel, err := filepath.Rel(subDir, globalPath)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}
	if !strings.Contains(snapshot.Content, "--- Context from: "+filepath.ToSlash(globalRel)+" ---") {
		t.Fatalf("expected global marker in content, got: %q", snapshot.Content)
	}
	if !strings.Contains(snapshot.Content, "Global") {
		t.Fatalf("expected global content, got: %q", snapshot.Content)
	}
	if !strings.Contains(snapshot.Content, "Project") {
		t.Fatalf("expected project content, got: %q", snapshot.Content)
	}
	if !strings.Contains(snapshot.Content, "Sub") {
		t.Fatalf("expected sub content, got: %q", snapshot.Content)
	}
	if !strings.Contains(snapshot.Content, "Child") {
		t.Fatalf("expected child content, got: %q", snapshot.Content)
	}
}

func TestLoadHierarchyRespectsGeminiIgnore(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	repoRoot := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".geminiignore"), []byte("ignored/\n"), 0o644); err != nil {
		t.Fatalf("write geminiignore: %v", err)
	}

	ignoredDir := filepath.Join(repoRoot, "ignored")
	if err := os.MkdirAll(ignoredDir, 0o755); err != nil {
		t.Fatalf("mkdir ignored: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ignoredDir, DefaultContextFilename), []byte("Ignored"), 0o644); err != nil {
		t.Fatalf("write ignored: %v", err)
	}

	okDir := filepath.Join(repoRoot, "ok")
	if err := os.MkdirAll(okDir, 0o755); err != nil {
		t.Fatalf("mkdir ok: %v", err)
	}
	okPath := filepath.Join(okDir, DefaultContextFilename)
	if err := os.WriteFile(okPath, []byte("Ok"), 0o644); err != nil {
		t.Fatalf("write ok: %v", err)
	}

	snapshot, err := LoadHierarchy(repoRoot, DefaultContextFilename)
	if err != nil {
		t.Fatalf("LoadHierarchy: %v", err)
	}
	for _, path := range snapshot.FilePaths {
		if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(ignoredDir, DefaultContextFilename)) {
			t.Fatalf("expected ignored path to be excluded, got %q", path)
		}
	}
	found := false
	for _, path := range snapshot.FilePaths {
		if filepath.ToSlash(path) == filepath.ToSlash(okPath) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ok path to be included, got %v", snapshot.FilePaths)
	}
}

func TestLoadHierarchyGeminiIgnoreNegationAndAnchoring(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	repoRoot := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	ignore := strings.Join([]string{
		"/dir/",
		"ignored/",
		"!ignored/keep/",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repoRoot, ".geminiignore"), []byte(ignore), 0o644); err != nil {
		t.Fatalf("write geminiignore: %v", err)
	}

	rootDir := filepath.Join(repoRoot, "dir")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, DefaultContextFilename), []byte("RootDir"), 0o644); err != nil {
		t.Fatalf("write root dir: %v", err)
	}

	nestedDir := filepath.Join(repoRoot, "other", "dir")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested dir: %v", err)
	}
	nestedPath := filepath.Join(nestedDir, DefaultContextFilename)
	if err := os.WriteFile(nestedPath, []byte("NestedDir"), 0o644); err != nil {
		t.Fatalf("write nested dir: %v", err)
	}

	ignoredDrop := filepath.Join(repoRoot, "ignored", "drop")
	if err := os.MkdirAll(ignoredDrop, 0o755); err != nil {
		t.Fatalf("mkdir ignored drop: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ignoredDrop, DefaultContextFilename), []byte("IgnoredDrop"), 0o644); err != nil {
		t.Fatalf("write ignored drop: %v", err)
	}

	ignoredKeep := filepath.Join(repoRoot, "ignored", "keep")
	if err := os.MkdirAll(ignoredKeep, 0o755); err != nil {
		t.Fatalf("mkdir ignored keep: %v", err)
	}
	keepPath := filepath.Join(ignoredKeep, DefaultContextFilename)
	if err := os.WriteFile(keepPath, []byte("IgnoredKeep"), 0o644); err != nil {
		t.Fatalf("write ignored keep: %v", err)
	}

	snapshot, err := LoadHierarchy(repoRoot, DefaultContextFilename)
	if err != nil {
		t.Fatalf("LoadHierarchy: %v", err)
	}
	for _, path := range snapshot.FilePaths {
		if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(rootDir, DefaultContextFilename)) {
			t.Fatalf("expected anchored ignore to exclude root dir file, got %q", path)
		}
		if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(ignoredDrop, DefaultContextFilename)) {
			t.Fatalf("expected ignored drop to be excluded, got %q", path)
		}
	}
	foundNested := false
	foundKeep := false
	for _, path := range snapshot.FilePaths {
		switch filepath.ToSlash(path) {
		case filepath.ToSlash(nestedPath):
			foundNested = true
		case filepath.ToSlash(keepPath):
			foundKeep = true
		}
	}
	if !foundNested {
		t.Fatalf("expected nested dir file to be included, got %v", snapshot.FilePaths)
	}
	if !foundKeep {
		t.Fatalf("expected negated keep file to be included, got %v", snapshot.FilePaths)
	}
}

func TestLoadHierarchyGeminiIgnoreNoSlashMatchesDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	repoRoot := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".geminiignore"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatalf("write geminiignore: %v", err)
	}

	ignoredDir := filepath.Join(repoRoot, "ignored")
	if err := os.MkdirAll(ignoredDir, 0o755); err != nil {
		t.Fatalf("mkdir ignored: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ignoredDir, DefaultContextFilename), []byte("Ignored"), 0o644); err != nil {
		t.Fatalf("write ignored: %v", err)
	}

	okDir := filepath.Join(repoRoot, "ok")
	if err := os.MkdirAll(okDir, 0o755); err != nil {
		t.Fatalf("mkdir ok: %v", err)
	}
	okPath := filepath.Join(okDir, DefaultContextFilename)
	if err := os.WriteFile(okPath, []byte("Ok"), 0o644); err != nil {
		t.Fatalf("write ok: %v", err)
	}

	snapshot, err := LoadHierarchy(repoRoot, DefaultContextFilename)
	if err != nil {
		t.Fatalf("LoadHierarchy: %v", err)
	}
	for _, path := range snapshot.FilePaths {
		if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(ignoredDir, DefaultContextFilename)) {
			t.Fatalf("expected no-slash ignore to exclude ignored dir file, got %q", path)
		}
	}
	found := false
	for _, path := range snapshot.FilePaths {
		if filepath.ToSlash(path) == filepath.ToSlash(okPath) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ok path to be included, got %v", snapshot.FilePaths)
	}
}

func TestLoadHierarchyGeminiIgnoreOverridesGitIgnore(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	repoRoot := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".gitignore"), []byte("secret/\n"), 0o644); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".geminiignore"), []byte("!secret/keep/\n"), 0o644); err != nil {
		t.Fatalf("write geminiignore: %v", err)
	}

	keepDir := filepath.Join(repoRoot, "secret", "keep")
	if err := os.MkdirAll(keepDir, 0o755); err != nil {
		t.Fatalf("mkdir keep: %v", err)
	}
	keepPath := filepath.Join(keepDir, DefaultContextFilename)
	if err := os.WriteFile(keepPath, []byte("Keep"), 0o644); err != nil {
		t.Fatalf("write keep: %v", err)
	}

	snapshot, err := LoadHierarchy(repoRoot, DefaultContextFilename)
	if err != nil {
		t.Fatalf("LoadHierarchy: %v", err)
	}
	found := false
	for _, path := range snapshot.FilePaths {
		if filepath.ToSlash(path) == filepath.ToSlash(keepPath) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected geminiignore to override gitignore, got %v", snapshot.FilePaths)
	}
}

func TestLoadHierarchyGeminiIgnoreNoSlashWithNegation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	repoRoot := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	ignore := strings.Join([]string{
		"ignored",
		"!keep/",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repoRoot, ".geminiignore"), []byte(ignore), 0o644); err != nil {
		t.Fatalf("write geminiignore: %v", err)
	}

	ignoredDir := filepath.Join(repoRoot, "ignored")
	if err := os.MkdirAll(ignoredDir, 0o755); err != nil {
		t.Fatalf("mkdir ignored: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ignoredDir, DefaultContextFilename), []byte("Ignored"), 0o644); err != nil {
		t.Fatalf("write ignored: %v", err)
	}

	keepDir := filepath.Join(repoRoot, "keep")
	if err := os.MkdirAll(keepDir, 0o755); err != nil {
		t.Fatalf("mkdir keep: %v", err)
	}
	keepPath := filepath.Join(keepDir, DefaultContextFilename)
	if err := os.WriteFile(keepPath, []byte("Keep"), 0o644); err != nil {
		t.Fatalf("write keep: %v", err)
	}

	snapshot, err := LoadHierarchy(repoRoot, DefaultContextFilename)
	if err != nil {
		t.Fatalf("LoadHierarchy: %v", err)
	}
	for _, path := range snapshot.FilePaths {
		if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(ignoredDir, DefaultContextFilename)) {
			t.Fatalf("expected ignored dir to stay excluded with negations present, got %q", path)
		}
	}
	found := false
	for _, path := range snapshot.FilePaths {
		if filepath.ToSlash(path) == filepath.ToSlash(keepPath) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected keep path to be included, got %v", snapshot.FilePaths)
	}
}

func TestLoadHierarchyGeminiIgnoreNegationDoesNotReincludeChildren(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	repoRoot := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	ignore := strings.Join([]string{
		"ignored/",
		"!ignored/keep",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repoRoot, ".geminiignore"), []byte(ignore), 0o644); err != nil {
		t.Fatalf("write geminiignore: %v", err)
	}

	keepDir := filepath.Join(repoRoot, "ignored", "keep")
	if err := os.MkdirAll(keepDir, 0o755); err != nil {
		t.Fatalf("mkdir keep: %v", err)
	}
	keepPath := filepath.Join(keepDir, DefaultContextFilename)
	if err := os.WriteFile(keepPath, []byte("Keep"), 0o644); err != nil {
		t.Fatalf("write keep: %v", err)
	}

	okDir := filepath.Join(repoRoot, "ok")
	if err := os.MkdirAll(okDir, 0o755); err != nil {
		t.Fatalf("mkdir ok: %v", err)
	}
	okPath := filepath.Join(okDir, DefaultContextFilename)
	if err := os.WriteFile(okPath, []byte("Ok"), 0o644); err != nil {
		t.Fatalf("write ok: %v", err)
	}

	snapshot, err := LoadHierarchy(repoRoot, DefaultContextFilename)
	if err != nil {
		t.Fatalf("LoadHierarchy: %v", err)
	}
	for _, path := range snapshot.FilePaths {
		if filepath.ToSlash(path) == filepath.ToSlash(keepPath) {
			t.Fatalf("expected keep file to stay ignored without dir pattern, got %q", path)
		}
	}
	found := false
	for _, path := range snapshot.FilePaths {
		if filepath.ToSlash(path) == filepath.ToSlash(okPath) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ok path to be included, got %v", snapshot.FilePaths)
	}
}
