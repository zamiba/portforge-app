package main

import (
	"os"
	"path/filepath"
	"testing"

	"portforge/metadata"

	"github.com/zamiba/config-forge/schema"
)

// Every config schema in the catalog is valid, and every file it describes is
// declared as user data by that port's spec.
//
// That second check is the one worth having. A config file the spec does not
// list in userDataPaths is not protected: uninstalling deletes it, reinstalling
// overwrites it, and it is never linked into the profile. Editing it from
// PortForge would write settings that silently vanish on the next update. The
// dependency runs one way — forge decides what is user data, a schema only
// annotates it.
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

			userData := map[string]bool{}
			specs, err := metadata.LoadInstallationSpecs(app.metadataPath, item)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range specs {
				for _, p := range s.UserDataPaths {
					userData[filepath.ToSlash(p)] = true
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
				if !userData[filepath.ToSlash(f.Path)] {
					t.Errorf("%q describes %s, which the spec does not list in userDataPaths: "+
						"an uninstall would delete it and it is never linked into a profile",
						f.Title, f.Path)
				}
			}
			checked++
		})
	}
	if checked == 0 {
		t.Error("no catalog item carries a config schema; this test is asserting nothing")
	}
}

func hasString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
