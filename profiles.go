package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/zamiba/forge/engine"
	"github.com/zamiba/go-mediaitems-profiles/profile"
	"github.com/zamiba/go-mediaitems-profiles/profilesync"

	"portforge/metadata"
)

// Profiles are the suite's shared notion of a person: a folder, beside the
// storage-unit list, that holds everything recorded about them — game saves
// here, achievements and watched episodes in other programs. PortForge does
// not require anyone to create one. When nobody has, saves go to a profile
// PortForge makes for itself, named after it, which is an ordinary profile
// that can be renamed or replaced later. Having a profile always — rather
// than a with-profile and a without-profile path — is what lets someone adopt
// profiles later without moving anything.
//
// PortForge only ever writes inside its own item folders in a profile, and
// never syncs, snapshots or merges one: that is the profiles module's job, or
// whatever the user runs over the folder.

// defaultProfileName is the profile PortForge uses when the user has not
// chosen one. It is created by Ensure on first use, so a fresh install has a
// profile the moment it needs one.
const defaultProfileName = "portforge"

// ProfileInfo is a profile as the frontend sees it.
type ProfileInfo struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	CreatedBy string `json:"createdBy,omitempty"`
	Active    bool   `json:"active"`
	// Picture is a URL the frontend can load, or empty. It carries the file's
	// modification time so a changed picture is not served from cache.
	Picture string `json:"picture,omitempty"`
}

func (a *App) profileInfo(p profile.Profile, active bool) ProfileInfo {
	info := ProfileInfo{Slug: p.Slug, Name: p.Name, CreatedBy: p.CreatedBy, Active: active}
	if p.Picture != "" {
		v := ""
		if st, err := os.Stat(p.Picture); err == nil {
			v = fmt.Sprintf("?v=%d", st.ModTime().UnixNano())
		}
		info.Picture = profilePictureRoute + url.PathEscape(p.Slug) + v
	}
	return info
}

// activeProfile returns the profile saves go to right now. A preference that
// names a profile that no longer exists — the folder was deleted, or the
// preferences file came from another machine — falls back to the default and
// is cleared, with a line in the log, rather than failing every launch.
func (a *App) activeProfile() (profile.Profile, error) {
	if a.profiles == nil {
		return profile.Profile{}, errors.New("profiles are unavailable: no configuration directory")
	}
	if slug := a.prefs.ActiveProfile; slug != "" {
		p, err := a.profiles.Get(slug)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, profile.ErrNotFound) {
			return profile.Profile{}, err
		}
		log.Printf("profiles: the active profile %q is gone; using the default", slug)
		a.prefs.ActiveProfile = ""
		_ = savePreferences(a.prefsPath, a.prefs)
		// Said once, since the preference is cleared: the person's saves are
		// about to go somewhere they did not choose, and a log line is not
		// where they would look for that.
		a.emit("profile:missing", map[string]string{"slug": slug, "using": defaultProfileName})
	}
	return a.profiles.Ensure(defaultProfileName, "portforge")
}

// GetProfiles lists every profile on this device, with the active one marked.
// The default is created here if it is missing, so the list is never empty and
// the Settings page always has something to show as active.
func (a *App) GetProfiles() ([]ProfileInfo, error) {
	active, err := a.activeProfile()
	if err != nil {
		return nil, err
	}
	all, err := a.profiles.List()
	if err != nil {
		return nil, err
	}
	out := make([]ProfileInfo, 0, len(all))
	for _, p := range all {
		out = append(out, a.profileInfo(p, p.Slug == active.Slug))
	}
	return out, nil
}

