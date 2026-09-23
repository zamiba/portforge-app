package main

import (
	"os"
	"path/filepath"
	"testing"

	"portforge/metadata"
	"portforge/models"
)

// newNoticesApp writes one item carrying notices into a temp metadata tree.
func newNoticesApp(t *testing.T) *App {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, metadata.PortItemType, "Ghostship · 2026")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const item = `{
	  "_itemType": "VideoGameFanPort",
	  "_itemTitle": "Ghostship · 2026",
	  "title": "Ghostship",
	  "releaseYear": 2026,
	  "versionType": "FanPort",
	  "platforms": ["Windows", "Linux"],
	  "versions": [
	    {"_itemType": "SoftwareVersion", "title": "Nautilus - Alfa 3.0.0",
	     "notices": [
	       {"type": "warning", "affectedPlatforms": ["Windows"], "message": "Windows only."},
	       {"type": "info", "message": "Everyone."},
	       {"type": "critical", "affectedPlatforms": ["Linux"], "message": "Linux only."}
	     ]},
	    {"_itemType": "SoftwareVersion", "title": "Mary Celeste - Alfa 2.0.0"}
	  ]
	}`
	if err := os.WriteFile(filepath.Join(dir, ".mediaitem.json"), []byte(item), 0o644); err != nil {
		t.Fatal(err)
	}
	return &App{metadataPath: base}
}

func TestGetVersionNoticesFiltersByPlatform(t *testing.T) {
	app := newNoticesApp(t)

	got, err := app.GetVersionNotices("Ghostship · 2026", "Nautilus - Alfa 3.0.0", "Windows")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want the Windows notice and the unrestricted one, got %d: %+v", len(got), got)
	}
	if got[0].Message != "Windows only." || got[1].Message != "Everyone." {
		t.Errorf("wrong notices or order: %+v", got)
	}
	if got[0].Type != models.NoticeWarning || got[1].Type != models.NoticeInfo {
		t.Errorf("types not resolved: %+v", got)
	}

	// The Linux-only notice is typed "critical", which this build does not know.
	// It must still arrive, as a warning.
	got, err = app.GetVersionNotices("Ghostship · 2026", "Nautilus - Alfa 3.0.0", "Linux")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want the unrestricted notice and the Linux one, got %d: %+v", len(got), got)
	}
	if got[1].Message != "Linux only." || got[1].Type != models.NoticeWarning {
		t.Errorf("an unknown type should arrive as a warning, got %+v", got[1])
	}
}

// An arch-qualified build target still gets the bare OS notice, since specs spell
// a platform either way and a notice should not have to guess which.
func TestGetVersionNoticesMatchesArchTarget(t *testing.T) {
	app := newNoticesApp(t)

	got, err := app.GetVersionNotices("Ghostship · 2026", "Nautilus - Alfa 3.0.0", "Windows-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 notices for Windows-amd64, got %d: %+v", len(got), got)
	}
}

// A version with no entry in the item's metadata, and an unselected version, both
// have no notices rather than borrowing another release's.
func TestGetVersionNoticesWithoutAMatchingEntry(t *testing.T) {
	app := newNoticesApp(t)

	for _, version := range []string{"Mary Celeste - Alfa 2.0.0", "Some Unreleased Build", ""} {
		got, err := app.GetVersionNotices("Ghostship · 2026", version, "Windows")
		if err != nil {
			t.Fatalf("%q: %v", version, err)
		}
		if len(got) != 0 {
			t.Errorf("%q: want no notices, got %+v", version, got)
		}
	}
}
