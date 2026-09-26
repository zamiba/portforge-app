# Changelog

All notable changes to PortForge are recorded here.

## v0.3.6-alpha — 2026-09-26

### Added

**A port that keeps its settings outside its own folder can have them edited too.**
The Config tab used to look for a config file under the port's own folder, which is
where most of them are — as a link the profile owns. Some ports keep theirs
elsewhere: in one of the operating system's own data folders, or written straight
into the profile by a port that takes a save-location flag. PortForge now looks in
the profile as well, which is the one place every config file genuinely lives
whichever of the three routes put it there, so those ports get a Config tab on the
same terms as the rest. The port's own folder is still looked at first, and still
matters: it is where the file is when a port's data is not linked to a profile at
all, and reading only the profile would have reported a file that exists as one the
game had not written yet.

**Config editing covers two more kinds of settings file.** The grammars a port's
settings can be written in now include TOML and the plain `key value` form a C
program writes, which between them were the last thing standing between the catalog
and a Config tab on every port in it.

**A game's config files can be completed, so every setting it has is in them.** A
game writes a section of its config only once something in it is touched, which left
whole groups of settings with nowhere to be written and so nothing to edit. The
catalog can now ship a reference copy of a config file — the complete file, with every
setting at the value the game itself uses — and PortForge fills the gaps from it:
settings the game has not written are added at the game's own values, the sections
they belong in are added by copying them from the reference rather than by guessing,
and anything already in the file is left exactly as the game wrote it. A config file
the game has not created at all is shown from the reference and written the first time
you save or ask for the files to be completed, so a game that has never run can still
be set up before its first launch. Nothing is written for a release that does not have
it, and where a reference copy and the catalog's description of a setting disagree,
neither is used. A game whose config changes shape between releases can ship one
reference per shape, and the installed release decides which is used.

### Changed

**The Config tab only shows settings you can actually change.** A setting the
installed release stores differently, or one whose section the game has not written
and PortForge cannot add, used to appear greyed out with a line explaining itself.
Neither line offered anyone a choice — each stated something about the catalog that
nobody could act on from a settings page — so those settings are now left out
altogether, along with any section that held nothing else. A file with nothing
editable in it says so once instead of drawing an empty panel. The settings are still
noted in the log, because a catalog entry drifting away from the game it describes is
worth knowing about. A game that has not been launched yet is unaffected: it still
lists everything it will offer, since one launch is all that is missing.

### Fixed

**Saving a setting no longer reports itself as the game having changed it.** After a
save, PortForge reads the files back to show what is actually in them, and it compared
what it found against what it had read before — so the values it had just written
looked like values something else had moved, and the Config tab announced that the game
had been run and marked every setting that was saved. The comparison now only speaks for
changes PortForge did not make, which is what it was for: a game that really does rewrite
its config while the page is open is still reported, and still marked.

## v0.3.5-alpha — 2026-09-24

### Added

**A game's own settings can be edited from PortForge, in the profile they belong to.** A
port whose settings PortForge knows how to describe gets a Config tab beside Overview,
Mods and Options, listing its config files down one side and their settings the way the
game itself groups them: toggles, dropdowns, radio groups, sliders, text and paths, each
with the range or the choices the game actually accepts. Edits batch across every file and
one Save writes them, into the active profile's copy rather than a shared one, so two
people on the same machine keep separate graphics settings for the same install. PortForge
writes only the values listed on the page and leaves the rest of each file exactly as the
game wrote it — comments, key order, unknown entries and all — because the game is the
file's real author and rewriting it wholesale would lose everything the page does not
model. A game that is running owns its config, so editing pauses while it is up and the
files are read again when it quits; if the game changed a value in the meantime the row
says so. A setting the game has not written yet is still editable where PortForge knows
the game's own default for it, and each one can be put back to that default.

