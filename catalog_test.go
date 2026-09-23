package main

import (
	"os"
	"path/filepath"
	"testing"

	"portforge/metadata"

	"github.com/zamiba/forge/engine"
)

// Every (version, platform) pair a spec declares has to be reachable: the
// version picker offers exactly these combinations, so one that Select cannot
// resolve is a dead entry in the UI. A misspelled platform name is invisible
// until someone on that platform tries to install.
func TestCatalogSpecsAreSelectable(t *testing.T) {
	const catalog = "mediaitems"
	dirs, err := os.ReadDir(filepath.Join(catalog, metadata.PortItemType))
	if err != nil {
		t.Skip("mediaitems submodule not checked out")
	}

	known := map[string]bool{
		"Linux": true, "Windows": true, "Mac": true,
		"Linux-x64": true, "Linux-arm64": true,
		"Windows-x64": true, "Windows-arm64": true,
		"Mac-x64": true, "Mac-arm64": true,
	}

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		t.Run(d.Name(), func(t *testing.T) {
			file, err := metadata.LoadSpecFile(catalog, d.Name())
			if err != nil {
				t.Fatalf("LoadSpecFile: %v", err)
			}
			if file == nil {
				t.Skip("no spec file")
			}
			declared := engine.Versions(file.Specs)
			if len(declared) == 0 {
				t.Errorf("declares no versions, so the picker has nothing to offer")
			}
			for _, v := range declared {
				if v.Version == "" {
					t.Errorf("a build declares no version")
				}
				for _, p := range v.Platforms {
					if !known[p] {
						t.Errorf("version %q targets unknown platform %q", v.Version, p)
					}
					spec := engine.Select(file.Specs, p, v.Version)
					if spec == nil {
						t.Errorf("version %q platform %q resolves to no spec", v.Version, p)
						continue
					}
					var exe bool
					for _, s := range spec.Steps {
						if s.Step == "defineExecutable" {
							exe = true
						}
					}
					if !exe {
						t.Errorf("version %q platform %q builds nothing launchable", v.Version, p)
					}
				}
			}
			if dv := file.DefaultVersion; dv != "" {
				found := false
				for _, v := range declared {
					if v.Version == dv {
						found = true
					}
				}
				if !found {
					t.Errorf("defaultVersion %q is not one of the declared versions", dv)
				}
			}
		})
	}
}

// The host a user is actually on has to reach a build. This is the check that
// would have caught the catalog being Linux-only.
func TestCatalogCoversHostPlatforms(t *testing.T) {
	const catalog = "mediaitems"
	dirs, err := os.ReadDir(filepath.Join(catalog, metadata.PortItemType))
	if err != nil {
		t.Skip("mediaitems submodule not checked out")
	}

	hosts := []string{"Linux-x64", "Windows-x64", "Mac-arm64", "Mac-x64"}
	coverage := map[string][]string{}

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		file, err := metadata.LoadSpecFile(catalog, d.Name())
		if err != nil || file == nil {
			continue
		}
		for _, host := range hosts {
			p := resolvePlatform(file.Specs, host)
			if engine.Select(file.Specs, p, "") != nil {
				coverage[host] = append(coverage[host], d.Name())
			}
		}
	}
	for _, host := range hosts {
		t.Logf("%-12s %d ports installable", host, len(coverage[host]))
	}
	if len(coverage["Linux-x64"]) == 0 {
		t.Error("no port is installable on Linux")
	}
}

// A $name that no arg declares is left in the string verbatim by the engine,
// on the reasoning that a visibly wrong path beats a silently truncated one.
// That is right for the engine and useless for a catalog: nothing fails until
// the step runs, and what surfaces is the tool's own error about a nonsensical
// argument rather than anything pointing at the spec.
//
// Render96 shipped "VERSION=$romVersion" with only textureMod declared. The
// build ran for six steps, then make passed the literal string to
// extract_assets.py, which answered with its usage text.
// Every variable a catalog spec interpolates must resolve, and every reference
// must be braced. This runs the engine's own checks rather than a parallel
// implementation: a hand-rolled copy drifts from the real one silently, and the
// drift shows up as a spec that passes here and fails on a user's machine.
func TestCatalogSpecsDeclareEveryVariableTheyUse(t *testing.T) {
	const catalog = "mediaitems"
	dirs, err := os.ReadDir(filepath.Join(catalog, metadata.PortItemType))
	if err != nil {
		t.Skip("mediaitems submodule not checked out")
	}

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		t.Run(d.Name(), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(catalog, metadata.PortItemType, d.Name(), ".forge.json"))
			if os.IsNotExist(err) {
				t.Skip("no .forge.json")
			}
			if err != nil {
				t.Fatal(err)
			}
			file, err := engine.ParseSpecFile(raw)
			if err != nil {
				t.Fatalf("parse .forge.json: %v", err)
			}
			for i := range file.Specs {
				spec := &file.Specs[i]
				if missing := engine.UndeclaredArgs(spec); len(missing) > 0 {
					t.Errorf("build %d uses %s, which nothing declares", i, joinVarNames(missing))
				}
				if stale := engine.UnbracedRefs(spec); len(stale) > 0 {
					t.Errorf("build %d writes %s without braces, so it is not substituted", i, joinArgNames(stale))
				}
			}
		})
	}
}

// An entry outside the tree is checked by the engine at parse time — the
// type, the path staying beneath it — so every one the catalog declares is
// resolvable on the platform it names. What is checked here is that they parse
// at all, and how many there are, so a catalog without any is noticed.
func TestCatalogEntriesOutsideTheTreeParse(t *testing.T) {
	app := newCatalogApp(t)
	versions, err := app.GetVersions()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, v := range versions {
		file, err := metadata.LoadSpecFile(app.metadataPath, v.ItemTitle)
		if err != nil {
			t.Errorf("%s: %v", v.ItemTitle, err)
			continue
		}
		if file == nil || len(file.Specs) == 0 {
			continue
		}
		for _, e := range file.Specs[0].UserData {
			if e.Outside() {
				checked++
			}
		}
	}
	if checked == 0 {
		t.Error("no entry outside the tree in the catalog; melee-pc and reBlue declare them")
	}
	t.Logf("%d entries checked", checked)
}
