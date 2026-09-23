package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zamiba/forge/engine"
	"github.com/zamiba/go-mediaitems-profiles/profile"

	"portforge/metadata"
	"portforge/models"
)

// A port that writes to a per-user folder, as melee-pc does, and takes no
// flag: the spec names the folder by its location type and the path beneath
// it, in the same list as any path inside the port's folder.
const outsideSpec = `{
  "userDataPaths": [
    { "locationType": "linuxData", "path": "link-port" },
    { "locationType": "windowsRoaming", "path": "link-port" }
  ],
  "builds": [{
    "targetPlatforms": ["Linux", "Mac", "Windows"],
    "versions": ["1.0"],
    "dependencies": ["sh"],
    "steps": [
      { "step": "createDir", "path": "install" },
      { "step": "run", "cmd": "sh", "args": ["-c", "printf binary > install/game"] },
      { "step": "defineExecutable", "executable": "install/game", "title": "Play" }
    ]
  }]
}`

// outsideTestApp is linkTestApp with the per-user folders pointed at
// temporary ones, the way the ports themselves honour XDG_*_HOME. Linux only:
// the other platforms' folders are fixed locations under the home folder.
func outsideTestApp(t *testing.T) (*App, string, string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the locations are redirected through XDG_*_HOME")
	}
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	app, versionDir := linkTestApp(t, outsideSpec)
	return app, versionDir, home
}

func TestEntriesOutsideTheTreeAreLinkedIntoTheProfile(t *testing.T) {
	app, _, home := outsideTestApp(t)
	outside := filepath.Join(home, "data", "link-port")
	writeAt(t, filepath.Join(outside, "card.gci"), "memory card")

	if err := app.InstallVersion("Link Port", map[string]string{}, "1.0", ""); err != nil {
		t.Fatal(err)
	}
	inProfile := profileItemDir(t, app, "portforge")
	// The data moved into the profile under the entry's path — the same
	// place a Windows machine's windowsRoaming entry would use, so a profile
	// shared between the two holds one set of saves — and the port's place
	// is a link to it: the game still finds its card where it looks.
	fileIs(t, filepath.Join(inProfile, "link-port", "card.gci"), "memory card")
	linksTo(t, outside, filepath.Join(inProfile, "link-port"))
	fileIs(t, filepath.Join(outside, "card.gci"), "memory card")

	status, err := app.GetSaveLinks("Link Port")
	if err != nil {
		t.Fatal(err)
	}
	// The Windows entry is not this platform's business.
	if status.Failed || len(status.Links) != 1 || status.Links[0].State != linkLinked {
		t.Fatalf("GetSaveLinks = %+v", status)
	}
	if status.Links[0].Path != "linuxData:link-port" || status.Links[0].Location != outside {
		t.Errorf("link = %+v", status.Links[0])
	}

	// A switch re-points the link like any other; the first profile keeps
	// its card and the new one starts without.
	if _, err := app.CreateProfile("Kid"); err != nil {
		t.Fatal(err)
	}
	kidDir := profileItemDir(t, app, "kid")
	linksTo(t, outside, filepath.Join(kidDir, "link-port"))
	fileIs(t, filepath.Join(inProfile, "link-port", "card.gci"), "memory card")
	if _, err := os.Stat(filepath.Join(outside, "card.gci")); err == nil {
		t.Error("the new profile should not see the old profile's card")
	}
}