**Config editing is [config-forge](https://github.com/zamiba/config-forge) v0.0.3.** It
reads and writes the values inside a config file rather than the file itself, which is what
lets PortForge leave the rest of the file alone. It now covers four config grammars rather
than two, so the tab reaches most of the catalog instead of a couple of games. Keeping it a
separate module means anything else built on the suite gets the same editing, and the same
refusals, without reimplementing a grammar.

## v0.3.4-alpha — 2026-09-23

### Added

**A release can carry notices, and they are shown before you install it.** A version in
the catalog may now record caveats against itself — a bug that stops it launching on one
platform, something worth knowing before committing to the install — and PortForge shows
them at the top of the right-hand column, above Installation, for the version and the
build platform currently selected. They sit on the release rather than on the game, which
makes them a permanent record of that build instead of a live status: a version that
shipped broken stays marked, and a later release that fixes the problem simply carries no
notice, so nothing has to be un-said when the fix lands. A notice is markdown, so it can
link to the issue tracking the problem, and names the platforms it applies to — a
Windows-only bug stays silent on Linux, one naming no platform applies to all of them, and
naming the bare OS covers every architecture of it. Because the catalog is synced from
GitHub independently of app releases, a notice of a kind this version of PortForge does not
recognize is still shown, as a warning, rather than discarded: a warning that vanishes
because the app is behind the catalog would be worse than having none. The case that
prompted this is Ghostship on Windows, which cannot read its own folder when the path holds
a non-ASCII character and so refuses to launch from the name PortForge installs it under.

**A catalog update or index rebuild is shown in a status bar, wherever you are.** A sync
used to be visible only on the Settings page that started it, and the background refresh
at startup only as a one-line notice — so a library that went quiet or changed under you
had no explanation on screen. The catalog now reports what it is doing the same way an
install does: a bar across the top with the phase and its progress, whether the job was
started from Settings, from first-run setup, or by PortForge itself at startup. Clicking
it opens Settings; a failure stays in the bar until dismissed. The Settings page shows the
same state, so its buttons are disabled while a background refresh runs, and a page that
opens mid-sync picks it up rather than waiting for the next event. Rebuilding the index
is refused while a sync runs, since the sync rebuilds it itself.

**Saves go to a profile, and a port that takes a save-location flag receives its folder
at launch.** Profiles are the suite's shared notion of a person: a folder beside the
storage-unit list that every MediaItem program on the machine reads, so achievements and
watched episodes recorded by other programs sit next to the saves, and one folder can be
copied or synced to another device by whatever means the user prefers — PortForge itself
does not sync, snapshot or merge them. Nobody has to create one: first-run setup asks
whose saves these are — a profile of your own, or one PortForge makes, named `portforge`,
which is an ordinary profile that can be renamed or replaced. The sidebar's last row says
who is playing and opens the profile modal, where profiles are switched, renamed and
created, given a picture and deleted; Settings summarizes the active one and opens the
same modal. A switch takes effect at once — every installed port's save links are
re-pointed there and then — and is refused while a game runs. Deleting is a two-step act
with a warning worded for the whole suite, since the folder holds more than game saves;
the active profile cannot be deleted. A picture is cropped square and scaled by the
profiles module and kept inside the profile folder, so the other programs show the same
one. A spec can now put `${profilePath}` in a `defineExecutable` step's `args`, next to
the port's own flag — `"args": ["--save-dir", "${profilePath}"]` — and PortForge resolves
it to the active profile's folder for that port every time the game starts, creating it
on first use. A port that keeps its saves beside itself — the paths its spec lists in
`userDataPaths` — has those paths linked into the profile instead: the files move there,
the port's path becomes a link to them, and the game writes where it always did. A port
that writes to a per-user folder — `~/.local/share/<name>`, `%APPDATA%\<name>` — and
takes no flag to redirect it is reached the same way through an object entry in the same
list — `{ "locationType": "linuxData", "path": "melee-pc" }` — naming one of the
platform's per-user folders and a path beneath it; forge resolves the folder, PortForge
refuses a path that reaches its own folders, links the place and removes the link again
on uninstall. The profile side is the entry's path alone, so a port that writes to
`linuxData` on one machine and `windowsRoaming` on another keeps one set of saves in a
profile the two share. Links are made at startup, after an
install, when a profile is switched or created, before each launch and after each
session, so saves from before profiles existed are in the profile the first time PortForge
runs with one. Linking reads the catalog's current spec, so a port whose spec learns where
its saves are after the install is covered without a reinstall. Data is never merged; a path with data on both sides is left alone and
reported. On Windows, folders are linked as junctions and single files stay with the game.
When a session ends
PortForge tells the profiles module the profile changed, and the module sends it wherever
`MediaItem/profile-sync.json` says — a git commit if the profile is a repository, with a
push if a remote is named; rclone or Syncthing if configured; nothing otherwise — in the
background. A success is shown briefly; a failure stays in the notice bar until
dismissed, since it is the only sign that the saves did not leave the machine, and says
so: the saves themselves are safe. A game whose save folder could not be linked into the
profile — because something was already there — says so on its page, with a button that
reveals the folder. If the active profile's folder has vanished, PortForge falls back to
`portforge` and says so until dismissed. PortForge itself never runs a sync tool. Built on
the new `go-mediaitems-profiles` module (v0.2.0) and `go-mediaitems` v0.2.0.

### Changed

**The build engine is Forge v0.0.10-alpha**, which adds the object form of a
`userDataPaths` entry above and resolves its location types. It also adds `runDir`, the
location type for a path inside the port's own folder, so every entry in the list can now
be written the same way whichever side of the port's folder it is on — a bare string
still reads and means the same thing, and every protection the engine gives those paths
is unchanged either way.

**The dark/light toggle has moved from the sidebar to each page's top bar.** The
sidebar's bottom slot now belongs to the profile row.

**Release files are named after their version.** A download is
`portforge-0.3.4-alpha-linux-amd64` rather than `portforge-linux-amd64`, so two builds
kept side by side can be told apart, and the Windows and macOS files follow the same
pattern (`portforge-<version>-windows-amd64.exe`, `portforge-<version>-macos-universal.zip`).

### Fixed

**A spec that launches with `${profilePath}` is no longer refused at install.** The
installer checks that a spec reads only providers PortForge registers, and that check did
not know that launch arguments are left alone by the run and resolved at launch — where
`${profilePath}` exists — so the first catalog port to use it (Super Mario 64 Render96,
through its `--savepath` flag) could not be installed. The check now tells the run's steps
from the launch arguments, and names what is available in each.

**Covers no longer show a hairline of background along their top edge.** The tile's
one-pixel border shrank the box the image had to fit, so a 2:3 cover could not fill a 2:3
tile exactly and left a strip of background showing. The border is now drawn over the
image instead of around it.

**Every Markdown construct in a description is styled.** Descriptions have always been
rendered as Markdown, but only paragraphs and links had a look of their own; lists,
headings, code, quotes and rules fell back to the browser's defaults.

## v0.3.3-alpha — 2026-09-17

### Added

**The catalog is downloaded during first-run setup, and kept fresh after that.** A fresh
install used to arrive with an empty library and a setup screen that would not let you
past it until the catalog was synced — from a Settings page the setup screen does not
offer. Getting started now fetches the catalog and builds the index as its final step.
From then on PortForge checks for a newer catalog whenever it starts and fetches one in
the background when there is, with a notice once the library has been refreshed; the
check is one small request and the download happens only when something changed. A
switch on the Settings page turns it off. This is the first setting PortForge keeps for
itself since the library folder moved to the shared storage list; it lives in
`preferences.json` in the configuration directory.

## v0.3.2-alpha — 2026-09-16

### Added

**A port can be launched with arguments, including the path of its ROM.** A spec can give
`defineExecutable` an `args` list, and a `${romPath}` in it is resolved when the game is
launched — every time, against the library as it is then — rather than when it was
installed. So the disc is found after a storage unit moves, and a ROM added after
installing works without reinstalling. Launching a port whose ROM is missing is refused
with a message saying so, instead of starting a game that cannot find its data. This is
for the port that takes its disc on the command line and otherwise shows a chooser on
every start; the first such port in the catalog is melee-pc.

### Changed

**The build engine is Forge v0.0.8-alpha**, which adds the launch arguments above.

### Fixed

**GameCube disc images were invisible.** The catalog gained `GameCubeDiscImage` items with
Open Nectar, but PortForge's list of ROM types was never told, so v0.3.1 neither loaded
them from the catalog nor found them in a storage unit: the ROMs page showed Pikmin's
requirements with nothing that could satisfy them, and installing Open Nectar stopped with
"no ROM available" however many discs were present. The test that walks the catalog now
fails when a ROM type it references is unknown.

## v0.3.1-alpha — 2026-09-15

### Added

**An install can run the port's own installer against the ROM where it lives.** A spec
may run a file its own steps downloaded, and may hand any step the path of a ROM with
`${romPath}` instead of copying the ROM into the install. A port that extracts its assets
from a disc image now does so at install time, reading the disc where it sits in your
storage unit, rather than leaving it to a file dialog on first launch. The first port in
the catalog to use this is Open Nectar.

### Changed

**Storage units come from the shared `go-mediaitems` module.** PortForge's own copy of
the storage-unit code is gone, replaced by `github.com/zamiba/go-mediaitems/storageunit`
v0.1.0 — the same code every program in the suite now uses to read and write
`storage-units.json`. The file, its location and its format are unchanged, so nothing
needs migrating. Two things are better for it: a folder reached by two routes (a symlink,
a second mount) can no longer be added twice, and a file you have edited by hand keeps
the order you wrote it in.

**Browser mode catches up after a dropped connection.** Under `-server`, a tab that lost
its connection used to miss every event emitted while it was away — an install's progress
simply stopped. Events now carry a sequence number, the server keeps the most recent 1,024,
and a reconnecting browser is handed what it missed before live delivery resumes. The
desktop build is unaffected. A tab gone for longer than that loses the overflow; a freshly
loaded page never receives history, since it fetches its state on load.

**The build engine is Forge v0.0.7-alpha**, which adds the two capabilities above. A spec
that reads a provider PortForge does not have is refused before anything is downloaded.

## v0.3.0-alpha — 2026-09-12

### Added

**User data survives uninstalling and updating.** A port that keeps its save games or
configuration inside its own install folder used to lose them twice over: the teardown
removed `install/`, and a rebuild wrote over whatever was left. A spec can now declare
`userDataPaths`, and those paths are kept when uninstalling, when installing over an
existing install, and when a build fails partway through. Where a build ships its own
copy of a file you have edited, your copy wins.

Ports that declare nothing behave exactly as before.

**A real install section in the README.** Downloads are now the first thing on the page,
with a table of the release files, how to tell which of the two Linux builds your
distribution needs, and what to do about the unsigned-build warnings on macOS and
Windows. It also documents `-server` for the first time — the workaround for systems
where the window renders slowly — and what a `libjxl.so.0.11: cannot open shared object
file` error means, which is a mismatch between your own WebKitGTK and libjxl rather than
anything PortForge pins.

### Changed

**A port that declares no uninstall steps now gets a teardown that spares user data.**
Previously such a port had its install folder removed wholesale, with no way to make an
exception. PortForge now supplies the default teardown as a proper step, so
`userDataPaths` applies whether or not a port author wrote an uninstall sequence.

**Install spec variables are namespaced and must be braced.** A spec writes
`${args.region}` for an argument, `${platform}` and `${version}` for the build target,
and `${platform.slug}` for a variable a platform binds. The unbraced `$name` is no
longer substituted at all, so PortForge refuses to install a spec that still uses one —
it would otherwise build the wrong thing in silence, or skip a step whose condition can
never be true again. Every catalog spec that used a variable has been migrated.

**The build engine is [Forge](https://github.com/zamiba/forge) v0.0.6-alpha.**
Protecting user data across a rebuild moved into the engine, so it is identical here and
available to anything else built on it.

### Fixed

**An unreadable install spec no longer takes your save data with it.** Uninstalling
discarded the error from loading a port's spec and treated a broken file the same as a
missing one, which meant removing the install folder wholesale — reaching for the
destructive default in exactly the case where the file naming what to spare was the one
that failed to load. Uninstalling now stops and says so, which means it can fail where
it previously appeared to succeed. Leaving a port installed is recoverable; deleting a
save file is not.

## v0.2.0-alpha — 2026-09-07

The largest release so far. PortForge narrowed from a do-everything app into one program
in a suite of MediaItem tools, gained shared storage locations, a rebuilt interface, and
support for building ports across platforms and versions.

**Upgrading is automatic.** Item type folders are renamed and an existing library folder is
carried into the new shared storage list on first launch. Nothing needs to be moved by hand,
and no configuration is lost.

### Added

**Storage locations.** The single library folder is now a list of storage locations shared
with every other program in the suite, stored alongside the PortForge config rather than
inside it. Locations can be added, renamed, reordered and removed, each showing its free
space and the share PortForge uses. The first location in the list is where PortForge writes;
order is priority, and an unplugged drive is reported as unavailable rather than quietly
skipped, so a library never relocates itself behind your back.

**A ROMs page.** Every ROM the ports in your catalog can use, grouped by port. Each
requirement expands to list all of its accepted dumps with filename, size, format and
checksums, and marks the ones you already have. Files are matched by checksum, so a rename
or a different extension makes no difference.

**Cross-platform and multi-version builds.** Ports can declare several versions and several
target platforms, with a picker for each on the game page. Architecture is part of the
platform name (`Mac-arm64`, `Windows-x64`), so a port can target one architecture without
claiming the other. A port's recommended version is honoured where it differs from its
newest.

**Build output.** The game page shows live build output while a port installs and keeps it
on screen after a failure, which is when it is actually read. A banner tracks an install
from anywhere in the app, with progress and a stop button. Full output is still written to
`install.log` in the game's data folder.

**Install spec validation.** A spec that uses a variable it never declares is now rejected
before anything is downloaded or compiled, naming the variable. Previously such a spec built
for as long as it took to reach the offending step and then failed with whatever the build
tool made of the literal text.

**Server mode.** `portforge -server` serves the interface over HTTP and prints a URL instead
of opening a window, for systems whose native web view renders it poorly. `-addr` chooses
the address. This is a workaround rather than a supported mode, and choosing folders still
requires the desktop window.

**New ports in the catalog:** Gen1Recomp and re:Blue, along with Game Boy, Game Boy Color
and PlayStation ROM support.

### Changed

**Rebuilt interface.** A text sidebar replaces the icon rail. Cover art is no longer written
over: status, label and version moved below the title into their own row. Artwork is rounded
less than the surrounding chrome, tiles share a uniform shape so titles line up across a row,
and non-standard art letterboxes rather than being cropped. Settings was rebuilt on the
shared palette with real storage figures and catalog information. New typefaces throughout
(Outfit and IBM Plex Mono).

**Artwork is resized before it reaches the interface**, with a thumbnail cache. Catalog
artwork is stored at print resolution because it is shared across the suite; handing it
straight to the web view was making the whole app stutter.

**ROM requirements are a list of requirements, each satisfied by any one of its options.**
A three-disc game asks for each disc and accepts any region for each, rather than
enumerating every valid combination. A requirement marked required is one the port cannot be
*played* without — increasingly ports build without ROMs and extract their assets on first
run — so a port can install and launch with requirements outstanding.

**Item types were renamed** to name the platform, medium and form: `VideoGameFanPort` for
ports, and `N64CartRom`, `NESCartRom`, `GBCartRom`, `GBCCartRom`, `PS1DiscImage`,
`Xbox360DiscImage` for ROMs. Existing folders are renamed on first launch.

**Install specs are now `.forge.json`** and may carry file-level defaults shared by every
build in the file. The build engine is [Forge](https://github.com/zamiba/forge), now a
published dependency rather than a local one, so PortForge builds from a fresh clone.

**The catalog shrank from 1.9 GB to 27 MB** by keeping only the ROMs that a port in the
catalog actually references.

### Removed

- **Disc dumping** — optical drive detection, redumper integration and the dumper screen.
- **Emulator launching** — DuckStation integration and ROM launching. Launching installed
  ports is unaffected.
- **The standalone ROM browser**, replaced by the ROMs page described above.
- **PortForge's private settings file.** Its only setting was the library folder, which now
  comes from the shared storage list; an existing one is consumed during the upgrade.

### Fixed

- Artwork failed to load after the item type rename.
- A banner at the top of the window pushed the bottom of the page out of view.
- The Now Playing screen did not cover the sidebar, leaving navigation lit and clickable
  underneath it.
- The build output header showed a bare number that was really the display cap, so it always
  read `500`; it now says how many lines are shown and that older ones were trimmed.
- ROM scanning only looked at one item type, so ROMs filed under any other platform were
  never found.
- Super Mario 64 Render96 could not be installed: its spec used an undeclared variable, and
  separately declared no version.
- Settings added a duplicate progress listener every time it was opened.
- Removed the hover animation on library covers, which was the most visible stutter on Linux.

---

Earlier releases are listed at
<https://github.com/zamiba/portforge-app/releases>.
