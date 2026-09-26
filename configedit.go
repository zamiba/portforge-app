package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"portforge/metadata"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/zamiba/config-forge/configfile"
	"github.com/zamiba/config-forge/schema"
)

// Config page states, in the priority the design gives them.
const (
	configNormal        = "normal"
	configRunning       = "running"
	configNeverLaunched = "neverLaunched"
)

// playingTitle is the running game's item title, or "" when none is.
//
// Config editing asks about one game rather than about any game, unlike a profile
// switch: editing one port's settings while a different port runs is harmless, and
// the page's copy names the game whose settings they are.
func (a *App) playingTitle() string {
	a.playMu.Lock()
	defer a.playMu.Unlock()
	return a.playing
}

// ConfigOption is one choice of a select or radio field.
type ConfigOption struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}

// ConfigField is one setting as the page needs it: what it is, what it holds now,
// and whether it can be edited.
type ConfigField struct {
	Pointer  string `json:"pointer"`
	Label    string `json:"label"`
	Help     string `json:"help,omitempty"`
	Widget   string `json:"widget,omitempty"`
	Kind     string `json:"kind"`
	Value    any    `json:"value,omitempty"`
	Display  string `json:"display"`
	Editable bool   `json:"editable"`
	ReadOnly bool   `json:"readOnly,omitempty"`

	// Unset is a setting the game has not written yet, shown with the game's own
	// default and editable anyway: saving adds the key. Worth marking, because the
	// value on screen is what the game would use rather than what it recorded.
	Unset bool `json:"unset,omitempty"`

	// Reason is a code, not a sentence: the words belong to the page. Empty for a
	// field that is editable or read-only by design.
	Reason string `json:"reason,omitempty"`

	Options []ConfigOption `json:"options,omitempty"`
	Min     *float64       `json:"min,omitempty"`
	Max     *float64       `json:"max,omitempty"`
	Step    *float64       `json:"step,omitempty"`
	Unit    string         `json:"unit,omitempty"`
	On      any            `json:"on,omitempty"`
	Off     any            `json:"off,omitempty"`

	// Default is the value the game itself uses for this setting. Sent so the page
	// can show what a setting would revert to and offer to put it back.
	Default any `json:"default,omitempty"`
}

// ConfigSection groups fields under a heading, following the file's own grouping.
type ConfigSection struct {
	Title  string        `json:"title"`
	Help   string        `json:"help,omitempty"`
	Fields []ConfigField `json:"fields"`
}

// ConfigFile is one of the game's config files and the settings inside it.
type ConfigFile struct {
	Title string `json:"title"`
	Path  string `json:"path"`
	// Exists is false when the game has not written the file yet, in which case
	// every field is unavailable and the page says so once rather than per field.
	Exists bool `json:"exists"`

	// FromReference is a file the game has not written, whose settings are shown
	// from the catalog's reference copy of it. Every one of them is editable, and
	// saving creates the file; nothing in it is recorded yet.
	FromReference bool `json:"fromReference,omitempty"`

	// Empty is a file that exists and has nothing this release can edit — every
	// setting the schema names is absent in a way PortForge cannot put right, or
	// stored differently than the schema expects. The page says that once about
	// the file rather than listing settings nobody can touch.
	Empty    bool            `json:"empty,omitempty"`
	Sections []ConfigSection `json:"sections"`
}

// GameConfig is everything the Config tab needs for one port.
type GameConfig struct {
	Profile string `json:"profile"`
	Slug    string `json:"slug"`
	// State is the page state: normal, running or neverLaunched. A failed write
	// and a file changed on disk are outcomes the page learns from a save or a
	// re-read, not states this reports.
	State   string       `json:"state"`
	Running string       `json:"running,omitempty"`
	Files   []ConfigFile `json:"files"`
}

// ConfigChange is one edit the page asks to be written.
type ConfigChange struct {
	File    string          `json:"file"`    // the config file's declared path
	Pointer string          `json:"pointer"` // the field within it
	Value   json.RawMessage `json:"value"`
}

