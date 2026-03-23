package tools

import (
	"io/fs"
	"path/filepath"
	"sync"
	"time"
)

const (
	workspaceIndexMaxEntries = 4
	workspaceIndexTTL        = 10 * time.Second
)

type workspaceIndexEntry struct {
	files     []string
	scannedAt time.Time
}

var (
	workspaceIndexMu    sync.Mutex
	workspaceIndexOrder []string
	workspaceIndexCache = map[string]workspaceIndexEntry{}
)

func listWorkspaceFiles(rootAbs string) ([]string, error) {
	now := time.Now()
	if files, ok := loadWorkspaceIndex(rootAbs, now); ok {
		return files, nil
	}
	files, err := scanWorkspaceFiles(rootAbs)
	if err != nil {
		return nil, err
	}
	saveWorkspaceIndex(rootAbs, files, now)
	return copyStringSlice(files), nil
}

func invalidateWorkspaceIndex(rootAbs string) {
	if rootAbs == "" {
		return
	}
	workspaceIndexMu.Lock()
	defer workspaceIndexMu.Unlock()
	if _, ok := workspaceIndexCache[rootAbs]; !ok {
		return
	}
	delete(workspaceIndexCache, rootAbs)
	for i, key := range workspaceIndexOrder {
		if key == rootAbs {
			workspaceIndexOrder = append(workspaceIndexOrder[:i], workspaceIndexOrder[i+1:]...)
			break
		}
	}
}

func loadWorkspaceIndex(rootAbs string, now time.Time) ([]string, bool) {
	workspaceIndexMu.Lock()
	defer workspaceIndexMu.Unlock()
	entry, ok := workspaceIndexCache[rootAbs]
	if !ok {
		return nil, false
	}
	if now.Sub(entry.scannedAt) > workspaceIndexTTL {
		delete(workspaceIndexCache, rootAbs)
		for i, key := range workspaceIndexOrder {
			if key == rootAbs {
				workspaceIndexOrder = append(workspaceIndexOrder[:i], workspaceIndexOrder[i+1:]...)
				break
			}
		}
		return nil, false
	}
	return copyStringSlice(entry.files), true
}

func saveWorkspaceIndex(rootAbs string, files []string, scannedAt time.Time) {
	workspaceIndexMu.Lock()
	defer workspaceIndexMu.Unlock()
	if _, ok := workspaceIndexCache[rootAbs]; !ok {
		if len(workspaceIndexOrder) >= workspaceIndexMaxEntries {
			evict := workspaceIndexOrder[0]
			workspaceIndexOrder = workspaceIndexOrder[1:]
			delete(workspaceIndexCache, evict)
		}
		workspaceIndexOrder = append(workspaceIndexOrder, rootAbs)
	}
	workspaceIndexCache[rootAbs] = workspaceIndexEntry{
		files:     copyStringSlice(files),
		scannedAt: scannedAt,
	}
}

func scanWorkspaceFiles(rootAbs string) ([]string, error) {
	excludeMatchers, err := compilePatterns(defaultExcludes())
	if err != nil {
		return nil, err
	}
	var files []string
	walkErr := filepath.WalkDir(rootAbs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if matchesAny(rel, excludeMatchers) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return files, nil
}

func copyStringSlice(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	out := make([]string, len(items))
	copy(out, items)
	return out
}
