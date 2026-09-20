package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
)

// Preferences are the settings PortForge keeps for itself, as opposed to the
// storage units, which are shared with every program in the suite and live in
// their own file. They sit in preferences.json in the config directory — not
// settings.json, which an earlier version used for the library folder and
// which the migration still looks for.
type Preferences struct {
	// AutoRefreshCatalog checks for a newer catalog when PortForge starts and
	// fetches it in the background. On by default: the check is one small
	// request, the download only happens when something changed, and a catalog
	// that quietly goes stale is the failure this exists to prevent.
	AutoRefreshCatalog bool `json:"autoRefreshCatalog"`

	// ActiveProfile is the slug of the profile saves and settings go to. Empty
	// means PortForge's own default profile, which exists so that a person who
	// does not want profiles never has to think about them — see profiles.go.
	ActiveProfile string `json:"activeProfile,omitempty"`
}

func defaultPreferences() Preferences {
	return Preferences{AutoRefreshCatalog: true}
}

func preferencesPath() string {
	dir, err := configDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "preferences.json")
}

// loadPreferences reads the file, falling back to the defaults when it is
// absent. A file that is present but unreadable is reported and also falls
// back, so a corrupt preference never keeps the program from starting.
func loadPreferences(path string) Preferences {
	prefs := defaultPreferences()
	if path == "" {
		return prefs
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return prefs
	}
	if err != nil {
		log.Printf("preferences: %v; using defaults", err)
		return prefs
	}
	if err := json.Unmarshal(data, &prefs); err != nil {
		log.Printf("preferences: %s: %v; using defaults", path, err)
		return defaultPreferences()
	}
	return prefs
}

func savePreferences(path string, prefs Preferences) error {
	if path == "" {
		return errors.New("no configuration directory to save preferences in")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
