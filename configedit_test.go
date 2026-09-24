package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"portforge/metadata"
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
	if err := os.WriteFile(filepath.Join(metaDir, "options.json"), []byte(sch), 0o644); err != nil {
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

func fieldOf(t *testing.T, c GameConfig, pointer string) ConfigField {
	t.Helper()
	for _, f := range c.Files {
		for _, sec := range f.Sections {
			for _, fl := range sec.Fields {
				if fl.Pointer == pointer {
					return fl
				}
			}
		}
	}
	t.Fatalf("no field %s", pointer)
	return ConfigField{}
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

	// A setting the file has lost is shown with a reason code, not a sentence.
	gone := fieldOf(t, c, "gone")
	if gone.Editable || gone.Reason != "missing" {
		t.Errorf("gone = %+v", gone)
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
	if err := os.WriteFile(filepath.Join(dir, metadata.ConfigSchemaDir, "a.json"), []byte(sch), 0o644); err != nil {
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
