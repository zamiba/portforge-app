package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zamiba/go-mediaitems-profiles/profilesync"
)

// The whole path against a real git: a profile that is a repository gets a
// commit when the game that wrote to it ends, authored as PortForge, and the
// outcome reaches the frontend as an event. Skipped where git is missing;
// the module's own tests cover the command lines through a fake runner.
func TestAPlaySessionIsCommittedToAProfileThatIsARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	app, versionDir := launchTestApp(t, []string{"--save-dir", "${profilePath}"}, false)
	withProfiles(t, app)
	// Profile-sync config is read from beside the profiles folder; the temp
	// folder has none, so only the implicit commit applies.
	n, err := profilesync.Open(app.profiles, "portforge", profilesync.Options{
		ConfigPath: filepath.Join(t.TempDir(), "profile-sync.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var results []map[string]interface{}
	app.events = func(name string, data interface{}) {
		if name != "profile:synced" {
			return
		}
		mu.Lock()
		results = append(results, data.(map[string]interface{}))
		mu.Unlock()
	}
	n.OnResult(func(r profilesync.Result) {
		app.emit("profile:synced", map[string]interface{}{
			"slug": r.Slug, "kind": r.Kind, "reason": r.Reason, "output": r.Output, "error": errString(r.Err),
		})
	})
	app.sync = n

	p, err := app.CreateProfile("Sam")
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(app.profiles.Dir(), p.Slug)
	// A repository with no identity of its own: the fallback is what must
	// carry the commit.
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	// A game that saves: git tracks files, not the empty folder the
	// provider made, so the commit has to have something to carry.
	exe := filepath.Join(versionDir, "install", "melee")
	writeAt(t, exe, "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$(dirname \"$0\")/argv\"\necho saved > \"$2/save.dat\"\n")
	if err := os.Chmod(exe, 0755); err != nil {
		t.Fatal(err)
	}

	if err := app.LaunchVersion("Melee Test · 2026", ""); err != nil {
		t.Fatal(err)
	}
	argvOf(t, versionDir) // the game ran; its exit triggers the sync

	deadline := time.Now().Add(10 * time.Second)
	for {
		mu.Lock()
		n := len(results)
		mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(results) != 1 {
		t.Fatalf("results = %v, want one", results)
	}
	r := results[0]
	if r["error"] != "" || r["kind"] != "git" || r["slug"] != "sam" || r["output"] != "committed" {
		t.Errorf("result = %v", r)
	}
	if got := git("log", "-1", "--format=%an <%ae>%n%s"); got != "portforge <portforge@localhost>\nportforge: game ended: Melee Test · 2026" {
		t.Errorf("last commit:\n%s", got)
	}
	// The commit holds the profile's data, not only its metadata.
	if got := git("-c", "core.quotepath=off", "ls-files"); !strings.Contains(got, "profile.json") || !strings.Contains(got, "MediaItems/VideoGameFanPort/Melee Test · 2026/save.dat") {
		t.Errorf("committed files:\n%s", got)
	}
	if status := git("status", "--porcelain"); status != "" {
		t.Errorf("uncommitted after the sync:\n%s", status)
	}
}
