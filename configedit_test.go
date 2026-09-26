package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"portforge/metadata"

	"github.com/zamiba/config-forge/schema"
	"github.com/zamiba/go-mediaitems-profiles/profile"
)

// newConfigApp lays out a port with a schema and, optionally, the config file the
// game would have written.
func newConfigApp(t *testing.T, writeFile bool) *App {
	t.Helper()
	meta, data := t.TempDir(), t.TempDir()
	item := "Gen1Recomp · 2026"

	metaDir := filepath.Join(meta, metadata.PortItemType, item, metadata.ConfigSchemaDir)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const sch = `{
	  "title": "Options",
	  "path": "install/options.lua",
	  "format": "luaTable",
	  "sections": [{
	    "title": "Audio",
	    "fields": [
	      { "pointer": "musicVol", "kind": "int", "widget": "slider", "label": "Music", "min": 0, "max": 7, "unit": " / 7", "default": 7 },
	      { "pointer": "animations", "kind": "bool", "widget": "toggle", "label": "Animations", "default": true },
	      { "pointer": "battleStyle", "kind": "string", "widget": "select", "label": "Battle style", "default": "shift",
	        "options": [{ "label": "Shift", "value": "shift" }, { "label": "Set", "value": "set" }] },
	      { "pointer": "sfxVol", "kind": "int", "widget": "slider", "label": "Sound", "min": 0, "max": 7, "default": 7 },
	      { "pointer": "gone", "kind": "int", "widget": "number", "label": "Removed upstream" },
	      { "pointer": "cartOptions", "kind": "opaque", "label": "Per-cart settings", "readOnly": true }
	    ]
	  }]
	}`
	if err := os.WriteFile(filepath.Join(metaDir, "options.schema.json"), []byte(sch), 0o644); err != nil {
		t.Fatal(err)
	}
	// A spec, so the schema's path is declared user data and versions resolve.
	const spec = `{
	  "userDataPaths": [{ "locationType": "runDir", "path": "install/options.lua" }],
	  "builds": [{ "versions": ["0.2.55"], "targetPlatforms": ["Linux"], "steps": [] }]
	}`
	if err := os.WriteFile(filepath.Join(meta, metadata.PortItemType, item, ".forge.json"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	if writeFile {
		dir := filepath.Join(data, metadata.PortItemType, item, "install")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		const cfg = `return {
  animations = true,
  battleStyle = "shift",
  cartOptions = {},
  mods = {},
  musicVol = 7,
}
`
		if err := os.WriteFile(filepath.Join(dir, "options.lua"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &App{metadataPath: meta, dataPath: data}
}

func findField(c GameConfig, pointer string) (ConfigField, bool) {
	for _, f := range c.Files {
		for _, sec := range f.Sections {
			for _, fl := range sec.Fields {
				if fl.Pointer == pointer {
					return fl, true
				}
			}
		}
	}
	return ConfigField{}, false
}

func fieldOf(t *testing.T, c GameConfig, pointer string) ConfigField {
	t.Helper()
	fl, ok := findField(c, pointer)
	if !ok {
		t.Fatalf("no field %s", pointer)
	}
	return fl
}

func TestGetGameConfigResolvesAgainstTheFile(t *testing.T) {
	app := newConfigApp(t, true)
	c, err := app.GetGameConfig("Gen1Recomp · 2026")
	if err != nil {
		t.Fatal(err)
	}
	if c.State != configNormal {
		t.Errorf("state = %q, want normal", c.State)
	}
	if len(c.Files) != 1 || !c.Files[0].Exists {
		t.Fatalf("files = %+v", c.Files)
	}

	// Values arrive as the types they are, not as strings.
	if v := fieldOf(t, c, "musicVol").Value; v != int64(7) {
		t.Errorf("musicVol = %#v, want int64(7)", v)
	}
	if v := fieldOf(t, c, "animations").Value; v != true {
		t.Errorf("animations = %#v, want true", v)
	}
	if v := fieldOf(t, c, "battleStyle").Value; v != "shift" {
		t.Errorf("battleStyle = %#v, want \"shift\"", v)
	}
	if u := fieldOf(t, c, "musicVol").Unit; u != " / 7" {
		t.Errorf("unit = %q", u)
	}

	// A setting the file has lost is not offered at all. Nobody can act on a row
	// that says a release stores something differently, so the page does not carry
	// one; what was dropped goes to the log instead.
	if _, found := findField(c, "gone"); found {
		t.Error("a setting this release cannot edit should not reach the page")
	}

	// A read-only field carries its literal and is not editable.
	ro := fieldOf(t, c, "cartOptions")
	if !ro.ReadOnly || ro.Editable || ro.Display != "{}" || ro.Reason != "" {
		t.Errorf("cartOptions = %+v", ro)
	}

	// Options carry typed values too, so a select round-trips without parsing.
	opts := fieldOf(t, c, "battleStyle").Options
	if len(opts) != 2 || opts[0].Value != "shift" {
		t.Errorf("options = %+v", opts)
	}
}

// A game that has never run has written no config, and the page is told once
// rather than per field.
func TestGetGameConfigWhenTheGameHasNeverRun(t *testing.T) {
	app := newConfigApp(t, false)
	c, err := app.GetGameConfig("Gen1Recomp · 2026")
	if err != nil {
		t.Fatal(err)
	}
	if c.State != configNeverLaunched {
		t.Errorf("state = %q, want neverLaunched", c.State)
	}
	if len(c.Files) != 1 || c.Files[0].Exists {
		t.Fatalf("files = %+v", c.Files)
	}
	// The fields are still described, so the page can show the form disabled.
	if fl := fieldOf(t, c, "musicVol"); fl.Editable || fl.Reason != "missing" {
		t.Errorf("musicVol = %+v", fl)
	}
}

// A port with no schemas has no config tab at all.
func TestGetGameConfigWithoutSchemas(t *testing.T) {
	app := newConfigApp(t, true)
	c, err := app.GetGameConfig("Something Else · 2026")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Files) != 0 {
		t.Errorf("files = %+v", c.Files)
	}
}

// Saving changes the settings it names, leaves every byte it does not name as the
// game wrote it, and records the settings the game has built in but not written.
func TestSaveGameConfigWritesItsChangesAndNothingElseOfTheGames(t *testing.T) {
	app := newConfigApp(t, true)
	path := filepath.Join(app.dataPath, metadata.PortItemType, "Gen1Recomp · 2026", "install", "options.lua")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	err = app.SaveGameConfig("Gen1Recomp · 2026", []ConfigChange{
		{File: "install/options.lua", Pointer: "musicVol", Value: json.RawMessage(`3`)},
		{File: "install/options.lua", Pointer: "battleStyle", Value: json.RawMessage(`"set"`)},
		{File: "install/options.lua", Pointer: "animations", Value: json.RawMessage(`false`)},
	})
	if err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := string(before)
	for from, to := range map[string]string{
		"musicVol = 7":          "musicVol = 3",
		`battleStyle = "shift"`: `battleStyle = "set"`,
		"animations = true":     "animations = false",
	} {
		want = replaceOnce(want, from, to)
	}
	// sfxVol is in the schema with the game's default but not in the file, so the
	// save records it as well; everything else must match byte for byte.
	want = replaceOnce(want, "  musicVol = 3,\n}", "  musicVol = 3,\n  sfxVol = 7,\n}")
	if string(after) != want {
		t.Errorf("got:\n%s\nwant:\n%s", after, want)
	}
	// The game's own state is still there, untouched.
	if !contains(string(after), "mods = {},") || !contains(string(after), "cartOptions = {},") {
		t.Error("the game's own tables did not survive the write")
	}
}

func TestSaveGameConfigRefusesWhatItShould(t *testing.T) {
	app := newConfigApp(t, true)
	item := "Gen1Recomp · 2026"

	for name, change := range map[string]ConfigChange{
		"unknown file":     {File: "install/nope.lua", Pointer: "musicVol", Value: json.RawMessage(`1`)},
		"unknown setting":  {File: "install/options.lua", Pointer: "nope", Value: json.RawMessage(`1`)},
		"above the max":    {File: "install/options.lua", Pointer: "musicVol", Value: json.RawMessage(`9`)},
		"wrong json type":  {File: "install/options.lua", Pointer: "musicVol", Value: json.RawMessage(`"seven"`)},
		"unoffered option": {File: "install/options.lua", Pointer: "battleStyle", Value: json.RawMessage(`"rotate"`)},
		"read-only field":  {File: "install/options.lua", Pointer: "cartOptions", Value: json.RawMessage(`"{}"`)},
		"missing setting":  {File: "install/options.lua", Pointer: "gone", Value: json.RawMessage(`1`)},
	} {
		if err := app.SaveGameConfig(item, []ConfigChange{change}); err == nil {
			t.Errorf("%s should have been refused", name)
		}
	}

	// A refused batch leaves the file as it was.
	path := filepath.Join(app.dataPath, metadata.PortItemType, item, "install", "options.lua")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), "musicVol = 7") {
		t.Errorf("a refused save changed the file:\n%s", data)
	}
}

// The running game owns its config: it rewrites the file as it goes, so an edit
// would be lost or land mid-write.
func TestSaveGameConfigRefusesWhileTheGameRuns(t *testing.T) {
	app := newConfigApp(t, true)
	item := "Gen1Recomp · 2026"
	app.playing = item

	c, err := app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	if c.State != configRunning || c.Running != item {
		t.Errorf("state = %q, running = %q", c.State, c.Running)
	}
	if err := app.SaveGameConfig(item, []ConfigChange{
		{File: "install/options.lua", Pointer: "musicVol", Value: json.RawMessage(`1`)},
	}); err == nil {
		t.Error("saving while the game runs should be refused")
	}

	// A different game running is not this game's problem.
	app.playing = "Snap64 Recomp · 2026"
	if c, err := app.GetGameConfig(item); err != nil || c.State != configNormal {
		t.Errorf("state = %q, err = %v; another game running should not lock this page", c.State, err)
	}
}

// The file is a symlink into the profile, which is the normal case once a profile
// is in use. Writing must follow the link rather than replace it.
func TestSaveGameConfigKeepsTheProfileSymlink(t *testing.T) {
	app := newConfigApp(t, true)
	item := "Gen1Recomp · 2026"
	path := filepath.Join(app.dataPath, metadata.PortItemType, item, "install", "options.lua")

	// Move the file into a stand-in profile folder and link it back, the way
	// save linking does.
	profileDir := filepath.Join(t.TempDir(), "profile")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(profileDir, "options.lua")
	if err := os.Rename(path, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, path); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	if err := app.SaveGameConfig(item, []ConfigChange{
		{File: "install/options.lua", Pointer: "musicVol", Value: json.RawMessage(`2`)},
	}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the write replaced the profile symlink with a regular file")
	}
	data, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), "musicVol = 2") {
		t.Errorf("the edit did not reach the profile's copy:\n%s", data)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && indexOfStr(s, sub) >= 0 }

func indexOfStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func replaceOnce(s, from, to string) string {
	i := indexOfStr(s, from)
	if i < 0 {
		return s
	}
	return s[:i] + to + s[i+len(from):]
}

// A setting the game has built in is the game's, so recording it at the value the
// game already uses leaves the file complete and changes nothing about how the
// game runs. Saving completes the files it writes.
func TestSaveGameConfigFillsTheSettingsTheGameHasNotWritten(t *testing.T) {
	app := newConfigApp(t, true)
	item := "Gen1Recomp · 2026"
	path := filepath.Join(app.dataPath, metadata.PortItemType, item, "install", "options.lua")

	// sfxVol is in the schema with a default but not in the file.
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if contains(string(before), "sfxVol") {
		t.Fatal("the fixture should not already hold sfxVol")
	}
	c, err := app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	if fl := fieldOf(t, c, "sfxVol"); !fl.Unset || !fl.Editable || fl.Value != int64(7) {
		t.Fatalf("sfxVol before saving = %+v", fl)
	}

	if err := app.SaveGameConfig(item, []ConfigChange{
		{File: "install/options.lua", Pointer: "musicVol", Value: json.RawMessage(`3`)},
	}); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(after), "sfxVol = 7") {
		t.Errorf("the unwritten setting was not filled in:\n%s", after)
	}
	if !contains(string(after), "musicVol = 3") {
		t.Errorf("the edit did not land:\n%s", after)
	}
	// The game's own tables are still untouched, and a setting with no default is
	// still not invented.
	for _, want := range []string{"mods = {},", "cartOptions = {},"} {
		if !contains(string(after), want) {
			t.Errorf("%q did not survive", want)
		}
	}
	if contains(string(after), "gone") {
		t.Error("a setting with no default should not be written")
	}
	// Reading it back, nothing is unset any more.
	c, err = app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	if fl := fieldOf(t, c, "sfxVol"); fl.Unset || fl.Value != int64(7) {
		t.Errorf("sfxVol after saving = %+v", fl)
	}
}