// GetGameConfig reads a port's config schemas and resolves them against the files
// on disk, for the version installed and the profile in use.
//
// A port with no schemas returns no files, which is how the page knows to show no
// tab at all. Reading never writes: a game that has not been launched keeps its
// missing files, and PortForge does not create one to have something to show.
func (a *App) GetGameConfig(itemTitle string) (GameConfig, error) {
	out := GameConfig{State: configNormal}

	files, err := metadata.LoadConfigSchemas(a.metadataPath, itemTitle)
	if err != nil {
		return out, err
	}
	if len(files) == 0 {
		return out, nil
	}

	p, err := a.activeProfile()
	if err == nil {
		out.Profile, out.Slug = p.Name, p.Slug
	}

	if a.playingTitle() == itemTitle {
		out.State, out.Running = configRunning, itemTitle
	}

	version, versions := a.installedVersion(itemTitle), a.declaredVersions(itemTitle)

	missing := 0
	for _, f := range files {
		cf := ConfigFile{Title: f.Title, Path: f.Path}
		ref := a.configReference(itemTitle, version, versions, f)
		doc, err := configfile.Open(a.configFilePath(itemTitle, f.Path), f.Format)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Printf("config: %s: %s: %v", itemTitle, f.Path, err)
			}
			if ref != nil {
				// The game has not written this file, but the catalog ships a
				// reference copy of it, so every setting can be shown at the value
				// the game itself would use and edited from there. Saving writes the
				// file. Nothing is recorded yet, which is what Unset says.
				cf.FromReference = true
				cf.Sections = configSections(itemTitle, ref, ref, f, version, versions)
				markUnrecorded(cf.Sections)
				cf.Empty = len(cf.Sections) == 0
				out.Files = append(out.Files, cf)
				continue
			}
			// No reference either, so there is nothing to show but the shape of
			// what one launch would produce.
			missing++
			cf.Sections = configSections(itemTitle, nil, nil, f, version, versions)
			out.Files = append(out.Files, cf)
			continue
		}
		cf.Exists = true
		cf.Sections = configSections(itemTitle, doc, ref, f, version, versions)
		cf.Empty = len(cf.Sections) == 0
		out.Files = append(out.Files, cf)
	}

	// Every file missing, with no reference copy to stand in for any of them, means
	// the game has never run and there is nothing to offer until it has.
	if missing == len(out.Files) && out.State == configNormal {
		out.State = configNeverLaunched
	}
	return out, nil
}

// configReference opens the reference copy of a config file, if the catalog ships
// one. It is what makes a section the program has not written addable, and what a
// config file the program has never created is seeded from.
//
// A reference that does not parse as the format its schema declares is not usable
// and is reported rather than half-applied: the whole point of it is to be a
// faithful copy of what the program writes, and one that is not even the right
// grammar is not that.
// configReference opens the copy of a config file the schema names for this release,
// if it names one.
//
// There is no longer a second tier here. The old rule guessed from a filename whether
// a copy was safe to write out whole; the schema now states which releases each copy
// is right for, so a copy that applies to the installed release is one the catalog
// has vouched for and may be both completed from and written out whole. A copy that
// does not parse as the grammar its schema declares is not a faithful copy of
// anything, and is reported rather than half-used.
func (a *App) configReference(itemTitle, version string, versions []string, f schema.File) configfile.Doc {
	data, err := metadata.ConfigReference(a.metadataPath, itemTitle, version, versions, f)
	if err != nil {
		log.Printf("config: %s: %v", itemTitle, err)
		return nil
	}
	if data == nil {
		return nil
	}
	ref, err := configfile.Parse(data, f.Format)
	if err != nil {
		log.Printf("config: %s: %s: the reference copy is not valid %s: %v",
			itemTitle, f.Path, f.Format, err)
		return nil
	}
	return ref
}

// configFilePath locates a config file the schema names. The path is spelled as
// the program's own declaration spells it, and the same string reaches the file
// two different ways.
//
// Every config file a port keeps ends up in the active profile: saveLinks puts
// the real bytes at the profile's item folder and leaves a link behind wherever
// the port expects them, and a port that takes a save-location flag is handed
// that same folder as ${profilePath} and writes straight into it. So the
// profile's copy is the one place every one of them lives, whichever mechanism
// put it there — which is why a schema needs no location type of its own.
//
// The port's own folder is still tried first, and still matters: it is where the
// file is when the port's data is not linked at all, either because save linking
// hit a conflict or because the path was never declared as user data. Resolving
// only against the profile would report such a file as one the game has not
// written yet, while it sat in the port's folder the whole time.
func (a *App) configFilePath(itemTitle, rel string) string {
	inPort := filepath.Join(a.dataPath, metadata.PortItemType, itemTitle, filepath.FromSlash(rel))
	if _, err := os.Lstat(inPort); err == nil {
		return inPort
	}
	p, err := a.activeProfile()
	if err != nil {
		return inPort
	}
	itemDir, err := p.ItemDir(metadata.PortItemType, itemTitle)
	if err != nil {
		return inPort
	}
	inProfile := filepath.Join(itemDir, filepath.FromSlash(rel))
	if _, err := os.Lstat(inProfile); err == nil {
		return inProfile
	}
	// Neither is there. The port's own folder is the better thing to name in a
	// log or an error, since that is where the program would create it.
	return inPort
}

