package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/zamiba/forge/engine"
	"github.com/zamiba/go-mediaitems-profiles/profile"

	"portforge/metadata"
	"portforge/models"
)

// Tier 2 of profiles: a port that keeps its saves beside its own files — the
// ones a spec names in userDataPaths — gets those paths *linked* into the
// active profile. The real files live in the profile, at the same relative
// path they had under the port's folder; the port's folder holds a symlink
// (a junction on Windows) pointing at them. The game writes where it always
// did and the data lands in the profile, where the other tier — ${profilePath}
// — already puts it, and where a sync tool finds it.
//
// A port that writes somewhere else — its own folder under ~/.config or
// %APPDATA%, say — and takes no flag to redirect it is reached the same way
// through an object entry in the same list: {locationType, path} names one
// of the platform's per-user folders and a path beneath it, and that place
// gets the link. The spec never holds a real location, only a type's name
// and a relative path; the engine resolves the type and keeps the path
// beneath it, and PortForge refuses a place inside its own folders. That is
// the confinement a synced catalog needs. The profile side of such an entry
// is <path> alone, not <type>/<path>: a port that writes to linuxData on one
// machine and windowsRoaming on another keeps one set of saves in a profile
// the two share, as its in-tree paths already do.
//
// Linking runs at startup, after an install (data from before profiles existed
// is moved in), when a profile is switched or created, before a launch and
// after a session (data the game created for the first time is moved in before
// the profile is synced). It is the same idempotent pass each time. Nothing
// here ever deletes data: a path with data on both sides is left alone and
// reported, not merged.

// Link states, as GetSaveLinks reports them.
const (
	linkLinked      = "linked"      // the port's path points into the profile
	linkPending     = "pending"     // nothing exists yet on either side
	linkConflict    = "conflict"    // data on both sides; nothing was touched
	linkUnsupported = "unsupported" // this platform cannot link this kind of path
	linkError       = "error"       // the filesystem refused
)

// SaveLink is one declared path and where it stands.
type SaveLink struct {
	Path     string `json:"path"`               // as declared: relative to the port's folder, or <root>/<path>
	Location string `json:"location,omitempty"` // where that is on this machine, for a userData entry
	State    string `json:"state"`              // one of the link* constants
	Reason   string `json:"reason,omitempty"`   // for conflict, unsupported and error
}

// SaveLinkStatus is what the game page asks for: which profile this port's
// saves go to, and whether every declared path is actually going there.
type SaveLinkStatus struct {
	Profile string     `json:"profile"` // display name
	Slug    string     `json:"slug"`
	Folder  string     `json:"folder"` // the profile's folder for this port
	Links   []SaveLink `json:"links"`
	// Failed is true when at least one path is in a state the player should
	// know about — conflict, unsupported or error. Pending is not a failure.
	Failed bool `json:"failed"`
}

// GetSaveLinks reports the link state of every userDataPaths entry for an
// installed port without changing anything.
func (a *App) GetSaveLinks(itemTitle string) (SaveLinkStatus, error) {
	versionDir := filepath.Join(a.dataPath, metadata.PortItemType, itemTitle)
	state, err := metadata.ReadInstallState(versionDir)
	if err != nil || state == nil || !state.Installed {
		return SaveLinkStatus{}, nil
	}
	p, err := a.activeProfile()
	if err != nil {
		return SaveLinkStatus{}, err
	}
	paths, entries, err := a.installedUserData(itemTitle, versionDir, state)
	if err != nil {
		return SaveLinkStatus{}, err
	}
	return a.saveLinks(itemTitle, versionDir, paths, entries, p, true)
}

// RevealSaves opens the active profile's folder for this port in the file
// manager, creating it if the game has not yet.
func (a *App) RevealSaves(itemTitle string) error {
	p, err := a.activeProfile()
	if err != nil {
		return err
	}
	dir, err := p.ItemDir(metadata.PortItemType, itemTitle)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return openInFileManager(dir)
}

// linkSaves is the acting form: it moves and links, logs what could not be
// done, and never fails the caller — a game whose saves could not be linked
// still runs, with its saves where they were.
func (a *App) linkSaves(itemTitle, versionDir string, state *models.InstallState, p profile.Profile) {
	paths, entries, err := a.installedUserData(itemTitle, versionDir, state)
	if err != nil {
		log.Printf("save links for %s: %v", itemTitle, err)
		return
	}
	status, err := a.saveLinks(itemTitle, versionDir, paths, entries, p, false)
	if err != nil {
		log.Printf("save links for %s: %v", itemTitle, err)
		return
	}
	for _, l := range status.Links {
		if l.State == linkConflict || l.State == linkUnsupported || l.State == linkError {
			log.Printf("save links for %s: %s: %s — %s", itemTitle, l.Path, l.State, l.Reason)
		}
	}
}

