package main

import (
	"errors"
	"os"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/config"
	"github.com/puremachinery/gemini-cli-go/internal/storage"
)

func loadGlobalSettingsFileFresh() (*config.File, error) {
	path := storage.GlobalSettingsPath()
	store := config.JSONStore{}
	file, err := store.Load(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		file = &config.File{Path: path, Settings: config.Settings{}}
	}
	if file.Settings == nil {
		file.Settings = config.Settings{}
	}
	return file, nil
}

func saveGlobalSettingsFile(file *config.File) error {
	if file == nil {
		return errors.New("settings file is nil")
	}
	store := config.JSONStore{}
	return store.Save(file)
}

func withGlobalSettingsLock(update func(*config.File) error) error {
	return storage.WithFileLock(storage.GlobalSettingsPath(), func() error {
		file, err := loadGlobalSettingsFileFresh()
		if err != nil {
			return err
		}
		if update != nil {
			if err := update(file); err != nil {
				return err
			}
		}
		return saveGlobalSettingsFile(file)
	})
}

func setSelectedAuthType(authType string) error {
	return withGlobalSettingsLock(func(file *config.File) error {
		file.Settings.Set("security.auth.selectedType", authType)
		return nil
	})
}

func getSelectedAuthType() (string, error) {
	file, err := loadGlobalSettingsFileFresh()
	if err != nil {
		return "", err
	}
	value, _ := file.Settings.GetString("security.auth.selectedType")
	return strings.TrimSpace(value), nil
}

func clearSelectedAuthType() error {
	return withGlobalSettingsLock(func(file *config.File) error {
		if file.Settings != nil {
			file.Settings.Delete("security.auth.selectedType")
		}
		return nil
	})
}

func persistModelSelection(model string) error {
	return withGlobalSettingsLock(func(file *config.File) error {
		file.Settings.Set("model.name", strings.TrimSpace(model))
		return nil
	})
}
