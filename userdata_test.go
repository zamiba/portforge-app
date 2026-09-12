package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"portforge/metadata"
)

// userDataTestApp builds an App over two temp directories: a catalog holding
// one port's spec file, and a data directory holding what that port installed.
func userDataTestApp(t *testing.T, itemTitle, specJSON string) (*App, string) {
	t.Helper()
	catalog, data := t.TempDir(), t.TempDir()

	specDir := filepath.Join(catalog, metadata.PortItemType, itemTitle)
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, metadata.SpecFileName), []byte(specJSON), 0644); err != nil {
		t.Fatal(err)
	}
	// Installing loads the MediaItem itself; uninstalling tolerates its absence
	// but there is no reason for the two paths to differ here.
	item := `{"_itemType":"VideoGameFanPort","_itemTitle":"` + itemTitle + `","title":"` + itemTitle + `"}`
	if err := os.WriteFile(filepath.Join(specDir, ".mediaitem.json"), []byte(item), 0644); err != nil {
		t.Fatal(err)
	}

	app := &App{
		ctx:          context.Background(),
		metadataPath: catalog,
		dataPath:     data,
		// Swallow events rather than reaching for a Wails runtime that is not
		// there. See events.go.
		events: func(string, interface{}) {},
	}
	return app, filepath.Join(data, metadata.PortItemType, itemTitle)
}

func writeAt(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func fileIs(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(b) != want {
		t.Errorf("%s = %q, want %q", filepath.Base(path), b, want)
	}
}

func gone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err == nil {
		t.Errorf("%s should have been removed", path)
	}
}

const userDataSpec = `{
  "userDataPaths": ["install/saves", "install/game.config"],
  "uninstallSteps": [{ "step": "deletePath", "path": "install" }],
  "builds": [{
    "targetPlatforms": ["Linux", "Mac", "Windows"],
    "versions": ["1.0"],
    "steps": []
  }]
}`

// The spec declares no teardown at all, so PortForge supplies the default one.
// It must still honour the preserved paths.
const userDataSpecNoUninstall = `{
  "userDataPaths": ["install/saves"],
  "builds": [{
    "targetPlatforms": ["Linux", "Mac", "Windows"],
    "versions": ["1.0"],
    "steps": []
  }]
}`

