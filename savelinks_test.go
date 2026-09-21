package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zamiba/go-mediaitems-profiles/profile"

	"portforge/metadata"
	"portforge/models"
)

// A spec whose build ships a game and a default config, and declares the
// saves folder and the config as the user's.
const linkSpec = `{
  "userDataPaths": ["install/saves", "install/game.config"],
  "builds": [{
    "targetPlatforms": ["Linux", "Mac", "Windows"],
    "versions": ["1.0"],
    "dependencies": ["sh"],
    "steps": [
      { "step": "createDir", "path": "install" },
      { "step": "run", "cmd": "sh", "args": ["-c", "printf binary > install/game; printf shipped-default > install/game.config"] },
      { "step": "defineExecutable", "executable": "install/game", "title": "Play" }
    ]
  }]
}`

func linkTestApp(t *testing.T, spec string) (*App, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the spec uses a POSIX shell and the assertions expect symlinks")
	}
	app, versionDir := userDataTestApp(t, "Link Port", spec)
	withProfiles(t, app)
	return app, versionDir
}

func profileItemDir(t *testing.T, app *App, slug string) string {
	t.Helper()
	p, err := app.profiles.Get(slug)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := p.ItemDir(metadata.PortItemType, "Link Port")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func linksTo(t *testing.T, link, want string) {
	t.Helper()
	target, ok := readLink(link)
	if !ok {
		t.Fatalf("%s is not a link", link)
	}
	if target != want {
		t.Errorf("%s -> %s, want %s", link, target, want)
	}
}

// Installing over a port from before profiles existed moves its saves into
// the profile and leaves links in their place; the game still finds them.
func TestInstallMovesExistingSavesIntoTheProfile(t *testing.T) {
	app, versionDir := linkTestApp(t, linkSpec)
	writeAt(t, filepath.Join(versionDir, "install", "game.config"), "user edited")
	writeAt(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")

	if err := app.InstallVersion("Link Port", map[string]string{}, "1.0", ""); err != nil {
		t.Fatal(err)
	}
	inProfile := profileItemDir(t, app, "portforge")
	// The user's copy won over the shipped default, and it now lives in the
	// profile with the port's path pointing at it.
	fileIs(t, filepath.Join(inProfile, "install", "game.config"), "user edited")
	fileIs(t, filepath.Join(inProfile, "install", "saves", "slot1.sav"), "save data")
	linksTo(t, filepath.Join(versionDir, "install", "game.config"), filepath.Join(inProfile, "install", "game.config"))
	linksTo(t, filepath.Join(versionDir, "install", "saves"), filepath.Join(inProfile, "install", "saves"))
	fileIs(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data") // through the link

	state, _ := metadata.ReadInstallState(versionDir)
	if got := strings.Join(state.UserDataPaths, ","); got != "install/saves,install/game.config" {
		t.Errorf("state.UserDataPaths = %q", got)
	}
	status, err := app.GetSaveLinks("Link Port")
	if err != nil {
		t.Fatal(err)
	}
	if status.Failed || len(status.Links) != 2 || status.Links[0].State != linkLinked || status.Links[1].State != linkLinked {
		t.Errorf("GetSaveLinks = %+v", status)
	}
	if status.Slug != "portforge" || status.Folder != inProfile {
		t.Errorf("status names %s at %s", status.Slug, status.Folder)
	}
}

// A fresh install has nothing to link yet; the first session creates the
// saves in the port's folder, and its end moves them in.
func TestASessionsNewSavesAreMovedInWhenItEnds(t *testing.T) {
	app, versionDir := linkTestApp(t, linkSpec)
	if err := app.InstallVersion("Link Port", map[string]string{}, "1.0", ""); err != nil {
		t.Fatal(err)
	}
	inProfile := profileItemDir(t, app, "portforge")
	// The config the build shipped is the user's from now on; the saves
	// folder does not exist until the game makes it.
	linksTo(t, filepath.Join(versionDir, "install", "game.config"), filepath.Join(inProfile, "install", "game.config"))
	status, _ := app.GetSaveLinks("Link Port")
	if status.Links[0].State != linkPending || status.Failed {
		t.Errorf("before the first run: %+v", status)
	}

	// The game: writes a save and exits.
	exe := filepath.Join(versionDir, "install", "game")
	writeAt(t, exe, "#!/bin/sh\nmkdir -p \"$(dirname \"$0\")/saves\"\necho slot > \"$(dirname \"$0\")/saves/slot1.sav\"\n")
	os.Chmod(exe, 0755)
	var ended = make(chan struct{})
	app.events = func(name string, _ interface{}) {
		if name == "game:ended" {
			close(ended)
		}
	}
	if err := app.LaunchVersion("Link Port", ""); err != nil {
		t.Fatal(err)
	}
	<-ended
	fileIs(t, filepath.Join(inProfile, "install", "saves", "slot1.sav"), "slot\n")
	linksTo(t, filepath.Join(versionDir, "install", "saves"), filepath.Join(inProfile, "install", "saves"))
}

// Switching profiles re-points every installed port's links there and then:
// the old profile keeps its saves, the new one starts empty.
func TestSwitchingProfilesRepointsLinksAtOnce(t *testing.T) {
	app, versionDir := linkTestApp(t, linkSpec)
	writeAt(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "sam's save")
	if err := app.InstallVersion("Link Port", map[string]string{}, "1.0", ""); err != nil {
		t.Fatal(err)
	}
	samDir := profileItemDir(t, app, "portforge")
	if _, err := app.CreateProfile("Kid"); err != nil {
		t.Fatal(err)
	}
	kidDir := profileItemDir(t, app, "kid")

	linksTo(t, filepath.Join(versionDir, "install", "saves"), filepath.Join(kidDir, "install", "saves"))
	fileIs(t, filepath.Join(samDir, "install", "saves", "slot1.sav"), "sam's save")
	if entries, _ := os.ReadDir(filepath.Join(kidDir, "install", "saves")); len(entries) != 0 {
		t.Error("the new profile should start with an empty saves folder")
	}

	if err := app.SetActiveProfile("portforge"); err != nil {
		t.Fatal(err)
	}
	linksTo(t, filepath.Join(versionDir, "install", "saves"), filepath.Join(samDir, "install", "saves"))
	fileIs(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "sam's save")
}

// A preference changed behind PortForge's back — the file edited, or copied
// from another machine — is caught up with at launch, so a game never runs
// against links that point at a profile other than the active one.
func TestLaunchRepointsLinksAtTheActiveProfile(t *testing.T) {
	app, versionDir := linkTestApp(t, linkSpec)
	writeAt(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "sam's save")
	if err := app.InstallVersion("Link Port", map[string]string{}, "1.0", ""); err != nil {
		t.Fatal(err)
	}
	samDir := profileItemDir(t, app, "portforge")
	if _, err := app.profiles.Create("Kid", "someone else"); err != nil {
		t.Fatal(err)
	}
	kidDir := profileItemDir(t, app, "kid")
	app.prefs.ActiveProfile = "kid"
	linksTo(t, filepath.Join(versionDir, "install", "saves"), filepath.Join(samDir, "install", "saves"))

	exe := filepath.Join(versionDir, "install", "game")
	writeAt(t, exe, "#!/bin/sh\nexit 0\n")
	os.Chmod(exe, 0755)
	ended := make(chan struct{})
	app.events = func(name string, _ interface{}) {
		if name == "game:ended" {
			close(ended)
		}
	}
	if err := app.LaunchVersion("Link Port", ""); err != nil {
		t.Fatal(err)
	}
	<-ended

	linksTo(t, filepath.Join(versionDir, "install", "saves"), filepath.Join(kidDir, "install", "saves"))
	linksTo(t, filepath.Join(versionDir, "install", "game.config"), filepath.Join(kidDir, "install", "game.config"))
	fileIs(t, filepath.Join(samDir, "install", "saves", "slot1.sav"), "sam's save")
	if entries, _ := os.ReadDir(filepath.Join(kidDir, "install", "saves")); len(entries) != 0 {
		t.Error("the new profile should start with an empty saves folder")
	}
	// A file path has no content yet in the new profile: the link dangles
	// until the game writes through it.
	if _, err := os.Stat(filepath.Join(kidDir, "install", "game.config")); err == nil {
		t.Error("nothing should have been invented for the config in the new profile")
	}
	if err := os.WriteFile(filepath.Join(versionDir, "install", "game.config"), []byte("kid's"), 0644); err != nil {
		t.Fatal(err)
	}
	fileIs(t, filepath.Join(kidDir, "install", "game.config"), "kid's")
}

// Data on both sides is never merged or overwritten: it is reported.
func TestDataOnBothSidesIsAConflictAndUntouched(t *testing.T) {
	app, versionDir := linkTestApp(t, linkSpec)
	writeAt(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "port side")
	if _, err := app.profiles.Ensure("portforge", "portforge"); err != nil {
		t.Fatal(err)
	}
	inProfile := profileItemDir(t, app, "portforge")
	writeAt(t, filepath.Join(inProfile, "install", "saves", "slot1.sav"), "profile side")

	if err := app.InstallVersion("Link Port", map[string]string{}, "1.0", ""); err != nil {
		t.Fatal(err)
	}
	fileIs(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "port side")
	fileIs(t, filepath.Join(inProfile, "install", "saves", "slot1.sav"), "profile side")
	if _, ok := readLink(filepath.Join(versionDir, "install", "saves")); ok {
		t.Error("a conflict must not be resolved by linking")
	}
	status, _ := app.GetSaveLinks("Link Port")
	if !status.Failed || status.Links[0].State != linkConflict || status.Links[0].Reason == "" {
		t.Errorf("GetSaveLinks = %+v", status)
	}
}

// The reinstall path: the engine sets user data aside and puts it back. With
// a link there, the link is what moves, and the build's shipped default at
// that path is discarded in favour of it.
func TestReinstallKeepsTheLinksAndTheProfileData(t *testing.T) {
	app, versionDir := linkTestApp(t, linkSpec)
	writeAt(t, filepath.Join(versionDir, "install", "game.config"), "user edited")
	for i := 0; i < 2; i++ {
		if err := app.InstallVersion("Link Port", map[string]string{}, "1.0", ""); err != nil {
			t.Fatalf("install %d: %v", i+1, err)
		}
	}
	inProfile := profileItemDir(t, app, "portforge")
	linksTo(t, filepath.Join(versionDir, "install", "game.config"), filepath.Join(inProfile, "install", "game.config"))
	fileIs(t, filepath.Join(inProfile, "install", "game.config"), "user edited")
	if _, err := os.Lstat(filepath.Join(versionDir, ".tmp-userdata")); err == nil {
		t.Error("the scratch folder was left behind")
	}
}

// Uninstalling removes the game and keeps the links — and, through them,
// nothing in the profile is touched.
func TestUninstallLeavesTheProfileAlone(t *testing.T) {
	app, versionDir := linkTestApp(t, linkSpec)
	writeAt(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")
	if err := app.InstallVersion("Link Port", map[string]string{}, "1.0", ""); err != nil {
		t.Fatal(err)
	}
	if err := app.UninstallVersion("Link Port"); err != nil {
		t.Fatal(err)
	}
	inProfile := profileItemDir(t, app, "portforge")
	fileIs(t, filepath.Join(inProfile, "install", "saves", "slot1.sav"), "save data")
	fileIs(t, filepath.Join(inProfile, "install", "game.config"), "shipped-default")
	gone(t, filepath.Join(versionDir, "install", "game"))
	linksTo(t, filepath.Join(versionDir, "install", "saves"), filepath.Join(inProfile, "install", "saves"))
}

// An install from before paths were recorded reads them from its spec.
func TestOlderInstallsReadUserDataPathsFromTheSpec(t *testing.T) {
	app, versionDir := linkTestApp(t, linkSpec)
	if err := metadata.WriteInstallState(versionDir, &models.InstallState{Installed: true, InstalledVersion: "1.0", TargetPlatform: "Linux"}); err != nil {
		t.Fatal(err)
	}
	state, _ := metadata.ReadInstallState(versionDir)
	paths, err := app.installedUserDataPaths("Link Port", versionDir, state)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths, ","); got != "install/saves,install/game.config" {
		t.Errorf("paths = %q", got)
	}
}

// The spec comes from a public catalog; a path in it must not reach outside
// the port's folder.
func TestSaveLinksRefusePathsThatLeaveThePort(t *testing.T) {
	app, versionDir := linkTestApp(t, linkSpec)
	p, _ := app.profiles.Ensure("portforge", "portforge")
	status, err := app.saveLinks("Link Port", versionDir, []string{"../other/saves", "/etc/passwd", "install/saves"}, p, false)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Failed || status.Links[0].State != linkError || status.Links[1].State != linkError || status.Links[2].State != linkPending {
		t.Errorf("status = %+v", status)
	}
	if _, err := os.Lstat(filepath.Join(app.profiles.Dir(), "other")); err == nil {
		t.Error("something was created outside the profile's item folder")
	}
}

// A profile's ItemDir guard and ours are the same rule from two sides; a
// title the module refuses is refused here too.
func TestSaveLinksSurfaceTheModulesOwnGuard(t *testing.T) {
	app, versionDir := linkTestApp(t, linkSpec)
	p := profile.Profile{Slug: "x", Path: app.profiles.Dir()}
	if _, err := app.saveLinks("../escape", versionDir, []string{"install/saves"}, p, false); err == nil {
		t.Error("a title that leaves the profile should be refused")
	}
}
