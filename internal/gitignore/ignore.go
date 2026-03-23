// Package gitignore wraps git check-ignore functionality.
package gitignore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const cacheMaxEntries = 8

var (
	cacheMu    sync.Mutex
	cacheOrder []string
	cache      = map[string]cacheEntry{}
)

type cacheEntry struct {
	entries map[string]bool
	ok      bool
}

// IgnoredSet returns paths ignored by git check-ignore. The bool reports if git was usable.
func IgnoredSet(rootAbs string, files []string) (map[string]struct{}, bool) {
	relPaths := make([]string, 0, len(files))
	for _, path := range files {
		rel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel != "" && rel != "." {
			relPaths = append(relPaths, rel)
		}
	}
	if len(relPaths) == 0 {
		return map[string]struct{}{}, true
	}

	rootKey := ignoreCacheKey(rootAbs)
	cached, ok := loadCache(rootKey)
	if ok && !cached.ok {
		return nil, false
	}
	ignored := map[string]struct{}{}
	unknown := make([]string, 0, len(relPaths))
	if ok {
		for _, rel := range relPaths {
			if ignoredFlag, known := cached.entries[rel]; known {
				if ignoredFlag {
					ignored[rel] = struct{}{}
				}
				continue
			}
			unknown = append(unknown, rel)
		}
	} else {
		unknown = relPaths
	}
	if len(unknown) == 0 {
		return ignored, true
	}

	cmd := exec.Command("git", "check-ignore", "--stdin")
	cmd.Dir = rootAbs
	cmd.Stdin = strings.NewReader(strings.Join(unknown, "\n") + "\n")
	var out strings.Builder
	var errOut strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			results := make(map[string]bool, len(unknown))
			for _, rel := range unknown {
				results[rel] = false
			}
			updateCache(rootKey, true, results)
			return ignored, true
		}
		if errors.Is(err, exec.ErrNotFound) {
			saveCache(rootKey, nil, false)
			return nil, false
		}
		saveCache(rootKey, nil, false)
		return nil, false
	}

	seen := map[string]struct{}{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rel := filepath.ToSlash(line)
		ignored[rel] = struct{}{}
		seen[rel] = struct{}{}
	}
	results := make(map[string]bool, len(unknown))
	for _, rel := range unknown {
		_, isIgnored := seen[rel]
		results[rel] = isIgnored
		if isIgnored {
			ignored[rel] = struct{}{}
		}
	}
	updateCache(rootKey, true, results)
	return ignored, true
}

func ignoreCacheKey(rootAbs string) string {
	h := sha256.New()
	h.Write([]byte(rootAbs))
	return hex.EncodeToString(h.Sum(nil))
}

func loadCache(key string) (cacheEntry, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	entry, ok := cache[key]
	if !ok {
		return cacheEntry{}, false
	}
	return cacheEntry{
		entries: cloneBoolMap(entry.entries),
		ok:      entry.ok,
	}, true
}

func updateCache(key string, ok bool, entries map[string]bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	entry, exists := cache[key]
	if !exists {
		entry = cacheEntry{entries: map[string]bool{}, ok: ok}
		if len(cacheOrder) >= cacheMaxEntries {
			evict := cacheOrder[0]
			cacheOrder = cacheOrder[1:]
			delete(cache, evict)
		}
		cacheOrder = append(cacheOrder, key)
	}
	if ok {
		entry.ok = true
	}
	if entry.entries == nil {
		entry.entries = map[string]bool{}
	}
	for k, v := range entries {
		entry.entries[k] = v
	}
	cache[key] = entry
}

func saveCache(key string, ignored map[string]struct{}, ok bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if len(cacheOrder) >= cacheMaxEntries {
		evict := cacheOrder[0]
		cacheOrder = cacheOrder[1:]
		delete(cache, evict)
	}
	cacheOrder = append(cacheOrder, key)
	entries := map[string]bool{}
	for path := range ignored {
		entries[path] = true
	}
	cache[key] = cacheEntry{entries: entries, ok: ok}
}

func cloneBoolMap(src map[string]bool) map[string]bool {
	if len(src) == 0 {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}