// unlinkUserData removes the links a port's entries outside the tree planted
// there, when they point into a profile. Anything else at those places —
// the port's own data, if it was never linked, or a link someone else made —
// is left alone.
func (a *App) unlinkUserData(itemTitle, versionDir string, state *models.InstallState) {
	if a.profiles == nil {
		return
	}
	_, entries, err := a.installedUserData(itemTitle, versionDir, state)
	if err != nil {
		log.Printf("save links for %s: %v", itemTitle, err)
		return
	}
	for _, e := range entries {
		location, err := a.userDataLocation(e)
		if err != nil {
			continue
		}
		if target, ok := readLink(location); ok && within(target, a.profiles.Dir()) {
			if err := os.Remove(location); err != nil {
				log.Printf("save links for %s: could not remove %s: %v", itemTitle, location, err)
			}
		}
	}
}

// installedUserData returns what an installed port declares as the player's:
// the in-tree paths of its userDataPaths, interpolated and relative to
// versionDir, and the entries outside the tree that are for this platform.
//
// Both come from the catalog's spec for the installed version, with the args
// the build had: a spec that learns where a port keeps its saves after the
// install should reach that install, not wait for a reinstall. The paths the
// install recorded are the fallback for a spec the catalog no longer has.
func (a *App) installedUserData(itemTitle, versionDir string, state *models.InstallState) ([]string, []engine.UserDataPath, error) {
	specs, err := metadata.LoadInstallationSpecs(a.metadataPath, itemTitle)
	if err != nil {
		return nil, nil, err
	}
	platform := state.TargetPlatform
	if platform == "" {
		platform = a.resolveTargetPlatform(specs)
	}
	spec := engine.Select(specs, platform, state.InstalledVersion)
	if spec == nil {
		return state.UserDataPaths, nil, nil
	}
	// An entry outside the tree is for the platform its location type names;
	// the ones for another platform are not this machine's business.
	var entries []engine.UserDataPath
	for _, e := range spec.UserData {
		if !e.Outside() {
			continue
		}
		if _, err := e.Location(); errors.Is(err, engine.ErrOtherPlatform) {
			continue
		}
		entries = append(entries, e)
	}
	return userDataPathsOf(spec, platform, state.InstalledVersion, state.Args), entries, nil
}

// userDataPathsOf interpolates a spec's userDataPaths the way the engine
// would for a run with these args: the same namespaced map, so a path written
// as `install/${args.region}/saves` or `${platform.dataDir}` resolves the same
// here as in a deletePath.
func userDataPathsOf(spec *engine.Spec, platform, version string, args map[string]string) []string {
	if len(spec.UserDataPaths) == 0 {
		return nil
	}
	platformVars, versionVars := spec.VarsFor(platform, version)
	vars := make(map[string]string, len(args)+len(platformVars)+len(versionVars)+2)
	for k, v := range args {
		vars["args."+k] = v
	}
	for k, v := range platformVars {
		vars["platform."+k] = v
	}
	for k, v := range versionVars {
		vars["version."+k] = v
	}
	if platform != "" {
		vars["platform"] = platform
	}
	if version != "" {
		vars["version"] = version
	}
	out := make([]string, 0, len(spec.UserDataPaths))
	for _, p := range spec.UserDataPaths {
		out = append(out, engine.Interpolate(p, vars))
	}
	return out
}

// saveLinks classifies every declared path and, unless dry, brings it to the
// linked state where that can be done without touching data. A path inside
// the port's folder mirrors its relative path under the profile's folder for
// the port; an entry outside it mirrors its path there, the same on every
// platform.
func (a *App) saveLinks(itemTitle, versionDir string, paths []string, entries []engine.UserDataPath, p profile.Profile, dry bool) (SaveLinkStatus, error) {
	itemDir, err := p.ItemDir(metadata.PortItemType, itemTitle)
	if err != nil {
		return SaveLinkStatus{}, err
	}
	status := SaveLinkStatus{Profile: p.Name, Slug: p.Slug, Folder: itemDir}
	add := func(l SaveLink, port, prof string, err error) {
		if err != nil {
			l.State, l.Reason = linkError, err.Error()
		} else {
			l.State, l.Reason = linkOne(port, prof, dry)
		}
		if l.State == linkConflict || l.State == linkUnsupported || l.State == linkError {
			status.Failed = true
		}
		status.Links = append(status.Links, l)
	}
	for _, rel := range paths {
		clean, err := confinedRel(rel)
		add(SaveLink{Path: rel}, filepath.Join(versionDir, clean), filepath.Join(itemDir, clean), err)
	}
	for _, e := range entries {
		l := SaveLink{Path: e.LocationType + ":" + e.Path}
		port, err := a.userDataLocation(e)
		l.Location = port
		add(l, port, filepath.Join(itemDir, filepath.FromSlash(e.Path)), err)
	}
	return status, nil
}

