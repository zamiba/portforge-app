package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"portforge/store"
)

// recordActivity captures every catalog:activity event an App emits.
func recordActivity(app *App) func() []CatalogActivity {
	var mu sync.Mutex
	var seen []CatalogActivity
	app.events = func(name string, data interface{}) {
		if name != "catalog:activity" {
			return
		}
		mu.Lock()
		seen = append(seen, data.(CatalogActivity))
		mu.Unlock()
	}
	return func() []CatalogActivity {
		mu.Lock()
		defer mu.Unlock()
		return append([]CatalogActivity(nil), seen...)
	}
}

func phasesOf(acts []CatalogActivity) []string {
	var phases []string
	for _, a := range acts {
		if n := len(phases); n == 0 || phases[n-1] != a.Phase {
			phases = append(phases, a.Phase)
		}
	}
	return phases
}

// A sync reports each phase as it goes and ends with done, after which the
// catalog reads as idle to a page that loads later.
func TestSyncReportsItsPhasesAndEndsIdle(t *testing.T) {
	zipBody := catalogZip(t, map[string]string{
		"VideoGameFanPort/New Port · 2026/.mediaitem.json": `{"_itemType":"VideoGameFanPort","_itemTitle":"New Port · 2026","title":"New Port"}`,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sha":
			w.Write([]byte("bbb"))
		case "/main.zip":
			w.Write(zipBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	oldAPI, oldZip := mediaItemsAPIURL, mediaItemsZipURL
	mediaItemsAPIURL, mediaItemsZipURL = srv.URL+"/sha", srv.URL+"/main.zip"
	defer func() { mediaItemsAPIURL, mediaItemsZipURL = oldAPI, oldZip }()

	app := &App{metadataPath: t.TempDir(), prefs: defaultPreferences()}
	events := recordActivity(app)

	if err := app.SyncMediaItems(); err != nil {
		t.Fatal(err)
	}
	want := []string{phaseDownloading, phaseExtracting, phaseCopying, phaseDone}
	if got := phasesOf(events()); !equalStrings(got, want) {
		t.Errorf("phases = %v, want %v (no store, so no indexing phase)", got, want)
	}
	for _, ev := range events() {
		if ev.Kind != "sync" {
			t.Errorf("event %+v is not a sync", ev)
		}
	}
	if act := app.GetCatalogActivity(); act.Kind != "" {
		t.Errorf("after the sync the catalog should be idle, got %+v", act)
	}
}

// A sync that cannot reach GitHub says so on the same channel, with the error,
// and leaves the catalog idle rather than stuck at "downloading".
func TestSyncFailureIsReportedAndClearsTheActivity(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // every request now fails to connect
	oldZip := mediaItemsZipURL
	mediaItemsZipURL = srv.URL + "/main.zip"
	defer func() { mediaItemsZipURL = oldZip }()

	app := &App{metadataPath: t.TempDir(), prefs: defaultPreferences()}
	events := recordActivity(app)

	if err := app.SyncMediaItems(); err == nil {
		t.Fatal("a sync against a dead server should fail")
	}
	got := events()
	last := got[len(got)-1]
	if last.Phase != phaseFailed || last.Error == "" || last.Kind != "sync" {
		t.Errorf("last event = %+v, want a sync failure carrying the error", last)
	}
	if act := app.GetCatalogActivity(); act.Kind != "" {
		t.Errorf("a failed sync left the catalog busy: %+v", act)
	}
}

// While a sync runs, a page that loads sees it in progress, and a rebuild of
// the index — which the sync will do itself — is refused.
func TestActivityIsVisibleMidSyncAndBlocksARebuild(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := &App{metadataPath: t.TempDir(), store: st}
	recordActivity(app)

	app.setCatalogActivity(CatalogActivity{Kind: "sync", Phase: phaseDownloading, Percent: 40})
	if act := app.GetCatalogActivity(); act.Kind != "sync" || act.Percent != 40 {
		t.Errorf("GetCatalogActivity = %+v, want the running sync", act)
	}
	app.syncMu.Lock()
	err = app.RefreshLibraryIndex()
	app.syncMu.Unlock()
	if err == nil || err.Error() != "a catalog sync is already running" {
		t.Errorf("rebuild during a sync: err = %v", err)
	}

	// With the sync over, the rebuild runs and reports itself as one.
	events := recordActivity(app)
	if err := app.RefreshLibraryIndex(); err != nil {
		t.Fatal(err)
	}
	if got, want := phasesOf(events()), []string{phaseIndexing, phaseDone}; !equalStrings(got, want) {
		t.Errorf("rebuild phases = %v, want %v", got, want)
	}
	if ev := events()[0]; ev.Kind != "index" {
		t.Errorf("a rebuild should report Kind index, got %+v", ev)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