// declaredVersions returns the port's releases oldest first, which is the order
// a field's version bounds are compared against.
func (a *App) declaredVersions(itemTitle string) []string {
	file, err := metadata.LoadSpecFile(a.metadataPath, itemTitle)
	if err != nil || file == nil {
		return nil
	}
	// SpecFile.Versions keeps the spec's own declaration order, which is oldest
	// first — that is the order version bounds compare against. GetSpecVersions
	// reverses it for the picker, which is a different job; reversing here too
	// inverted every bound and quietly applied them to the wrong releases.
	declared := file.Versions()
	out := make([]string, 0, len(declared))
	for _, v := range declared {
		out = append(out, v.Version)
	}
	return out
}

// configSections resolves one schema against its file and keeps the settings the
// page can actually offer.
//
// A setting this release cannot edit is left out rather than shown greyed: a row
// nobody can act on is noise, and the reasons are not a person's problem to solve
// — a schema naming a setting the release stores differently, or whose section the
// game has not written, is not something anyone can fix from the page. What is
// dropped is logged instead, because a schema drifting away from its program is
// worth knowing about and a silent page would be the only sign.
//
// The exception is a file the game has not written at all. That is temporary and
// one launch fixes it, so every setting is listed with no value and the page's own
// banner explains it once. nil doc is that case.
func configSections(itemTitle string, doc, ref configfile.Doc, f schema.File, version string, versions []string) []ConfigSection {
	neverWritten := doc == nil
	if neverWritten {
		doc = configfile.Empty()
	}
	var out []ConfigSection
	var dropped []string
	for _, sec := range schema.ResolveWith(doc, ref, f, version, versions) {
		cs := ConfigSection{Title: sec.Title, Help: sec.Help}
		for _, st := range sec.Fields {
			if !neverWritten && !st.Editable && !st.ReadOnly {
				dropped = append(dropped, fmt.Sprintf("%s (%s)", st.Pointer, st.Reason))
				continue
			}
			cs.Fields = append(cs.Fields, configField(st))
		}
		// A section whose every setting was dropped is not a heading worth drawing.
		if len(cs.Fields) > 0 {
			out = append(out, cs)
		}
	}
	if len(dropped) > 0 {
		log.Printf("config: %s: %s: not offered: %s", itemTitle, f.Path, strings.Join(dropped, ", "))
	}
	return out
}

// markUnrecorded marks every field as holding a value the game has not recorded.
// It is for a file resolved against the catalog's reference copy: each value is one
// the game would use, and none of it is in a file yet, which is exactly what Unset
// means everywhere else on the page.
func markUnrecorded(sections []ConfigSection) {
	for i := range sections {
		for j := range sections[i].Fields {
			if sections[i].Fields[j].Editable {
				sections[i].Fields[j].Unset = true
			}
		}
	}
}

func configField(st schema.FieldState) ConfigField {
	out := ConfigField{
		Pointer:  st.Pointer,
		Label:    st.Label,
		Help:     st.Help,
		Widget:   string(st.Widget),
		Kind:     string(st.Kind),
		Display:  st.Value.Display(),
		Editable: st.Editable,
		ReadOnly: st.ReadOnly,
		Unset:    st.Unset,
		Reason:   string(st.Reason),
		Min:      st.Min,
		Max:      st.Max,
		Step:     st.Step,
		Unit:     st.Unit,
	}
	if st.Present || st.Unset {
		out.Value = configValue(st.Value)
	}
	for _, o := range st.Options {
		out.Options = append(out.Options, ConfigOption{Label: o.Label, Value: configValue(o.V())})
	}
	if st.On != nil {
		out.On = configValue(st.On.V)
	}
	if st.Off != nil {
		out.Off = configValue(st.Off.V)
	}
	if st.Default != nil {
		out.Default = configValue(st.Default.V)
	}
	return out
}

// configValue renders a value for JSON as the type it actually is, so a number
// does not arrive in the page as a string and a bool does not arrive as 1.
func configValue(v configfile.Value) any {
	switch v.Kind {
	case configfile.KindBool:
		return v.Bool
	case configfile.KindInt:
		return v.Int
	case configfile.KindFloat:
		return v.Float
	case configfile.KindString:
		return v.Str
	}
	// Opaque: the literal, which is all a read-only row shows.
	return v.Raw
}

