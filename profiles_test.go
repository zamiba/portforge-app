package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zamiba/go-mediaitems-profiles/profile"

	"portforge/metadata"
)

// withProfiles gives an App a profiles folder and a preferences file of its
// own, so nothing here touches the real configuration directory.
func withProfiles(t *testing.T, app *App) {
	t.Helper()
	m, err := profile.New(profile.Options{Dir: filepath.Join(t.TempDir(), "profiles")})
	if err != nil {
		t.Fatal(err)
	}
	app.profiles = m
	app.prefsPath = filepath.Join(t.TempDir(), "preferences.json")
	app.prefs = defaultPreferences()
}

// Nobody has to create a profile: the first thing that needs one gets
// PortForge's own, made on the spot.
func TestTheDefaultProfileAppearsOnFirstUse(t *testing.T) {
	app := &App{}
	withProfiles(t, app)
	if entries, _ := os.ReadDir(app.profiles.Dir()); len(entries) != 0 {
		t.Fatal("opening the profiles folder must not create a profile")
	}
	list, err := app.GetProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Slug != "portforge" || !list[0].Active || list[0].CreatedBy != "portforge" {
		t.Errorf("GetProfiles = %+v", list)
	}
	if got := app.GetSettings().ActiveProfile; got != "portforge" {
		t.Errorf("Settings.ActiveProfile = %q", got)
	}
}

func TestCreatingAProfileSwitchesToIt(t *testing.T) {
	app := &App{}
	withProfiles(t, app)
	p, err := app.CreateProfile("Sam")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Active || p.Slug != "sam" {
		t.Errorf("CreateProfile = %+v", p)
	}
	if loadPreferences(app.prefsPath).ActiveProfile != "sam" {
		t.Error("the switch was not written to preferences")
	}
	list, _ := app.GetProfiles()
	var active []string
	for _, p := range list {
		if p.Active {
			active = append(active, p.Slug)
		}
	}
	if strings.Join(active, ",") != "sam" {
		t.Errorf("active = %v, want only sam", active)
	}
	// The default is only made when it is needed; someone who chose their
	// own profile first never gets a "portforge" one cluttering the list.
	if err := app.SetActiveProfile("portforge"); err == nil {
		t.Error("the default should not exist when a chosen profile was used first")
	}
	if _, err := app.CreateProfile("Kid"); err != nil {
		t.Fatal(err)
	}
	if err := app.SetActiveProfile("sam"); err != nil {
		t.Fatal(err)
	}
	if err := app.SetActiveProfile("nobody"); err == nil {
		t.Error("switching to a profile that does not exist should fail")
	}
	if app.GetSettings().ActiveProfile != "sam" {
		t.Error("a failed switch changed the active profile")
	}
}

// A preference pointing at a profile whose folder is gone must not break every
// launch; it falls back to the default and is forgotten.
func TestAMissingActiveProfileFallsBackToTheDefault(t *testing.T) {
	app := &App{}
	withProfiles(t, app)
	if _, err := app.CreateProfile("Sam"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(app.profiles.Dir(), "sam")); err != nil {
		t.Fatal(err)
	}
	p, err := app.activeProfile()
	if err != nil {
		t.Fatal(err)
	}
	if p.Slug != "portforge" {
		t.Errorf("active = %s, want the default", p.Slug)
	}
	if loadPreferences(app.prefsPath).ActiveProfile != "" {
		t.Error("the stale preference should have been cleared")
	}
}

func TestProfilesCannotBeSwitchedWhileAGameRuns(t *testing.T) {
	app := &App{}
	withProfiles(t, app)
	if _, err := app.CreateProfile("Sam"); err != nil {
		t.Fatal(err)
	}
	app.playing = "Melee Test · 2026"
	if err := app.SetActiveProfile("portforge"); err == nil || !strings.Contains(err.Error(), "while Melee Test · 2026 is running") {
		t.Errorf("SetActiveProfile: err = %v", err)
	}
	if _, err := app.CreateProfile("Kid"); err == nil {
		t.Error("CreateProfile switches, so it must be refused too")
	}
	// Renaming changes no path, so it is fine mid-game.
	if err := app.RenameProfile("sam", "Samantha"); err != nil {
		t.Errorf("RenameProfile: %v", err)
	}
}

// ${profilePath} becomes the active profile's folder for this port, mirroring
// the port's own folder on the storage unit, and is created for the game.
func TestLaunchResolvesTheProfilePath(t *testing.T) {
	app, versionDir := launchTestApp(t, []string{"--save-dir", "${profilePath}"}, false)
	withProfiles(t, app)
	if _, err := app.CreateProfile("Sam"); err != nil {
		t.Fatal(err)
	}
	if err := app.LaunchVersion("Melee Test · 2026", ""); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(app.profiles.Dir(), "sam", profile.ItemsDirName, metadata.PortItemType, "Melee Test · 2026")
	if got := argvOf(t, versionDir); got != "--save-dir\n"+want+"\n" {
		t.Errorf("launched with %q, want the profile folder %s", got, want)
	}
	if st, err := os.Stat(want); err != nil || !st.IsDir() {
		t.Error("the profile folder for the port was not created")
	}
}

// A port that names its profile folder with a qualifier is a spec error, not
// something to guess at.
func TestLaunchRefusesAQualifiedProfilePath(t *testing.T) {
	app, versionDir := launchTestApp(t, []string{"${profilePath.saves}"}, false)
	withProfiles(t, app)
	err := app.LaunchVersion("Melee Test · 2026", "")
	if err == nil || !strings.Contains(err.Error(), "cannot be launched") || !strings.Contains(err.Error(), "takes no name") {
		t.Errorf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(versionDir, "install", "argv")); statErr == nil {
		t.Error("the executable was started anyway")
	}
}
