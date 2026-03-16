package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFind_ByName(t *testing.T) {
	db := openTemp(t)
	defer func() { _ = db.Close() }()

	db.Set(&Entry{Name: "Bitwarden", Source: "catalog:Bitwarden"})

	got, ok := db.Find("Bitwarden")
	if !ok {
		t.Fatal("Find returned ok=false for existing entry by name")
	}
	if got.Name != "Bitwarden" {
		t.Errorf("Name = %q, want Bitwarden", got.Name)
	}
}

func TestFind_BySource(t *testing.T) {
	db := openTemp(t)
	defer func() { _ = db.Close() }()

	db.Set(&Entry{Name: "Bitwarden", Source: "catalog:Bitwarden"})

	got, ok := db.Find("catalog:Bitwarden")
	if !ok {
		t.Fatal("Find returned ok=false for existing entry by source")
	}
	if got.Name != "Bitwarden" {
		t.Errorf("Name = %q, want Bitwarden", got.Name)
	}
}

func TestFind_NotFound(t *testing.T) {
	db := openTemp(t)
	defer func() { _ = db.Close() }()

	_, ok := db.Find("NonExistent")
	if ok {
		t.Fatal("Find returned ok=true for missing entry")
	}
}

func TestFilterEntries_Empty(t *testing.T) {
	entries := []*Entry{{Name: "Alpha"}, {Name: "Beta"}}
	got, err := FilterEntries(entries, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(entries) {
		t.Errorf("got %d entries, want %d", len(got), len(entries))
	}
}

func TestFilterEntries_Match(t *testing.T) {
	entries := []*Entry{
		{Name: "Bitwarden"},
		{Name: "FreeCAD"},
		{Name: "BitBar"},
	}
	got, err := FilterEntries(entries, "Bit")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("got %d entries, want 2", len(got))
	}
}

func TestFilterEntries_CaseInsensitiveRegex(t *testing.T) {
	entries := []*Entry{{Name: "Bitwarden"}, {Name: "FreeCAD"}}
	got, err := FilterEntries(entries, "(?i)freecad")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "FreeCAD" {
		t.Errorf("got %v, want [FreeCAD]", got)
	}
}

func TestFilterEntries_NoMatch(t *testing.T) {
	entries := []*Entry{{Name: "Bitwarden"}}
	got, err := FilterEntries(entries, "xyz")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want 0", len(got))
	}
}

func TestFilterEntries_InvalidRegex(t *testing.T) {
	entries := []*Entry{{Name: "App"}}
	_, err := FilterEntries(entries, "[invalid")
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
}

func openTemp(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "installed.json")
	db, err := openPath(path)
	if err != nil {
		t.Fatalf("openPath: %v", err)
	}
	return db
}

func TestOpenEmpty(t *testing.T) {
	db := openTemp(t)
	defer func() { _ = db.Close() }()

	if len(db.All()) != 0 {
		t.Errorf("expected empty DB, got %d entries", len(db.All()))
	}
}

func TestSetAndGet(t *testing.T) {
	db := openTemp(t)
	defer func() { _ = db.Close() }()

	entry := &Entry{
		Name:          "Bitwarden",
		Source:        "catalog:Bitwarden",
		InstalledPath: "/home/user/Applications/Bitwarden.AppImage",
		Version:       "v2024.1.0",
		InstalledAt:   time.Now().UTC().Truncate(time.Second),
		DownloadURL:   "https://example.com/Bitwarden.AppImage",
		PublishedAt:   "2024-01-15T00:00:00Z",
	}
	db.Set(entry)

	got, ok := db.Get("Bitwarden")
	if !ok {
		t.Fatal("Get returned ok=false after Set")
	}
	if got.Name != entry.Name {
		t.Errorf("Name = %q, want %q", got.Name, entry.Name)
	}
	if got.Version != entry.Version {
		t.Errorf("Version = %q, want %q", got.Version, entry.Version)
	}
	if got.Source != entry.Source {
		t.Errorf("Source = %q, want %q", got.Source, entry.Source)
	}
}

func TestGet_NotFound(t *testing.T) {
	db := openTemp(t)
	defer func() { _ = db.Close() }()

	_, ok := db.Get("NonExistent")
	if ok {
		t.Fatal("Get returned ok=true for missing entry")
	}
}

func TestDelete(t *testing.T) {
	db := openTemp(t)
	defer func() { _ = db.Close() }()

	db.Set(&Entry{Name: "App"})
	db.Delete("App")

	_, ok := db.Get("App")
	if ok {
		t.Fatal("Get returned ok=true after Delete")
	}
}

func TestAll(t *testing.T) {
	db := openTemp(t)
	defer func() { _ = db.Close() }()

	db.Set(&Entry{Name: "Alpha"})
	db.Set(&Entry{Name: "Beta"})
	db.Set(&Entry{Name: "Gamma"})

	all := db.All()
	if len(all) != 3 {
		t.Errorf("All() returned %d entries, want 3", len(all))
	}
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed.json")

	db, err := openPath(path)
	if err != nil {
		t.Fatalf("openPath: %v", err)
	}
	db.Set(&Entry{
		Name:          "FreeCAD",
		Source:        "github:FreeCAD/FreeCAD",
		InstalledPath: "/home/user/Applications/FreeCAD.AppImage",
		Version:       "1.0.2",
		InstalledAt:   time.Now().UTC().Truncate(time.Second),
		PublishedAt:   "2025-01-01T00:00:00Z",
		Rolling:       false,
		Prerelease:    false,
	})
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := openPath(path)
	if err != nil {
		t.Fatalf("openPath (reopen): %v", err)
	}
	defer func() { _ = db2.Close() }()

	got, ok := db2.Get("FreeCAD")
	if !ok {
		t.Fatal("entry not found after reopen")
	}
	if got.Version != "1.0.2" {
		t.Errorf("Version = %q, want 1.0.2", got.Version)
	}
	if got.Source != "github:FreeCAD/FreeCAD" {
		t.Errorf("Source = %q, want github:FreeCAD/FreeCAD", got.Source)
	}
}

func TestSet_Overwrite(t *testing.T) {
	db := openTemp(t)
	defer func() { _ = db.Close() }()

	db.Set(&Entry{Name: "App", Version: "v1.0.0"})
	db.Set(&Entry{Name: "App", Version: "v2.0.0"})

	got, ok := db.Get("App")
	if !ok {
		t.Fatal("Get returned ok=false")
	}
	if got.Version != "v2.0.0" {
		t.Errorf("Version = %q, want v2.0.0", got.Version)
	}
	if len(db.All()) != 1 {
		t.Errorf("All() = %d, want 1 (Set should overwrite)", len(db.All()))
	}
}