// SaveGameConfig writes a batch of edits to a port's config files.
//
// Nothing is written while the game is running: several of these programs rewrite
// their config every few seconds and on exit, so an edit would be lost or would
// land on top of a file the game is mid-write on. Each file is read fresh rather
// than from whatever the page last saw, so a value the game changed in the
// meantime is preserved unless this batch names it.
//
// A file is written whole only in the sense that its bytes are written back: every
// byte no change touched is the byte that was read. A file that fails to write
// leaves the others' writes in place and reports which one failed, because the
// alternative — unwinding successful writes — means writing again to undo, which
// can fail in the same way.
func (a *App) SaveGameConfig(itemTitle string, changes []ConfigChange) error {
	if len(changes) == 0 {
		return nil
	}

	if a.playingTitle() == itemTitle {
		return fmt.Errorf("%s is running; its settings cannot be edited until it quits", itemTitle)
	}

	files, err := metadata.LoadConfigSchemas(a.metadataPath, itemTitle)
	if err != nil {
		return err
	}
	byPath := make(map[string]schema.File, len(files))
	for _, f := range files {
		byPath[f.Path] = f
	}

	// Group by file so each is opened, written and closed once.
	grouped := map[string][]ConfigChange{}
	for _, c := range changes {
		if _, ok := byPath[c.File]; !ok {
			return fmt.Errorf("%s: no config schema describes %s", itemTitle, c.File)
		}
		grouped[c.File] = append(grouped[c.File], c)
	}
	paths := make([]string, 0, len(grouped))
	for p := range grouped {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	version, versions := a.installedVersion(itemTitle), a.declaredVersions(itemTitle)

	for _, rel := range paths {
		f := byPath[rel]
		full := a.configFilePath(itemTitle, rel)
		ref := a.configReference(itemTitle, version, versions, f)
		doc, _, err := a.openOrSeed(full, f, ref)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}

		// Every setting the file does not hold yet, at the value the game itself
		// uses, before any edit is applied. It has to be this way round: an edit to a
		// setting whose section the game has not written cannot be written until the
		// section is there, and this is what puts it there. A setting the person
		// edited is then overwritten by their value below.
		if _, err := schema.Complete(doc, refOrEmpty(ref), f, version, versions); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}

		fields := fieldsByPointer(f)
		for _, c := range grouped[rel] {
			fl, ok := fields[c.Pointer]
			if !ok {
				return fmt.Errorf("%s: %s is not a setting this schema describes", rel, c.Pointer)
			}
			v, err := decodeConfigValue(fl, c.Value)
			if err != nil {
				return fmt.Errorf("%s: %s: %w", rel, c.Pointer, err)
			}
			if err := schema.WriteField(doc, fl, v); err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
		}

		if err := writeConfigFile(full, doc.Bytes()); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
	}
	return nil
}

// FillConfigDefaults writes every setting a port's config files do not hold yet,
// at the value the game itself uses, leaving the files complete without changing
// how the game behaves. Settings already written are untouched.
//
// It is the same filling a save does, for someone who wants the files completed
// without editing anything first.
func (a *App) FillConfigDefaults(itemTitle string) error {
	if a.playingTitle() == itemTitle {
		return fmt.Errorf("%s is running; its settings cannot be edited until it quits", itemTitle)
	}
	files, err := metadata.LoadConfigSchemas(a.metadataPath, itemTitle)
	if err != nil {
		return err
	}
	version, versions := a.installedVersion(itemTitle), a.declaredVersions(itemTitle)

	for _, f := range files {
		full := a.configFilePath(itemTitle, f.Path)
		ref := a.configReference(itemTitle, version, versions, f)
		doc, seeded, err := a.openOrSeed(full, f, ref)
		if err != nil {
			// A file the game has not created and the catalog cannot stand in for
			// is one PortForge will not author: it writes settings inside a config,
			// never a config out of nothing.
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("%s: %w", f.Path, err)
		}
		n, err := schema.Complete(doc, refOrEmpty(ref), f, version, versions)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
		// A seeded file is complete already and still not on disk, so it is written
		// even though Complete found nothing left to add.
		if n == 0 && !seeded {
			continue
		}
		if err := writeConfigFile(full, doc.Bytes()); err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
	}
	return nil
}