func TestUninstallKeepsPreservedUserData(t *testing.T) {
	app, versionDir := userDataTestApp(t, "Test Port", userDataSpec)
	writeAt(t, filepath.Join(versionDir, "install", "game"), "binary")
	writeAt(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")
	writeAt(t, filepath.Join(versionDir, "install", "game.config"), "fullscreen=1")

	if err := app.UninstallVersion("Test Port"); err != nil {
		t.Fatalf("UninstallVersion: %v", err)
	}

	fileIs(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")
	fileIs(t, filepath.Join(versionDir, "install", "game.config"), "fullscreen=1")
	gone(t, filepath.Join(versionDir, "install", "game"))

	state, err := metadata.ReadInstallState(versionDir)
	if err != nil {
		t.Fatalf("ReadInstallState: %v", err)
	}
	if state == nil || state.Installed {
		t.Error("the port should be marked uninstalled even though files remain")
	}
}

// The gap that made the list file-level rather than step-level: a spec with no
// uninstallSteps used to be torn down by a plain RemoveAll with no way to
// declare an exception.
func TestUninstallWithoutDeclaredStepsStillKeepsUserData(t *testing.T) {
	app, versionDir := userDataTestApp(t, "Test Port", userDataSpecNoUninstall)
	writeAt(t, filepath.Join(versionDir, "install", "game"), "binary")
	writeAt(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")

	if err := app.UninstallVersion("Test Port"); err != nil {
		t.Fatalf("UninstallVersion: %v", err)
	}

	fileIs(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")
	gone(t, filepath.Join(versionDir, "install", "game"))
}

// A port that declares nothing to preserve is torn down exactly as before.
func TestUninstallWithoutUserDataPathsRemovesEverything(t *testing.T) {
	app, versionDir := userDataTestApp(t, "Test Port", `{
	  "uninstallSteps": [{ "step": "deletePath", "path": "install" }],
	  "builds": [{
	    "targetPlatforms": ["Linux", "Mac", "Windows"],
	    "versions": ["1.0"],
	    "steps": []
	  }]
	}`)
	writeAt(t, filepath.Join(versionDir, "install", "game"), "binary")
	writeAt(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")

	if err := app.UninstallVersion("Test Port"); err != nil {
		t.Fatalf("UninstallVersion: %v", err)
	}
	gone(t, filepath.Join(versionDir, "install"))
}

// An update runs the build over the previous install. A build that ships its
// own copy of a file the user has edited must not win.
func TestInstallOverAnExistingInstallKeepsEditedUserData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the spec below uses a POSIX shell command")
	}
	// The build writes both a program file and its own default config, the
	// second of which collides with the preserved one.
	spec := `{
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
	app, versionDir := userDataTestApp(t, "Test Port", spec)

	// What the previous install left behind, with the user's own edits.
	writeAt(t, filepath.Join(versionDir, "install", "game"), "old binary")
	writeAt(t, filepath.Join(versionDir, "install", "game.config"), "user edited")
	writeAt(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")

	if err := app.InstallVersion("Test Port", map[string]string{}, "1.0", ""); err != nil {
		t.Fatalf("InstallVersion: %v", err)
	}

	fileIs(t, filepath.Join(versionDir, "install", "game"), "binary")
	fileIs(t, filepath.Join(versionDir, "install", "game.config"), "user edited")
	fileIs(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")
}

// A build that dies partway must not leave user data stranded in the scratch
// directory.
func TestFailedInstallRestoresPreservedUserData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the spec below uses a POSIX shell command")
	}
	spec := `{
	  "userDataPaths": ["install/saves"],
	  "builds": [{
	    "targetPlatforms": ["Linux", "Mac", "Windows"],
	    "versions": ["1.0"],
	    "dependencies": ["sh"],
	    "steps": [
	      { "step": "run", "cmd": "sh", "args": ["-c", "exit 1"] }
	    ]
	  }]
	}`
	app, versionDir := userDataTestApp(t, "Test Port", spec)
	writeAt(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")

	if err := app.InstallVersion("Test Port", map[string]string{}, "1.0", ""); err == nil {
		t.Fatal("expected the build to fail")
	}

	fileIs(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")
}

// A spec written before braces became mandatory builds the wrong thing rather
// than failing, so installing one has to be refused outright.
func TestInstallRejectsUnbracedSpecVariables(t *testing.T) {
	app, _ := userDataTestApp(t, "Test Port", `[{
	  "targetPlatforms": ["Linux", "Mac", "Windows"],
	  "versions": ["1.0"],
	  "steps": [{ "step": "createDir", "path": "install/$platform" }]
	}]`)

	err := app.InstallVersion("Test Port", map[string]string{}, "1.0", "")
	if err == nil {
		t.Fatal("expected the install to be refused")
	}
	if !strings.Contains(err.Error(), "without braces") {
		t.Errorf("error should explain the problem: %v", err)
	}
}

// A spec that will not parse is the one that would have named the paths to
// spare, so uninstalling has to stop rather than fall back to removing
// everything.
func TestUninstallRefusesWhenTheSpecCannotBeRead(t *testing.T) {
	app, versionDir := userDataTestApp(t, "Test Port", `{ "builds": [ nonsense`)
	writeAt(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")

	err := app.UninstallVersion("Test Port")
	if err == nil {
		t.Fatal("expected the uninstall to be refused")
	}
	if !strings.Contains(err.Error(), "save data") {
		t.Errorf("the error should say why it stopped: %v", err)
	}
	fileIs(t, filepath.Join(versionDir, "install", "saves", "slot1.sav"), "save data")
}
