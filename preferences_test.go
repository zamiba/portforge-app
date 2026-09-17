package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPreferencesDefaultToAutoRefreshOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if p := loadPreferences(path); !p.AutoRefreshCatalog {
		t.Error("a missing file should mean the defaults, and the default is on")
	}
	// A corrupt file must not stop the program: defaults again.
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if p := loadPreferences(path); !p.AutoRefreshCatalog {
		t.Error("an unreadable file should fall back to the defaults")
	}
}

func TestSetAutoRefreshCatalogPersists(t *testing.T) {
	app := &App{prefsPath: filepath.Join(t.TempDir(), "prefs", "preferences.json"), prefs: defaultPreferences()}
	if err := app.SetAutoRefreshCatalog(false); err != nil {
		t.Fatal(err)
	}
	if app.GetSettings().AutoRefreshCatalog {
		t.Error("the live setting did not change")
	}
	if p := loadPreferences(app.prefsPath); p.AutoRefreshCatalog {
		t.Error("the setting was not written to disk")
	}
}

// Startup only checks for a newer catalog when there is a synced one to be
// newer than; the first sync belongs to the first-run screen.
func TestAutoRefreshWaitsForTheFirstSync(t *testing.T) {
	catalog := t.TempDir()
	app := &App{metadataPath: catalog, prefs: defaultPreferences()}
	if app.shouldAutoRefreshCatalog() {
		t.Error("refreshing before the first sync would race the first-run screen")
	}
	if err := os.WriteFile(filepath.Join(catalog, mediaItemsSHAFile), []byte("abc"), 0644); err != nil {
		t.Fatal(err)
	}
	if !app.shouldAutoRefreshCatalog() {
		t.Error("a synced catalog with the default preference should refresh")
	}
	app.prefs.AutoRefreshCatalog = false
	if app.shouldAutoRefreshCatalog() {
		t.Error("the preference was ignored")
	}
}

// catalogZip builds what GitHub serves for the main branch: one top-level
// directory that the sync strips.
func catalogZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create("portforge-mediaitems-main/" + name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(content))
	}
	w.Close()
	return buf.Bytes()
}

// The whole background path against a stand-in for GitHub: an unchanged SHA
// fetches nothing; a changed one syncs, rewrites the SHA, and tells the UI.
func TestAutoRefreshSyncsOnlyWhenTheCatalogChanged(t *testing.T) {
	sha := "aaa"
	zipBody := catalogZip(t, map[string]string{
		"VideoGameFanPort/New Port · 2026/.mediaitem.json": `{"_itemType":"VideoGameFanPort","_itemTitle":"New Port · 2026","title":"New Port"}`,
	})
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sha":
			w.Write([]byte(sha))
		case "/main.zip":
			downloads++
			w.Write(zipBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	oldAPI, oldZip := mediaItemsAPIURL, mediaItemsZipURL
	mediaItemsAPIURL, mediaItemsZipURL = srv.URL+"/sha", srv.URL+"/main.zip"
	defer func() { mediaItemsAPIURL, mediaItemsZipURL = oldAPI, oldZip }()

	catalog := t.TempDir()
	if err := os.WriteFile(filepath.Join(catalog, mediaItemsSHAFile), []byte("aaa"), 0644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var events []string
	app := &App{
		metadataPath: catalog,
		prefs:        defaultPreferences(),
		events: func(name string, _ interface{}) {
			mu.Lock()
			events = append(events, name)
			mu.Unlock()
		},
	}

	app.refreshCatalogIfChanged()
	if downloads != 0 || len(events) != 0 {
		t.Fatalf("an unchanged catalog was fetched: downloads=%d events=%v", downloads, events)
	}

	sha = "bbb"
	app.refreshCatalogIfChanged()
	if downloads != 1 {
		t.Fatalf("a changed catalog should be fetched once, got %d downloads", downloads)
	}
	if _, err := os.Stat(filepath.Join(catalog, "VideoGameFanPort", "New Port · 2026", ".mediaitem.json")); err != nil {
		t.Error("the new catalog was not written")
	}
	if got, _ := os.ReadFile(filepath.Join(catalog, mediaItemsSHAFile)); string(got) != "bbb" {
		t.Errorf("SHA file = %q after the sync, want bbb", got)
	}
	if len(events) < 2 || events[0] != "catalog:refreshing" || events[len(events)-1] != "catalog:refreshed" {
		t.Errorf("events = %v, want catalog:refreshing … catalog:refreshed", events)
	}
}

func TestOnlyOneCatalogSyncRunsAtATime(t *testing.T) {
	app := &App{metadataPath: t.TempDir()}
	app.syncMu.Lock()
	defer app.syncMu.Unlock()
	if err := app.SyncMediaItems(); err == nil || err.Error() != "a catalog sync is already running" {
		t.Errorf("err = %v", err)
	}
}
