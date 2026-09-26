package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"portforge/metadata"

	"github.com/zamiba/config-forge/configfile"
	"github.com/zamiba/config-forge/schema"
)

// Every config schema in the catalog is valid, and every file it describes ends
// up in the active profile.
//
// That second check is the one worth having, and "ends up in the profile" is the
// thing to check rather than "is spelled exactly like a userDataPaths entry". A
// config file that does not reach the profile is not protected: uninstalling
// deletes it, reinstalling overwrites it, and switching profiles does not carry
// it. Editing it from PortForge would write settings that silently vanish. The
// dependency runs one way — forge decides what is user data, a schema only
// annotates it.
//
// Three declarations get a file there, and the locationType is not among the
// things that matter: saveLinks puts the real bytes at the profile's item folder
// under the entry's own path whether the entry is inside the tree or outside it,
// and a port handed ${profilePath} writes into that same folder directly. So a
// schema path is covered when it names an entry, sits under one that is a
// folder, or the port launches with ${profilePath} — in which case the whole of
// the profile's item folder is where the port writes.
func TestCatalogConfigSchemas(t *testing.T) {
	app := newCatalogApp(t)
	entries, err := os.ReadDir(filepath.Join(app.metadataPath, metadata.PortItemType))
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		item := e.Name()
		files, err := metadata.LoadConfigSchemas(app.metadataPath, item)
		if err != nil {
			t.Errorf("%s: %v", item, err)
			continue
		}
		if len(files) == 0 {
			continue
		}
		t.Run(item, func(t *testing.T) {
			for _, err := range schema.ValidateAll(files) {
				t.Errorf("%v", err)
			}

			specs, err := metadata.LoadInstallationSpecs(app.metadataPath, item)
			if err != nil {
				t.Fatal(err)
			}
			// Every entry, not only the in-tree ones: Spec.UserDataPaths holds
			// what the engine acts on, while Spec.UserData is the whole
			// declaration, and the profile's copy of an entry outside the tree
			// sits under the same path as one inside it.
			var userData []string
			writesToProfile := false
			for _, s := range specs {
				for _, u := range s.UserData {
					userData = append(userData, filepath.ToSlash(u.Path))
				}
				for _, step := range s.Steps {
					if step.Step != "defineExecutable" {
						continue
					}
					for _, arg := range step.Args {
						if strings.Contains(arg, "${profilePath}") {
							writesToProfile = true
						}
					}
				}
			}
			// A version bound that names no declared release silently applies to
			// nothing, or to everything, depending which bound it is. Only the
			// spec knows the release names, so this check lives here.
			var declared []string
			for _, s := range specs {
				for _, v := range s.Versions {
					if !hasString(declared, v) {
						declared = append(declared, v)
					}
				}
			}
			for _, f := range files {
				for _, err := range schema.ValidateAgainstVersions(f, declared) {
					t.Errorf("%v", err)
				}
			}

			for _, f := range files {
				if writesToProfile || coveredByUserData(userData, f.Path) {
					continue
				}
				t.Errorf("%q describes %s, which no userDataPaths entry covers and which the "+
					"port does not write into ${profilePath}: an uninstall would delete it and "+
					"it is never carried into a profile", f.Title, f.Path)
			}
			checked++
		})
	}
	if checked == 0 {
		t.Error("no catalog item carries a config schema; this test is asserting nothing")
	}
}

// coveredByUserData reports whether a declared path is one of the entries or
// lives inside one of them. A prefix match is on whole path elements, so an entry
// of "install/profiles" covers "install/profiles/default/x.toml" but not a
// sibling folder called "install/profiles-old".
func coveredByUserData(entries []string, path string) bool {
	path = filepath.ToSlash(path)
	for _, e := range entries {
		if e == path || strings.HasPrefix(path, e+"/") {
			return true
		}
	}
	return false
}

func hasString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// The coverage rule accepts three shapes and still refuses everything else — the
// check is only worth having if it can fail.
func TestCoveredByUserData(t *testing.T) {
	entries := []string{"install/options.lua", "install/profiles", "melee-pc"}
	for path, want := range map[string]bool{
		// Named outright.
		"install/options.lua": true,
		"melee-pc":            true,
		// Inside a folder that is named, which is how reBlue keeps its config and
		// how a port whose data lives in an OS folder keeps its.
		"install/profiles/default/reblue.toml": true,
		"melee-pc/launcher.cfg":                true,
		// Not covered. The third is the one a plain string prefix would wrongly
		// accept: a sibling folder whose name merely starts the same way.
		"install/settings.json":   false,
		"sm64config.txt":          false,
		"install/profiles-old/x":  false,
		"install/options.lua.bak": false,
		"":                        false,
	} {
		if got := coveredByUserData(entries, path); got != want {
			t.Errorf("coveredByUserData(%q) = %v, want %v", path, got, want)
		}
	}
}