// The engine keeps an entry beneath its folder; PortForge adds that the place
// must not be one of its own, which hold every profile.
func TestEntriesOutsideTheTreeCannotReachPortForgesOwnFolders(t *testing.T) {
	app, versionDir, home := outsideTestApp(t)
	// Profiles where they really live: <config>/MediaItem/profiles.
	m, err := profile.New(profile.Options{Dir: filepath.Join(home, "config", "MediaItem", "profiles")})
	if err != nil {
		t.Fatal(err)
	}
	app.profiles = m
	p, _ := app.profiles.Ensure("portforge", "portforge")

	entries := []engine.UserDataPath{
		{LocationType: "linuxConfig", Path: "MediaItem"},          // every profile
		{LocationType: "linuxConfig", Path: "MediaItem/profiles"}, // likewise
		{LocationType: "linuxConfig", Path: "PortForge"},          // PortForge's own settings
		{LocationType: "linuxData", Path: "fine"},                 // the one acceptable entry
	}
	status, err := app.saveLinks("Link Port", versionDir, nil, entries, p, false)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range status.Links[:len(entries)-1] {
		if l.State != linkError || !strings.Contains(l.Reason, "PortForge keeps its own data") {
			t.Errorf("entry %d %+v: %s %q, want an error", i, entries[i], l.State, l.Reason)
		}
	}
	if last := status.Links[len(entries)-1]; last.State != linkPending {
		t.Errorf("the acceptable entry: %+v", last)
	}
	if _, err := os.Lstat(filepath.Join(home, "config", "MediaItem", "profiles", "portforge", "MediaItem")); err == nil {
		t.Error("something was created at a refused place")
	}
}

// Uninstalling removes the link PortForge planted outside the port's folder
// and leaves the data where it is, in the profile.
func TestUninstallRemovesTheLinkOutsideThePort(t *testing.T) {
	app, _, home := outsideTestApp(t)
	outside := filepath.Join(home, "data", "link-port")
	writeAt(t, filepath.Join(outside, "card.gci"), "memory card")
	if err := app.InstallVersion("Link Port", map[string]string{}, "1.0", ""); err != nil {
		t.Fatal(err)
	}
	inProfile := profileItemDir(t, app, "portforge")
	linksTo(t, outside, filepath.Join(inProfile, "link-port"))

	if err := app.UninstallVersion("Link Port"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(outside); !os.IsNotExist(err) {
		t.Errorf("the link should be gone, got %v", err)
	}
	fileIs(t, filepath.Join(inProfile, "link-port", "card.gci"), "memory card")
}

// A folder at the port's place that is not PortForge's link — the port's own
// data, never linked — is not the uninstall's to remove.
func TestUninstallLeavesUnlinkedDataOutsideThePort(t *testing.T) {
	app, versionDir, home := outsideTestApp(t)
	outside := filepath.Join(home, "data", "link-port")
	if err := metadata.WriteInstallState(versionDir, &models.InstallState{Installed: true, InstalledVersion: "1.0", TargetPlatform: "Linux"}); err != nil {
		t.Fatal(err)
	}
	writeAt(t, filepath.Join(outside, "card.gci"), "memory card")
	if err := app.UninstallVersion("Link Port"); err != nil {
		t.Fatal(err)
	}
	fileIs(t, filepath.Join(outside, "card.gci"), "memory card")
}

// The catalog's spec is what linking reads, so a port whose spec learns where
// its saves are after the install reaches that install too.
func TestLinkingReadsTheCatalogSpecOverTheRecordedPaths(t *testing.T) {
	app, versionDir := linkTestApp(t, linkSpec)
	if err := metadata.WriteInstallState(versionDir, &models.InstallState{Installed: true, InstalledVersion: "1.0", TargetPlatform: "Linux", UserDataPaths: []string{"install/saves"}}); err != nil {
		t.Fatal(err)
	}
	state, _ := metadata.ReadInstallState(versionDir)
	paths, _, err := app.installedUserData("Link Port", versionDir, state)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths, ","); got != "install/saves,install/game.config" {
		t.Errorf("paths = %q", got)
	}
	// A version the catalog no longer has falls back to what was recorded.
	state.InstalledVersion = "0.9"
	paths, _, err = app.installedUserData("Link Port", versionDir, state)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths, ","); got != "install/saves" {
		t.Errorf("paths = %q", got)
	}
}
