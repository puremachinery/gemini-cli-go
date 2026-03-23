package config

import (
	"context"
	"fmt"
	"os"

	"github.com/puremachinery/gemini-cli-go/internal/storage"
)

// LoadResult represents layered settings and the final merged output.
type LoadResult struct {
	SystemDefaults *File
	System         *File
	Global         *File
	Workspace      *File
	Merged         Settings
}

// Loader loads layered settings from system, global, and workspace paths.
type Loader struct {
	Store Store
}

// Load merges settings in order: system defaults -> system -> global -> workspace.
func (l Loader) Load(ctx context.Context, workspaceRoot string) (*LoadResult, error) {
	_ = ctx
	store := l.Store
	if store == nil {
		store = JSONStore{}
	}

	loadOptional := func(path string) (*File, error) {
		file, err := store.Load(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to load settings %s: %w", path, err)
		}
		return file, nil
	}

	systemDefaults, err := loadOptional(storage.SystemDefaultsPath())
	if err != nil {
		return nil, err
	}
	systemSettings, err := loadOptional(storage.SystemSettingsPath())
	if err != nil {
		return nil, err
	}
	globalSettings, err := loadOptional(storage.GlobalSettingsPath())
	if err != nil {
		return nil, err
	}
	workspaceSettings := (*File)(nil)
	if workspaceRoot != "" {
		workspaceSettings, err = loadOptional(storage.WorkspaceSettingsPath(workspaceRoot))
		if err != nil {
			return nil, err
		}
	}

	merged := Merge(
		fileSettings(systemDefaults),
		fileSettings(systemSettings),
		fileSettings(globalSettings),
		fileSettings(workspaceSettings),
	)

	return &LoadResult{
		SystemDefaults: systemDefaults,
		System:         systemSettings,
		Global:         globalSettings,
		Workspace:      workspaceSettings,
		Merged:         merged,
	}, nil
}

func fileSettings(file *File) Settings {
	if file == nil || file.Settings == nil {
		return Settings{}
	}
	return file.Settings
}