// CreateProfile makes a new profile and switches to it: someone creating a
// profile from the Settings page means to use it, and the two-step version
// leaves a window where a launch still goes to the old one.
func (a *App) CreateProfile(name string) (ProfileInfo, error) {
	if a.profiles == nil {
		return ProfileInfo{}, errors.New("profiles are unavailable: no configuration directory")
	}
	if err := a.refuseWhilePlaying("switch profiles"); err != nil {
		return ProfileInfo{}, err
	}
	p, err := a.profiles.Create(name, "portforge")
	switch {
	case errors.Is(err, profile.ErrEmptyName):
		return ProfileInfo{}, errors.New("give the profile a name")
	case errors.Is(err, profile.ErrExists):
		// Named by slug, not by what was typed: "Sam" and "sam" are the same
		// profile, and the folder name is what the two collide on.
		return ProfileInfo{}, fmt.Errorf("a profile called %s already exists", profile.Slugify(name))
	case err != nil:
		return ProfileInfo{}, err
	}
	a.prefs.ActiveProfile = p.Slug
	if err := savePreferences(a.prefsPath, a.prefs); err != nil {
		return ProfileInfo{}, err
	}
	a.relinkInstalledPorts(p)
	return a.profileInfo(p, true), nil
}

// SetActiveProfile makes saves go to the named profile, from this moment.
// Refused while a game runs: the running game keeps writing to the profile it
// was started with, and a switch under it would leave the screen saying one
// thing and the disk doing another.
func (a *App) SetActiveProfile(slug string) error {
	if a.profiles == nil {
		return errors.New("profiles are unavailable: no configuration directory")
	}
	if err := a.refuseWhilePlaying("switch profiles"); err != nil {
		return err
	}
	p, err := a.profiles.Get(slug)
	if err != nil {
		return err
	}
	a.prefs.ActiveProfile = slug
	if err := savePreferences(a.prefsPath, a.prefs); err != nil {
		return err
	}
	a.relinkInstalledPorts(p)
	return nil
}

// relinkInstalledPorts points every installed port's save links at the given
// profile. It is what makes a switch take effect at once rather than at the
// next launch: a port started outside PortForge in between, or its folder
// opened by hand, would otherwise still be writing to the old profile. The
// launch-time linking stays, for a preference edited behind PortForge's back.
func (a *App) relinkInstalledPorts(p profile.Profile) {
	if a.dataPath == "" {
		return
	}
	entries, err := os.ReadDir(filepath.Join(a.dataPath, metadata.PortItemType))
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		versionDir := filepath.Join(a.dataPath, metadata.PortItemType, e.Name())
		state, err := metadata.ReadInstallState(versionDir)
		if err != nil || state == nil || !state.Installed {
			continue
		}
		a.linkSaves(e.Name(), versionDir, state, p)
	}
}

// DeleteProfile removes a profile from this device — the folder and everything
// in it, for every program that used it. The active one cannot be deleted:
// switch away first, so there is never a moment with saves going nowhere.
// The default is an ordinary profile here; if it is deleted while not active
// it simply reappears, empty, the next time nothing else is chosen.
func (a *App) DeleteProfile(slug string) error {
	if a.profiles == nil {
		return errors.New("profiles are unavailable: no configuration directory")
	}
	active, err := a.activeProfile()
	if err != nil {
		return err
	}
	if slug == active.Slug {
		return fmt.Errorf("%s is in use; switch to another profile first", active.Name)
	}
	if err := a.profiles.Delete(slug); errors.Is(err, profile.ErrNotFound) {
		return fmt.Errorf("there is no profile called %s any more", slug)
	} else if err != nil {
		return err
	}
	return nil
}

// profilePictureRoute is where the asset handler serves a profile's picture
// from; see assets.go.
const profilePictureRoute = "/profiles/picture/"

// SetProfilePicture asks for an image and makes it the profile's picture. The
// module does the cropping and scaling, so the file handed over is the one
// the person picked; what lands in the profile folder is its 256px square.
func (a *App) SetProfilePicture(slug string) error {
	if a.profiles == nil {
		return errors.New("profiles are unavailable: no configuration directory")
	}
	if err := a.requireNativeDialogs("Choosing a picture"); err != nil {
		return err
	}
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Choose a profile picture",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Images", Pattern: "*.png;*.jpg;*.jpeg;*.gif"},
		},
	})
	if err != nil {
		return err
	}
	if path == "" {
		return nil // dismissed
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := a.profiles.SetPicture(slug, f); err != nil {
		if errors.Is(err, profile.ErrBadPicture) {
			return errors.New("that file is not a PNG, JPEG or GIF image")
		}
		return err
	}
	return nil
}

