package config

// File represents a parsed settings document.
type File struct {
	Path     string
	Settings Settings
	Raw      []byte
}

// Store loads and saves settings files, preserving formatting when possible.
type Store interface {
	Load(path string) (*File, error)
	Save(file *File) error
}
