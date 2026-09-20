package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

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

// ProfileInfo is a profile as the Settings page sees it.
type ProfileInfo struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	CreatedBy string `json:"createdBy,omitempty"`
	Active    bool   `json:"active"`
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
		out = append(out, ProfileInfo{Slug: p.Slug, Name: p.Name, CreatedBy: p.CreatedBy, Active: p.Slug == active.Slug})
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
	if err != nil {
		return ProfileInfo{}, err
	}
	a.prefs.ActiveProfile = p.Slug
	if err := savePreferences(a.prefsPath, a.prefs); err != nil {
		return ProfileInfo{}, err
	}
	return ProfileInfo{Slug: p.Slug, Name: p.Name, CreatedBy: p.CreatedBy, Active: true}, nil
}

// SetActiveProfile makes saves go to the named profile from the next launch on.
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
	if _, err := a.profiles.Get(slug); err != nil {
		return err
	}
	a.prefs.ActiveProfile = slug
	return savePreferences(a.prefsPath, a.prefs)
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