// RemoveProfilePicture takes the picture away; the profile shows its initial again.
func (a *App) RemoveProfilePicture(slug string) error {
	if a.profiles == nil {
		return errors.New("profiles are unavailable: no configuration directory")
	}
	return a.profiles.RemovePicture(slug)
}

// profilePicture returns the path of a profile's picture file, for the asset
// handler. Empty when there is none. The module names the file; nothing here
// builds a path inside a profile.
func (a *App) profilePicture(slug string) string {
	if a.profiles == nil {
		return ""
	}
	p, err := a.profiles.Get(slug)
	if err != nil {
		return ""
	}
	return p.Picture
}

// RenameProfile changes a profile's display name. The folder — and so every
// path recorded against it — stays where it is.
func (a *App) RenameProfile(slug, name string) error {
	if a.profiles == nil {
		return errors.New("profiles are unavailable: no configuration directory")
	}
	return a.profiles.Rename(slug, name)
}

// openProfileSync starts the notifier that sends a changed profile wherever the
// user has asked for it to go — a git commit, an rclone copy — after each play
// session. PortForge does not know which, if any; it says "this profile
// changed" and shows what came back. Nothing configured means every call is
// a quiet no-op.
func (a *App) openProfileSync() {
	n, err := profilesync.Open(a.profiles, "portforge", profilesync.Options{})
	if err != nil {
		log.Printf("profile sync: %v", err)
		return
	}
	// Called from the notifier's goroutine: hand it to the event hub and the
	// log, touch nothing else.
	n.OnResult(func(r profilesync.Result) {
		if r.Err != nil {
			log.Printf("profile sync %s (%s): %v", r.Slug, r.Kind, r.Err)
		}
		a.emit("profile:synced", map[string]interface{}{
			"slug":   r.Slug,
			"kind":   r.Kind,
			"reason": r.Reason,
			"output": r.Output,
			"error":  errString(r.Err),
		})
	})
	a.sync = n
}

// profileChanged tells the notifier a profile has new data. Safe with no
// notifier and with no slug, so callers need not check either.
func (a *App) profileChanged(slug, reason string) {
	if a.sync == nil || slug == "" {
		return
	}
	a.sync.Changed(slug, reason)
}

// shutdown lets an in-flight profile sync finish before the process goes:
// a commit half-made because the window closed is what Close is for.
func (a *App) shutdown(context.Context) {
	if a.sync != nil {
		_ = a.sync.Close()
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// refuseWhilePlaying is the guard for anything that would change where a
// running game's data goes.
func (a *App) refuseWhilePlaying(action string) error {
	a.playMu.Lock()
	defer a.playMu.Unlock()
	if a.playing != "" {
		return fmt.Errorf("cannot %s while %s is running", action, a.playing)
	}
	return nil
}

// profileProvider answers ${profilePath} in a launch argument: the active
// profile's folder for this port, created on first use. The port's own flag
// names what the folder is for — "--save-dir", "--userdata" — and PortForge
// supplies only the path, so ports with different flags need no different
// handling. Ports whose data cannot be redirected by a flag are handled by
// linking their userDataPaths into the same folder instead.
func (a *App) profileProvider(itemTitle string) engine.Provider {
	return engine.ProviderFunc(func(ctx context.Context, req engine.ProviderRequest) (string, error) {
		if req.Src != "" {
			return "", fmt.Errorf("${profilePath.%s}: the profile path takes no name", req.Src)
		}
		p, err := a.activeProfile()
		if err != nil {
			return "", err
		}
		dir, err := p.ItemDir(metadata.PortItemType, itemTitle)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
		return dir, nil
	})
}
