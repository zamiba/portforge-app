package main

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"portforge/metadata"
	"portforge/models"
)

// launchTestApp is userDataTestApp plus a port that wants a GameCube disc, the
// catalog entry for that disc, and an executable that records its arguments.
// The disc itself is only written when present is true.
func launchTestApp(t *testing.T, args []string, present bool) (*App, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	const item = "Melee Test · 2026"
	app, versionDir := userDataTestApp(t, item, `{"builds":[{"versions":["1.0"],"steps":[]}]}`)

	disc := []byte("the disc")
	sum := md5.Sum(disc)
	writeAt(t, filepath.Join(app.metadataPath, metadata.PortItemType, item, ".mediaitem.json"),
		`{"_itemType":"VideoGameFanPort","_itemTitle":"`+item+`","title":"Melee Test",
		  "romDependencies":[{"name":"Disc","required":true,"options":[{"_itemType":"GameCubeDiscImage","title":"Melee (USA)"}]}]}`)
	writeAt(t, filepath.Join(app.metadataPath, "GameCubeDiscImage", "Melee (USA) · GameCube", ".mediaitem.json"),
		`{"_itemType":"GameCubeDiscImage","title":"Melee (USA)","platform":"GameCube",
		  "formats":[{"filename":"melee.iso","filesize":8,"ext":"iso","checksums":{"md5":"`+hex.EncodeToString(sum[:])+`"}}]}`)
	if present {
		writeAt(t, filepath.Join(app.dataPath, "GameCubeDiscImage", "Melee (USA) · GameCube", "melee.iso"), string(disc))
	}

	exe := filepath.Join(versionDir, "install", "melee")
	writeAt(t, exe, "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$(dirname \"$0\")/argv\"\n")
	if err := os.Chmod(exe, 0755); err != nil {
		t.Fatal(err)
	}
	if err := metadata.WriteInstallState(versionDir, &models.InstallState{
		Installed:   true,
		Executables: []models.ExecutableEntry{{Path: "install/melee", Title: "Play", Args: args}},
	}); err != nil {
		t.Fatal(err)
	}
	return app, versionDir
}

// argvOf waits for the launched script to write what it was started with.
func argvOf(t *testing.T, versionDir string) string {
	t.Helper()
	path := filepath.Join(versionDir, "install", "argv")
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if b, err := os.ReadFile(path); err == nil {
			return string(b)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the executable never ran")
	return ""
}

// The install recorded ${romPath} verbatim; the launch is what turns it into
// the disc's path, wherever the disc is at that moment.
func TestLaunchResolvesTheROMWhenTheGameStarts(t *testing.T) {
	app, versionDir := launchTestApp(t, []string{"--dvd", "${romPath}", "--windowed"}, true)
	if err := app.LaunchVersion("Melee Test · 2026", ""); err != nil {
		t.Fatal(err)
	}
	disc := filepath.Join(app.dataPath, "GameCubeDiscImage", "Melee (USA) · GameCube", "melee.iso")
	if got, want := argvOf(t, versionDir), "--dvd\n"+disc+"\n--windowed\n"; got != want {
		t.Errorf("launched with %q, want %q", got, want)
	}
}

// Without the disc the game cannot be started as the spec meant it to be, and
// the launcher-less port would just crash or show nothing — say why instead.
func TestLaunchRefusesWhenTheROMItNeedsIsMissing(t *testing.T) {
	app, versionDir := launchTestApp(t, []string{"${romPath}"}, false)
	err := app.LaunchVersion("Melee Test · 2026", "")
	if err == nil || !strings.Contains(err.Error(), "needs a ROM to launch") || !strings.Contains(err.Error(), "${romPath}") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(versionDir, "install", "argv")); statErr == nil {
		t.Error("the executable was started anyway")
	}
}

// Nearly every port takes no arguments; that path must not touch the library.
func TestLaunchWithoutArgumentsPassesNone(t *testing.T) {
	app, versionDir := launchTestApp(t, nil, false)
	if err := app.LaunchVersion("Melee Test · 2026", ""); err != nil {
		t.Fatal(err)
	}
	if got := argvOf(t, versionDir); got != "\n" {
		t.Errorf("launched with %q, want no arguments", got)
	}
}

func TestLaunchNamesAnExecutableItDoesNotHave(t *testing.T) {
	app, _ := launchTestApp(t, nil, false)
	err := app.LaunchVersion("Melee Test · 2026", "install/other")
	if err == nil || !strings.Contains(err.Error(), `no executable at "install/other"`) {
		t.Errorf("err = %v", err)
	}
}
