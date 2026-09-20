package main

import "sync"

// CatalogActivity is what the catalog is doing right now: a sync pulling a new
// copy from GitHub, or a rebuild of the library index. The frontend shows it in
// a status bar the way it shows a running install, whichever screen started it —
// the Settings page, the first-run screen or the background refresh at startup.
type CatalogActivity struct {
	Kind    string `json:"kind"`  // "sync" or "index"; "" when idle
	Phase   string `json:"phase"` // downloading, extracting, copying, indexing, done, failed
	Percent int    `json:"percent"`
	Error   string `json:"error,omitempty"` // set when Phase is "failed"
}

// Progress phases of a catalog sync, in the order they happen. A rebuild of the
// index on its own goes straight to indexing.
const (
	phaseDownloading = "downloading"
	phaseExtracting  = "extracting"
	phaseCopying     = "copying"
	phaseIndexing    = "indexing"
	phaseDone        = "done"
	phaseFailed      = "failed"
)

type catalogActivity struct {
	mu      sync.Mutex
	current CatalogActivity
}

// setCatalogActivity records the catalog's state and tells the frontend. A
// terminal phase is broadcast but not kept: a page that loads afterwards asks
// GetCatalogActivity and should see idle, not the tail of something finished.
func (a *App) setCatalogActivity(act CatalogActivity) {
	a.activity.mu.Lock()
	if act.Phase == phaseDone || act.Phase == phaseFailed {
		a.activity.current = CatalogActivity{}
	} else {
		a.activity.current = act
	}
	a.activity.mu.Unlock()
	a.emit("catalog:activity", act)
}

// GetCatalogActivity returns what the catalog is doing, or an idle value with an
// empty Kind. The frontend reads it once on load, so a sync that began before
// the page did — the startup refresh, or a page reloaded mid-download under
// -server — still gets its status bar.
func (a *App) GetCatalogActivity() CatalogActivity {
	a.activity.mu.Lock()
	defer a.activity.mu.Unlock()
	return a.activity.current
}
