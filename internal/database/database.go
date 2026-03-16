// Package database manages the installed AppImage registry.
package database

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/adrg/xdg"
)

const dbKey = "appimg/installed.json"

// Entry records a single installed AppImage.
type Entry struct {
	Name          string    `json:"name"`
	Source        string    `json:"source"`
	AssetPattern  string    `json:"asset_pattern,omitempty"`
	InstalledPath string    `json:"installed_path"`
	Version       string    `json:"version"`
	InstalledAt   time.Time `json:"installed_at"`
	DownloadURL   string    `json:"download_url"`
	PublishedAt   string    `json:"published_at"`
	Prerelease    bool      `json:"prerelease,omitempty"`
	PinnedVersion string    `json:"pinned_version,omitempty"`
	Rolling       bool      `json:"rolling,omitempty"`
	VersionPrefix string    `json:"version_prefix,omitempty"`
}

// DB is the in-memory representation of the installed package registry.
type DB struct {
	Entries map[string]*Entry `json:"entries"`
	path    string
	file    *os.File
}

func dbFile() (string, error) {
	return xdg.StateFile(dbKey)
}

// Open loads the database from disk, acquiring an exclusive flock for the
// duration of the caller's operation. Call Close to save and release.
func Open() (*DB, error) {
	path, err := dbFile()
	if err != nil {
		return nil, err
	}
	return openPath(path)
}

func openPath(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock database: %w", err)
	}

	db := &DB{
		Entries: make(map[string]*Entry),
		path:    path,
		file:    f,
	}

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 0 {
		if err := json.NewDecoder(f).Decode(db); err != nil {
			return nil, fmt.Errorf("parse database: %w", err)
		}
		if db.Entries == nil {
			db.Entries = make(map[string]*Entry)
		}
	}

	return db, nil
}

// Save writes the current state to disk.
func (db *DB) Save() error {
	if err := db.file.Truncate(0); err != nil {
		return err
	}
	if _, err := db.file.Seek(0, 0); err != nil {
		return err
	}
	enc := json.NewEncoder(db.file)
	enc.SetIndent("", "  ")
	return enc.Encode(db)
}

// Close saves the database and releases the file lock.
func (db *DB) Close() error {
	if err := db.Save(); err != nil {
		return err
	}
	_ = syscall.Flock(int(db.file.Fd()), syscall.LOCK_UN)
	return db.file.Close()
}

func (db *DB) Get(name string) (*Entry, bool) {
	e, ok := db.Entries[name]
	return e, ok
}

// Find looks up an entry by name or source string, returning the first match.
func (db *DB) Find(nameOrSource string) (*Entry, bool) {
	if e, ok := db.Entries[nameOrSource]; ok {
		return e, true
	}
	for _, e := range db.Entries {
		if e.Source == nameOrSource {
			return e, true
		}
	}
	return nil, false
}

func (db *DB) Set(entry *Entry) {
	db.Entries[entry.Name] = entry
}

func (db *DB) Delete(name string) {
	delete(db.Entries, name)
}

func (db *DB) All() []*Entry {
	entries := make([]*Entry, 0, len(db.Entries))
	for _, e := range db.Entries {
		entries = append(entries, e)
	}
	return entries
}

// FilterEntries returns the entries whose Name matches the given regex pattern.
// An empty pattern returns all entries unchanged.
func FilterEntries(entries []*Entry, pattern string) ([]*Entry, error) {
	if pattern == "" {
		return entries, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid filter pattern: %w", err)
	}
	var out []*Entry
	for _, e := range entries {
		if re.MatchString(e.Name) {
			out = append(out, e)
		}
	}
	return out, nil
}