// openOrSeed opens a config file, or starts one from the catalog's reference copy
// when the program has not written it yet.
//
// Seeding is the one case where PortForge produces a whole config file, and what
// makes it defensible is that it is not composing one: the bytes are the reference
// copy's, authored from the program's own source as a faithful copy of what the
// program writes. Without a reference the original refusal stands and the caller
// gets the not-exist error to handle.
func (a *App) openOrSeed(full string, f schema.File, ref configfile.Doc) (configfile.Doc, bool, error) {
	doc, err := configfile.Open(full, f.Format)
	if err == nil {
		return doc, false, nil
	}
	if !os.IsNotExist(err) || ref == nil {
		return nil, false, err
	}
	doc, err = configfile.Parse(ref.Bytes(), f.Format)
	return doc, err == nil, err
}

// refOrEmpty is the reference copy, or the document for a file that is not there.
// Complete falls back to each field's own default where the reference says nothing,
// so with no reference it behaves as it did before there were any: it fills the
// settings whose section the program has already written, and leaves the rest.
func refOrEmpty(ref configfile.Doc) configfile.Doc {
	if ref == nil {
		return configfile.Empty()
	}
	return ref
}

// installedVersion is the version recorded for the installed copy, or "" when the
// port is not installed.
func (a *App) installedVersion(itemTitle string) string {
	state, _ := metadata.ReadInstallState(filepath.Join(a.dataPath, metadata.PortItemType, itemTitle))
	if state == nil {
		return ""
	}
	return state.InstalledVersion
}

func fieldsByPointer(f schema.File) map[string]schema.Field {
	out := map[string]schema.Field{}
	for _, sec := range f.Sections {
		for _, fl := range sec.Fields {
			out[fl.Pointer] = fl
		}
	}
	return out
}

// decodeConfigValue turns the page's JSON into a value of the kind the field
// declares. Decoding by the declared kind rather than by what the JSON looks like
// is what keeps a whole number meant for an int field from arriving as a float.
func decodeConfigValue(fl schema.Field, raw json.RawMessage) (configfile.Value, error) {
	switch fl.Kind {
	case schema.KindBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return configfile.Value{}, fmt.Errorf("expected true or false: %w", err)
		}
		return configfile.Bool(b), nil
	case schema.KindInt:
		var i int64
		if err := json.Unmarshal(raw, &i); err != nil {
			return configfile.Value{}, fmt.Errorf("expected a whole number: %w", err)
		}
		return configfile.Int(i), nil
	case schema.KindFloat:
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return configfile.Value{}, fmt.Errorf("expected a number: %w", err)
		}
		return configfile.Float(f), nil
	case schema.KindString:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return configfile.Value{}, fmt.Errorf("expected text: %w", err)
		}
		return configfile.String(s), nil
	}
	return configfile.Value{}, fmt.Errorf("a %s field cannot be written", fl.Kind)
}

// writeConfigFile replaces a config file's contents, atomically where it can.
//
// The path is very likely a symlink into the active profile, which is why the
// link is resolved first: writing a temporary file beside the link and renaming
// over it would replace the link with a regular file, quietly detaching the
// game's settings from the profile they belong to. Resolving means the temporary
// file is created beside the real file and the rename lands on the real file, so
// the link survives and the replacement is still atomic.
func writeConfigFile(path string, data []byte) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}

	mode := os.FileMode(0o644)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
	} else if os.IsNotExist(err) {
		// Starting a file the program has never written: its folder may not be there
		// either, which is the ordinary case for a profile's copy of a config.
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), ".portforge-config-*")
	if err != nil {
		// A read-only directory, a full disk: fall back to writing in place. Less
		// safe, but refusing to save at all is worse when the alternative works.
		return os.WriteFile(target, data, mode)
	}
	name := tmp.Name()
	defer os.Remove(name)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	return os.Rename(name, target)
}

// ChooseConfigPath opens a file or folder picker for a path field and returns
// what was chosen, or "" if the person cancelled.
//
// The schema says a setting is a location; it does not say whether the program
// wants a file or a folder, so both are offered as separate calls rather than
// guessed at. The chosen path is returned to the page and written like any other
// string: nothing here checks that it exists, because the program the setting
// belongs to is the only thing that knows what a valid location is for it.
func (a *App) ChooseConfigPath(folder bool, current string) (string, error) {
	opts := wailsruntime.OpenDialogOptions{Title: "Choose a location"}
	if current != "" {
		opts.DefaultDirectory = filepath.Dir(current)
	}
	if folder {
		return wailsruntime.OpenDirectoryDialog(a.ctx, opts)
	}
	return wailsruntime.OpenFileDialog(a.ctx, opts)
}