// userDataLocation resolves an entry outside the tree to the place on this
// machine the port writes to. The engine resolved the location type and kept
// the path beneath it; what is checked here is PortForge's own concern — that
// the place is not one of PortForge's own folders, which hold every profile
// and would link a profile into itself.
func (a *App) userDataLocation(e engine.UserDataPath) (string, error) {
	location, err := e.Location()
	if err != nil {
		return "", err
	}
	for _, own := range a.ownFolders() {
		if own != "" && (within(location, own) || within(own, location)) {
			return "", fmt.Errorf("userDataPaths: %s:%s is where PortForge keeps its own data", e.LocationType, e.Path)
		}
	}
	return location, nil
}

// ownFolders lists the places an entry outside the tree must never touch:
// the folder holding every profile and the storage-unit list beside it,
// PortForge's settings, the catalog, and the storage unit the ports are
// installed on.
func (a *App) ownFolders() []string {
	own := []string{a.metadataPath, a.dataPath}
	if a.profiles != nil {
		own = append(own, filepath.Dir(a.profiles.Dir()))
	}
	if dir, err := configDir(); err == nil {
		own = append(own, dir)
	}
	return own
}

// within reports whether path is dir or inside it, by name.
func within(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// confinedRel cleans a declared path and refuses one that would leave the
// port's folder. The spec is synced from a public catalog; a path in it must
// not be able to link somewhere else on the machine into a profile.
func confinedRel(rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", fmt.Errorf("userDataPaths: %q is not a relative path", rel)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("userDataPaths: %q leaves the port's folder", rel)
	}
	return clean, nil
}

// linkOne brings src (in the port's folder) to a link at dst (in the profile),
// or says why not. Four situations, by what exists where:
//
//   - src is already a link into this profile: done.
//   - src is a link elsewhere (the previous profile): the link is replaced;
//     whether a directory or a file goes at dst is learnt from the old target.
//   - only one side has real data: it is the profile's, moved there if it was
//     the port's, and src becomes the link.
//   - both sides have real data: conflict. Neither is touched.
//
// Nothing existing on either side is pending: the game has not written yet.
// The link is made once it has, on the next pass.
func linkOne(src, dst string, dry bool) (state, reason string) {
	kind := kindUnknown
	if target, ok := readLink(src); ok {
		if sameFile(target, dst) {
			return linkLinked, ""
		}
		kind = kindOf(target)
		if dry {
			return linkPending, "" // would be re-pointed on the next pass
		}
		if err := os.Remove(src); err != nil {
			return linkError, fmt.Sprintf("could not remove the previous link: %v", err)
		}
	}

	_, srcErr := os.Lstat(src)
	srcExists := srcErr == nil
	dstInfo, dstErr := os.Lstat(dst)
	dstExists := dstErr == nil

	switch {
	case srcExists && dstExists:
		return linkConflict, "the game's folder and the profile both hold data at this path; move one aside to let PortForge link them"
	case !srcExists && !dstExists && kind == kindUnknown:
		return linkPending, ""
	}
	if dry {
		return linkPending, ""
	}

	if srcExists {
		// The port's data becomes the profile's. A move, so a large save
		// directory costs nothing on the same filesystem.
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return linkError, err.Error()
		}
		if err := engine.MovePath(src, dst); err != nil {
			return linkError, fmt.Sprintf("could not move the data into the profile: %v", err)
		}
		dstInfo, dstErr = os.Lstat(dst)
		dstExists = dstErr == nil
	}
	if !dstExists {
		// The previous profile had this path; the new one does not yet. A
		// directory is made so the game finds it; a file is left for the
		// game to create through the link.
		if kind == kindDir {
			if err := os.MkdirAll(dst, 0755); err != nil {
				return linkError, err.Error()
			}
			dstInfo, _ = os.Lstat(dst)
		}
	}
	isDir := kind == kindDir || (dstInfo != nil && dstInfo.IsDir())
	if err := os.MkdirAll(filepath.Dir(src), 0755); err != nil {
		return linkError, err.Error()
	}
	if err := makeLink(dst, src, isDir); err != nil {
		if errors.Is(err, errLinkUnsupported) {
			return linkUnsupported, err.Error()
		}
		return linkError, err.Error()
	}
	return linkLinked, ""
}

type pathKind int

const (
	kindUnknown pathKind = iota
	kindDir
	kindFile
)

func kindOf(path string) pathKind {
	info, err := os.Stat(path)
	if err != nil {
		return kindUnknown
	}
	if info.IsDir() {
		return kindDir
	}
	return kindFile
}

// sameFile reports whether two paths name the same place once cleaned. Links
// are written with absolute targets, so a string comparison after Clean is
// enough; EvalSymlinks would follow the very link being checked.
func sameFile(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

// errLinkUnsupported is returned by makeLink where the platform cannot make
// this kind of link at all, as opposed to having failed to.
var errLinkUnsupported = errors.New("this kind of path cannot be linked on this platform")