// The same filling, without editing anything first.
func TestFillConfigDefaults(t *testing.T) {
	app := newConfigApp(t, true)
	item := "Gen1Recomp · 2026"
	path := filepath.Join(app.dataPath, metadata.PortItemType, item, "install", "options.lua")

	if err := app.FillConfigDefaults(item); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), "sfxVol = 7") {
		t.Errorf("not filled:\n%s", data)
	}
	// Values the game had written keep what they were.
	if !contains(string(data), "musicVol = 7") || !contains(string(data), `battleStyle = "shift"`) {
		t.Errorf("an existing value was changed:\n%s", data)
	}

	// Running it again writes nothing, since nothing is unset any more.
	first := string(data)
	if err := app.FillConfigDefaults(item); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != first {
		t.Errorf("filling twice changed the file the second time:\n%s", again)
	}
}

// A file the game has never created is not authored by PortForge: it fills
// settings inside a file, not the file itself.
func TestFillConfigDefaultsLeavesAMissingFileAlone(t *testing.T) {
	app := newConfigApp(t, false)
	item := "Gen1Recomp · 2026"
	if err := app.FillConfigDefaults(item); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(app.dataPath, metadata.PortItemType, item, "install", "options.lua")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the file should not have been created: %v", err)
	}
}

// Version bounds are compared against the spec's own declaration order, oldest
// first. Reversing it — which GetSpecVersions does, for the picker — inverts every
// bound and applies it to the wrong releases, silently.
func TestDeclaredVersionsAreOldestFirstAndBoundFields(t *testing.T) {
	meta, data := t.TempDir(), t.TempDir()
	item := "Two Versions · 2026"
	dir := filepath.Join(meta, metadata.PortItemType, item)
	if err := os.MkdirAll(filepath.Join(dir, metadata.ConfigSchemaDir), 0o755); err != nil {
		t.Fatal(err)
	}
	// Declared oldest first, as a spec does.
	const spec = `{
	  "userDataPaths": [{ "locationType": "runDir", "path": "install/a.cfg" }],
	  "builds": [
	    { "versions": ["1.0.2"], "targetPlatforms": ["Linux"], "steps": [] },
	    { "versions": ["1.1 RC4"], "targetPlatforms": ["Linux"], "steps": [] }
	  ]
	}`
	if err := os.WriteFile(filepath.Join(dir, ".forge.json"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	const sch = `{
	  "title": "Settings", "path": "install/a.cfg", "format": "godot",
	  "sections": [{ "title": "Video", "fields": [
	    { "pointer": "video.mode", "kind": "int", "widget": "number", "label": "Mode", "default": 0 },
	    { "pointer": "video.scale", "kind": "int", "widget": "number", "label": "Scale", "default": 3, "sinceVersion": "1.1 RC4" }
	  ]}]
	}`
	if err := os.WriteFile(filepath.Join(dir, metadata.ConfigSchemaDir, "a.schema.json"), []byte(sch), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{metadataPath: meta, dataPath: data}
	if got := app.declaredVersions(item); len(got) != 2 || got[0] != "1.0.2" || got[1] != "1.1 RC4" {
		t.Fatalf("declaredVersions = %v, want [1.0.2 1.1 RC4]", got)
	}

	// The install state decides which release's settings are offered.
	install := filepath.Join(data, metadata.PortItemType, item)
	if err := os.MkdirAll(filepath.Join(install, "install"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(install, "install", "a.cfg"), []byte("[video]\nmode=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for version, want := range map[string]int{"1.0.2": 1, "1.1 RC4": 2} {
		stateDir := filepath.Join(install, ".state")
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			t.Fatal(err)
		}
		meta := `{"installed":true,"installedVersion":` + quote(version) + `}`
		if err := os.WriteFile(filepath.Join(stateDir, "meta.json"), []byte(meta), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := app.GetGameConfig(item)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, f := range c.Files {
			for _, sec := range f.Sections {
				n += len(sec.Fields)
			}
		}
		if n != want {
			t.Errorf("%s offers %d fields, want %d", version, n, want)
		}
	}
}

func quote(s string) string { return `"` + s + `"` }

// A port that keeps its config outside its own folder — inside an OS data
// directory, or written straight into the profile through ${profilePath} — has
// nothing at the path under the port's folder. Its config file is still in the
// profile, under the same relative path, because that is where saveLinks puts
// the real bytes of every entry whatever its locationType.
func TestConfigFilePathFallsBackToTheProfile(t *testing.T) {
	app := newConfigApp(t, false)
	item := "Gen1Recomp · 2026"

	mgr, err := profile.New(profile.Options{Dir: filepath.Join(t.TempDir(), "profiles")})
	if err != nil {
		t.Fatal(err)
	}
	p, err := mgr.Create("Sam", "test")
	if err != nil {
		t.Fatal(err)
	}
	app.profiles = mgr
	app.prefs.ActiveProfile = p.Slug

	inPort := filepath.Join(app.dataPath, metadata.PortItemType, item, "install", "options.lua")

	// Nothing anywhere: the port's own folder is what gets named, since that is
	// where the program would create the file.
	if got := app.configFilePath(item, "install/options.lua"); got != inPort {
		t.Errorf("with the file nowhere, got %q, want the port's folder %q", got, inPort)
	}

	// Only in the profile: that is the one that gets read.
	itemDir, err := p.ItemDir(metadata.PortItemType, item)
	if err != nil {
		t.Fatal(err)
	}
	inProfile := filepath.Join(itemDir, "install", "options.lua")
	if err := os.MkdirAll(filepath.Dir(inProfile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inProfile, []byte("return {\n  musicVol = 4,\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := app.configFilePath(item, "install/options.lua"); got != inProfile {
		t.Errorf("with the file only in the profile, got %q, want %q", got, inProfile)
	}

	// And it is genuinely readable through the whole page, not just resolvable.
	cfg, err := app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Files) != 1 || !cfg.Files[0].Exists {
		t.Fatalf("the page did not find the profile's copy: %+v", cfg.Files)
	}
	if got := fieldOf(t, cfg, "musicVol").Display; got != "4" {
		t.Errorf("musicVol = %s, want 4", got)
	}

	// The port's own folder still wins when the file is there — which is the case
	// where the port's data was never linked, and resolving only against the
	// profile would report a file that exists as one the game had not written.
	if err := os.MkdirAll(filepath.Dir(inPort), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inPort, []byte("return {\n  musicVol = 1,\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := app.configFilePath(item, "install/options.lua"); got != inPort {
		t.Errorf("with the file in both, got %q, want the port's folder %q", got, inPort)
	}
}

// A broken link counts as the file being there: Lstat sees the link itself, so a
// port whose data is linked to a profile that has since gone is reported against
// its own folder rather than being looked for somewhere it never was.
func TestConfigFilePathNamesABrokenLink(t *testing.T) {
	app := newConfigApp(t, false)
	item := "Gen1Recomp · 2026"
	inPort := filepath.Join(app.dataPath, metadata.PortItemType, item, "install", "options.lua")
	if err := os.MkdirAll(filepath.Dir(inPort), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "gone", "options.lua"), inPort); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	if got := app.configFilePath(item, "install/options.lua"); got != inPort {
		t.Errorf("got %q, want %q", got, inPort)
	}
}

// A setting this release cannot edit is left out of the page rather than shown
// greyed with a reason. A section that loses every setting goes with it, and a file
// that loses every section says so once about the file.
func TestGetGameConfigLeavesOutWhatCannotBeEdited(t *testing.T) {
	meta, data := t.TempDir(), t.TempDir()
	item := "Test Port · 2026"
	metaDir := filepath.Join(meta, metadata.PortItemType, item, metadata.ConfigSchemaDir)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Three sections: one editable, one whose only setting the file stores as
	// another kind, one whose only setting is absent with no default to add.
	const sch = `{
	  "title": "Settings",
	  "path": "install/settings.cfg",
	  "format": "godot",
	  "sections": [
	    { "title": "Video", "fields": [
	      { "pointer": "video.mode", "kind": "int", "widget": "number", "label": "Mode" }
	    ] },
	    { "title": "Changed", "fields": [
	      { "pointer": "video.vsync", "kind": "string", "widget": "text", "label": "V-sync" }
	    ] },
	    { "title": "Absent", "fields": [
	      { "pointer": "video.hdr", "kind": "bool", "widget": "toggle", "label": "HDR" }
	    ] }
	  ]
	}`
	if err := os.WriteFile(filepath.Join(metaDir, "settings.schema.json"), []byte(sch), 0o644); err != nil {
		t.Fatal(err)
	}
	const spec = `{
	  "userDataPaths": [{ "locationType": "runDir", "path": "install/settings.cfg" }],
	  "builds": [{ "versions": ["1.0"], "targetPlatforms": ["Linux"], "steps": [] }]
	}`
	if err := os.WriteFile(filepath.Join(meta, metadata.PortItemType, item, ".forge.json"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(data, metadata.PortItemType, item, "install")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// vsync is an int here, and hdr is not in the file at all.
	if err := os.WriteFile(filepath.Join(dir, "settings.cfg"),
		[]byte("[video]\nmode=0\nvsync=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{metadataPath: meta, dataPath: data}
	c, err := app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Files) != 1 || !c.Files[0].Exists || c.Files[0].Empty {
		t.Fatalf("files = %+v", c.Files)
	}
	if _, ok := findField(c, "video.mode"); !ok {
		t.Error("the editable setting should be on the page")
	}
	for _, p := range []string{"video.vsync", "video.hdr"} {
		if _, ok := findField(c, p); ok {
			t.Errorf("%s cannot be edited, so it should not be on the page", p)
		}
	}
	// The two sections that lost their only setting are gone with it.
	var titles []string
	for _, sec := range c.Files[0].Sections {
		titles = append(titles, sec.Title)
	}
	if strings.Join(titles, ",") != "Video" {
		t.Errorf("sections = %v, want only Video", titles)
	}
}

// A file that exists and holds nothing this release can edit is marked, so the page
// can say that once rather than drawing an empty panel.
func TestGetGameConfigMarksAFileWithNothingEditable(t *testing.T) {
	meta, data := t.TempDir(), t.TempDir()
	item := "Test Port · 2026"
	metaDir := filepath.Join(meta, metadata.PortItemType, item, metadata.ConfigSchemaDir)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const sch = `{
	  "title": "Settings",
	  "path": "install/settings.cfg",
	  "format": "godot",
	  "sections": [{ "title": "Audio", "fields": [
	    { "pointer": "audio.master", "kind": "string", "widget": "text", "label": "Master" }
	  ] }]
	}`
	if err := os.WriteFile(filepath.Join(metaDir, "settings.schema.json"), []byte(sch), 0o644); err != nil {
		t.Fatal(err)
	}
	const spec = `{
	  "userDataPaths": [{ "locationType": "runDir", "path": "install/settings.cfg" }],
	  "builds": [{ "versions": ["1.0"], "targetPlatforms": ["Linux"], "steps": [] }]
	}`
	if err := os.WriteFile(filepath.Join(meta, metadata.PortItemType, item, ".forge.json"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(data, metadata.PortItemType, item, "install")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.cfg"), []byte("[audio]\nmaster=10\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{metadataPath: meta, dataPath: data}
	c, err := app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	// The file is there, so this is not the never-launched state — it is a schema
	// that does not fit the release, which is a different thing to say.
	if c.State != configNormal {
		t.Errorf("state = %q, want normal", c.State)
	}
	if len(c.Files) != 1 || !c.Files[0].Exists || !c.Files[0].Empty {
		t.Fatalf("files = %+v", c.Files)
	}
	if len(c.Files[0].Sections) != 0 {
		t.Errorf("sections = %+v, want none", c.Files[0].Sections)
	}
}

// The exception to the rule: a file the game has not written yet. That is
// temporary, one launch fixes it, and the page's banner explains it once — so the
// settings are listed with no value rather than hidden.
func TestGetGameConfigStillListsSettingsBeforeTheFirstLaunch(t *testing.T) {
	app := newConfigApp(t, false)
	c, err := app.GetGameConfig("Gen1Recomp · 2026")
	if err != nil {
		t.Fatal(err)
	}
	if c.State != configNeverLaunched {
		t.Fatalf("state = %q, want neverLaunched", c.State)
	}
	// Every field of the schema is there, including the one with no default, so a
	// person can see what they would get.
	for _, p := range []string{"musicVol", "animations", "battleStyle", "gone"} {
		fl, ok := findField(c, p)
		if !ok {
			t.Errorf("%s should be listed before the first launch", p)
			continue
		}
		if fl.Editable {
			t.Errorf("%s should not be editable yet", p)
		}
	}
}

// newReferenceApp lays out a port whose schema names a setting in a section the
// game does not always write, plus the catalog's reference copy of that file.
func newReferenceApp(t *testing.T, existing string, withReference bool) (*App, string) {
	t.Helper()
	// A schema names the copy it ships; without that declaration a file sitting in
	// the folder is not a reference, which is the point of declaring it.
	reference := ""
	if withReference {
		reference = `"reference": { "file": "settings.cfg.example" },`
	}
	meta, data := t.TempDir(), t.TempDir()
	item := "Ref Port · 2026"
	metaDir := filepath.Join(meta, metadata.PortItemType, item, metadata.ConfigSchemaDir)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sch := `{
	  "title": "Settings",
	  "path": "install/settings.cfg",
	  "format": "godot",
	  ` + reference + `
	  "sections": [
	    { "title": "Video", "fields": [
	      { "pointer": "video.mode", "kind": "int", "widget": "number", "label": "Mode", "default": 0 }
	    ] },
	    { "title": "Editor", "fields": [
	      { "pointer": "editor.autosave", "kind": "int", "widget": "number", "label": "Autosave", "default": 5 }
	    ] }
	  ]
	}`
	if err := os.WriteFile(filepath.Join(metaDir, "settings.schema.json"), []byte(sch), 0o644); err != nil {
		t.Fatal(err)
	}
	if withReference {
		// A complete copy of the file, as the game writes it.
		const ref = "[video]\nmode=0\nvsync=1\n\n[editor]\nautosave=5\n"
		if err := os.WriteFile(filepath.Join(metaDir, "settings.cfg.example"), []byte(ref), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const spec = `{
	  "userDataPaths": [{ "locationType": "runDir", "path": "install/settings.cfg" }],
	  "builds": [{ "versions": ["1.0"], "targetPlatforms": ["Linux"], "steps": [] }]
	}`
	if err := os.WriteFile(filepath.Join(meta, metadata.PortItemType, item, ".forge.json"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	if existing != "" {
		dir := filepath.Join(data, metadata.PortItemType, item, "install")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "settings.cfg"), []byte(existing), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &App{metadataPath: meta, dataPath: data}, item
}

// A file the game has not written, with a reference copy behind it: every setting
// is shown at the value the game would use and every one is editable, so this is no
// longer the never-launched state.
func TestGetGameConfigFallsBackToTheReference(t *testing.T) {
	app, item := newReferenceApp(t, "", true)
	c, err := app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	if c.State != configNormal {
		t.Errorf("state = %q, want normal: the settings are editable", c.State)
	}
	if len(c.Files) != 1 || c.Files[0].Exists || !c.Files[0].FromReference {
		t.Fatalf("files = %+v", c.Files)
	}
	for ptr, want := range map[string]string{"video.mode": "0", "editor.autosave": "5"} {
		fl, ok := findField(c, ptr)
		if !ok {
			t.Errorf("%s: not on the page", ptr)
			continue
		}
		if !fl.Editable {
			t.Errorf("%s should be editable from the reference", ptr)
		}
		// Nothing is recorded yet, whatever the reference holds.
		if !fl.Unset {
			t.Errorf("%s should be marked as not yet recorded", ptr)
		}
		if fl.Display != want {
			t.Errorf("%s = %q, want %q", ptr, fl.Display, want)
		}
	}
}

// Without a reference the old answer stands: nothing to offer until the game runs.
func TestGetGameConfigWithoutAReferenceIsUnchanged(t *testing.T) {
	app, item := newReferenceApp(t, "", false)
	c, err := app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	if c.State != configNeverLaunched {
		t.Errorf("state = %q, want neverLaunched", c.State)
	}
	if c.Files[0].FromReference {
		t.Error("there is no reference copy to have come from")
	}
}

// Saving a file the game has not written creates it from the reference, carrying the
// person's edit — and the reference's own settings come with it, including the one
// no field names.
func TestSaveGameConfigSeedsFromTheReference(t *testing.T) {
	app, item := newReferenceApp(t, "", true)
	if err := app.SaveGameConfig(item, []ConfigChange{
		{File: "install/settings.cfg", Pointer: "editor.autosave", Value: json.RawMessage(`9`)},
	}); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(app.dataPath, metadata.PortItemType, item, "install", "settings.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	const want = "[video]\nmode=0\nvsync=1\n\n[editor]\nautosave=9\n"
	if string(out) != want {
		t.Errorf("got  %q\nwant %q", out, want)
	}
}

// The case this was built for: the game has written the file but not the section a
// setting lives in. The reference attests the section, so the setting is editable
// and saving adds the section rather than guessing at it.
func TestSaveGameConfigCompletesAMissingSection(t *testing.T) {
	app, item := newReferenceApp(t, "[video]\nmode=2\n", true)

	c, err := app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	fl, ok := findField(c, "editor.autosave")
	if !ok {
		t.Fatal("editor.autosave should be offered: the reference has the section")
	}
	if !fl.Editable || !fl.Unset {
		t.Errorf("editor.autosave = %+v", fl)
	}

	if err := app.SaveGameConfig(item, []ConfigChange{
		{File: "install/settings.cfg", Pointer: "editor.autosave", Value: json.RawMessage(`3`)},
	}); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(app.dataPath, metadata.PortItemType, item, "install", "settings.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	// The game's own value for mode is kept, and the section is appended.
	const want = "[video]\nmode=2\n\n[editor]\nautosave=3\n"
	if string(out) != want {
		t.Errorf("got  %q\nwant %q", out, want)
	}
}

// Without a reference, a setting whose section is missing stays off the page and the
// file is left alone — the refusal this whole feature is an exception to.
func TestSaveGameConfigLeavesAMissingSectionAloneWithoutAReference(t *testing.T) {
	app, item := newReferenceApp(t, "[video]\nmode=2\n", false)
	c, err := app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findField(c, "editor.autosave"); ok {
		t.Error("nothing attests the [editor] section, so the setting must not be offered")
	}
	if err := app.SaveGameConfig(item, []ConfigChange{
		{File: "install/settings.cfg", Pointer: "video.mode", Value: json.RawMessage(`1`)},
	}); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(app.dataPath, metadata.PortItemType, item, "install", "settings.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "[video]\nmode=1\n" {
		t.Errorf("got %q, want the section left alone", got)
	}
}

// Fill defaults creates the file too, so someone can complete a port's configs
// without editing anything first.
func TestFillConfigDefaultsSeedsFromTheReference(t *testing.T) {
	app, item := newReferenceApp(t, "", true)
	if err := app.FillConfigDefaults(item); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(app.dataPath, metadata.PortItemType, item, "install", "settings.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "[video]\nmode=0\nvsync=1\n\n[editor]\nautosave=5\n" {
		t.Errorf("got %q", got)
	}
	// And with no reference it still declines to author a file.
	app2, item2 := newReferenceApp(t, "", false)
	if err := app2.FillConfigDefaults(item2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(app2.dataPath, metadata.PortItemType, item2, "install", "settings.cfg")); !os.IsNotExist(err) {
		t.Error("PortForge should not author a config file with nothing to copy from")
	}
}

// A reference copy that is not even the grammar its schema declares is not a
// faithful copy of anything, so it is ignored and reported rather than half-used.
//
// The fixture is a Lua data file rather than a Godot one on purpose: Godot's
// ConfigFile is deliberately lenient — a line it does not recognise is one it leaves
// alone — so it would accept almost any text as an empty config. A grammar with a
// required preamble is what makes "this is not that format" an answer at all.
func TestConfigReferenceIgnoresAnUnparseableExample(t *testing.T) {
	meta, data := t.TempDir(), t.TempDir()
	item := "Lua Port · 2026"
	metaDir := filepath.Join(meta, metadata.PortItemType, item, metadata.ConfigSchemaDir)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const sch = `{
	  "title": "Options",
	  "path": "install/options.lua",
	  "format": "luaTable",
	  "reference": { "file": "options.lua.example" },
	  "sections": [{ "title": "Audio", "fields": [
	    { "pointer": "volume", "kind": "int", "widget": "number", "label": "Volume", "default": 7 }
	  ] }]
	}`
	if err := os.WriteFile(filepath.Join(metaDir, "options.schema.json"), []byte(sch), 0o644); err != nil {
		t.Fatal(err)
	}
	const spec = `{
	  "userDataPaths": [{ "locationType": "runDir", "path": "install/options.lua" }],
	  "builds": [{ "versions": ["1.0"], "targetPlatforms": ["Linux"], "steps": [] }]
	}`
	if err := os.WriteFile(filepath.Join(meta, metadata.PortItemType, item, ".forge.json"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	// No `return`, so this is not a Lua data file whatever it is.
	if err := os.WriteFile(filepath.Join(metaDir, "options.lua.example"),
		[]byte("volume = 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{metadataPath: meta, dataPath: data}
	c, err := app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	if c.Files[0].FromReference {
		t.Error("an unparseable reference should not be used")
	}
	if c.State != configNeverLaunched {
		t.Errorf("state = %q, want neverLaunched: there is no usable reference", c.State)
	}

	// And a valid one is used, which is what proves the test is testing the parse.
	if err := os.WriteFile(filepath.Join(metaDir, "options.lua.example"),
		[]byte("return {\n  volume = 7,\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err = app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Files[0].FromReference {
		t.Error("a valid reference should be used")
	}
}

// A copy sitting in the folder that no schema names is not a reference. The
// declaration is what makes it one, which is the whole point of declaring it.
func TestAnUndeclaredCopyIsNotAReference(t *testing.T) {
	app, item := newReferenceApp(t, "", false)
	metaDir := filepath.Join(app.metadataPath, metadata.PortItemType, item, metadata.ConfigSchemaDir)
	if err := os.WriteFile(filepath.Join(metaDir, "settings.cfg.example"),
		[]byte("[video]\nmode=0\n\n[editor]\nautosave=5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	if c.Files[0].FromReference {
		t.Error("the schema does not name this copy, so it is not a reference")
	}
	if c.State != configNeverLaunched {
		t.Errorf("state = %q, want neverLaunched", c.State)
	}
}

// A schema that names a copy the catalog does not ship is a mistake worth reporting,
// not something to treat as "no reference": somebody meant it to be there.
func TestANamedCopyThatIsMissingIsReported(t *testing.T) {
	app, item := newReferenceApp(t, "", true)
	metaDir := filepath.Join(app.metadataPath, metadata.PortItemType, item, metadata.ConfigSchemaDir)
	if err := os.Remove(filepath.Join(metaDir, "settings.cfg.example")); err != nil {
		t.Fatal(err)
	}
	if _, err := metadata.ConfigReference(app.metadataPath, item, "1.0", []string{"1.0"},
		schema.File{Path: "install/settings.cfg", Reference: schema.References{{File: "settings.cfg.example"}}}); err == nil {
		t.Error("a named copy that is not there should be an error")
	}
	// The page still works; it just has no reference to lean on.
	c, err := app.GetGameConfig(item)
	if err != nil {
		t.Fatal(err)
	}
	if c.Files[0].FromReference {
		t.Error("there is no copy to have come from")
	}
}

// A program whose config changes shape between releases names one copy per shape, and
// the installed release decides which is used. Seeding 1.1's shape into 1.0.2 would
// write sections 1.0.2 has never heard of, and its loader indexes file[section][key].
func TestReferencePerReleasePicksTheRightShape(t *testing.T) {
	meta, data := t.TempDir(), t.TempDir()
	item := "Two Shapes · 2026"
	metaDir := filepath.Join(meta, metadata.PortItemType, item, metadata.ConfigSchemaDir)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const sch = `{
	  "title": "Settings",
	  "path": "install/settings.cfg",
	  "format": "godot",
	  "reference": [
	    { "file": "old.example", "untilVersion": "1.0" },
	    { "file": "new.example", "sinceVersion": "2.0" }
	  ],
	  "sections": [
	    { "title": "Video", "fields": [
	      { "pointer": "video.mode", "kind": "int", "widget": "number", "label": "Mode", "default": 0 }
	    ] },
	    { "title": "Editor", "fields": [
	      { "pointer": "editor.autosave", "kind": "int", "widget": "number", "label": "Autosave",
	        "default": 5, "sinceVersion": "2.0" }
	    ] }
	  ]
	}`
	if err := os.WriteFile(filepath.Join(metaDir, "settings.schema.json"), []byte(sch), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "old.example"), []byte("[video]\nmode=0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "new.example"),
		[]byte("[video]\nmode=0\n\n[editor]\nautosave=5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const spec = `{
	  "userDataPaths": [{ "locationType": "runDir", "path": "install/settings.cfg" }],
	  "builds": [{ "versions": ["1.0", "2.0"], "targetPlatforms": ["Linux"], "steps": [] }]
	}`
	if err := os.WriteFile(filepath.Join(meta, metadata.PortItemType, item, ".forge.json"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	for version, want := range map[string]string{
		"1.0": "[video]\nmode=0\n",
		"2.0": "[video]\nmode=0\n\n[editor]\nautosave=5\n",
	} {
		dir := filepath.Join(data, metadata.PortItemType, item)
		if err := os.MkdirAll(filepath.Join(dir, ".state"), 0o755); err != nil {
			t.Fatal(err)
		}
		state := `{"installed":true,"installedVersion":"` + version + `","targetPlatform":"Linux"}`
		if err := os.WriteFile(filepath.Join(dir, ".state", "meta.json"), []byte(state), 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(filepath.Join(dir, "install", "settings.cfg"))

		app := &App{metadataPath: meta, dataPath: data}
		if err := app.FillConfigDefaults(item); err != nil {
			t.Fatalf("%s: %v", version, err)
		}
		out, err := os.ReadFile(filepath.Join(dir, "install", "settings.cfg"))
		if err != nil {
			t.Fatalf("%s: %v", version, err)
		}
		if string(out) != want {
			t.Errorf("%s: got %q, want %q", version, out, want)
		}
	}
}