// Every reference copy in the catalog is a usable one: it parses as the grammar its
// schema declares, and it actually holds the settings the page will offer from it.
//
// That second part is the invariant worth having. A reference copy exists to attest
// that a section is real, so PortForge can add it rather than guess at it. One that
// is missing a setting the schema names attests nothing about it, and the setting
// goes back to being uneditable with no sign of why.
func TestCatalogConfigReferences(t *testing.T) {
	app := newCatalogApp(t)
	entries, err := os.ReadDir(filepath.Join(app.metadataPath, metadata.PortItemType))
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		item := e.Name()
		files, err := metadata.LoadConfigSchemas(app.metadataPath, item)
		if err != nil || len(files) == 0 {
			continue
		}
		t.Run(item, func(t *testing.T) {
			var declared []string
			specs, err := metadata.LoadInstallationSpecs(app.metadataPath, item)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range specs {
				for _, v := range s.Versions {
					if !hasString(declared, v) {
						declared = append(declared, v)
					}
				}
			}
			newest := ""
			if len(declared) > 0 {
				newest = declared[len(declared)-1]
			}

			// Every reference copy in the folder is named by a schema, and every copy
			// a schema names is there. A file nobody reads drifts, and a name nobody
			// ships is a setting that silently stops being editable.
			dir := filepath.Join(app.metadataPath, metadata.PortItemType, item, metadata.ConfigSchemaDir)
			named := map[string]bool{}
			for _, f := range files {
				for _, r := range f.Reference {
					named[r.File] = true
					if _, err := os.Stat(filepath.Join(dir, r.File)); err != nil {
						t.Errorf("%s names the reference copy %q, which is not there", f.Path, r.File)
					}
				}
			}
			found, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, fe := range found {
				if fe.IsDir() || strings.EqualFold(filepath.Ext(fe.Name()), ".json") {
					continue
				}
				if !named[fe.Name()] {
					t.Errorf("%s is in %s and no schema names it", fe.Name(), metadata.ConfigSchemaDir)
				}
			}

			// A port with more than one release needs its references to cover every
			// one of them, or a release quietly loses the settings a reference makes
			// editable. An unbounded reference covers all of them by declaration.
			if len(declared) > 1 {
				for _, f := range files {
					if len(f.Reference) == 0 {
						continue
					}
					for _, v := range declared {
						if _, ok := f.ReferenceFor(v, declared); !ok {
							t.Errorf("%s has reference copies but none for %q", f.Path, v)
						}
					}
				}
			}

			for _, f := range files {
				data, err := metadata.ConfigReference(app.metadataPath, item, newest, declared, f)
				if err != nil {
					t.Errorf("%s: %v", f.Path, err)
					continue
				}
				if data == nil {
					continue
				}
				checked++
				ref, err := configfile.Parse(data, f.Format)
				if err != nil {
					t.Errorf("%s: the reference copy is not valid %s: %v", f.Path, f.Format, err)
					continue
				}
				// Resolved against itself, every setting the schema declares a
				// default for has to come out editable — which is exactly what the
				// page will ask of it.
				for _, sec := range schema.ResolveWith(configfile.Empty(), ref, f, newest, declared) {
					for _, st := range sec.Fields {
						if st.ReadOnly || st.Default == nil {
							continue
						}
						if !st.Editable {
							t.Errorf("%s: %s: the reference copy does not make this editable (%s: %s)",
								f.Path, st.Pointer, st.Reason, st.Problem)
						}
					}
				}
				// And where the reference and the schema both speak, they agree.
				for _, sec := range f.Sections {
					for _, fl := range sec.Fields {
						if fl.Default == nil {
							continue
						}
						v, ok := ref.Get(configfile.ParsePath(fl.Pointer))
						if !ok {
							continue
						}
						if v.Display() != fl.Default.V.Display() {
							t.Errorf("%s: %s: the reference copy holds %q and the schema's default is %q",
								f.Path, fl.Pointer, v.Display(), fl.Default.V.Display())
						}
					}
				}
			}
		})
	}
	if checked == 0 {
		t.Error("no catalog item ships a reference copy; this test is asserting nothing")
	}
}

// A .configs folder that yields no schema is a mistake, not a port without one — a
// port without one has no folder. It is the failure a suffix convention invites: a
// schema whose name is not <something>.schema.json is ignored rather than refused, so
// the port silently loses its Config tab with nothing to say why.
func TestEveryConfigFolderYieldsASchema(t *testing.T) {
	app := newCatalogApp(t)
	root := filepath.Join(app.metadataPath, metadata.PortItemType)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name(), metadata.ConfigSchemaDir)
		found, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		files, err := metadata.LoadConfigSchemas(app.metadataPath, e.Name())
		if err != nil {
			t.Errorf("%s: %v", e.Name(), err)
			continue
		}
		if len(files) == 0 {
			var names []string
			for _, f := range found {
				names = append(names, f.Name())
			}
			t.Errorf("%s has a %s folder holding %v, and none of it is a schema: a schema is named %q",
				e.Name(), metadata.ConfigSchemaDir, names, schema.SchemaSuffix)
			continue
		}
		checked++
	}
	if checked == 0 {
		t.Error("no catalog item has a config folder; this test is asserting nothing")
	}
}
