# Changelog

All notable changes to PortForge are recorded here.

## v0.3.4-alpha — Unreleased

### Added

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
created, given a picture and deleted; Settings summarises the active one and opens the
same modal. A switch takes effect at once — every installed port's save links are
re-pointed there and then — and is refused while a game runs. Deleting is a two-step act
with a warning worded for the whole suite, since the folder holds more than game saves;
the active profile cannot be deleted. A picture is cropped square and scaled by the
profiles module and kept inside the profile folder, so the other programs show the same
one. A spec can
now put `${profilePath}` in a `defineExecutable` step's `args`, next to the port's own
flag — `"args": ["--save-dir", "${profilePath}"]` — and PortForge resolves it to the
active profile's folder for that port every time the game starts, creating it on first
use. A port that keeps its saves beside itself — the paths its spec lists in
`userDataPaths` — has those paths linked into the profile instead: the files move there,
the port's path becomes a link to them, and the game writes where it always did. Links are
made after an install, before each launch (so switching profiles takes effect) and after
each session. Data is never merged; a path with data on both sides is left alone and
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

**The dark/light toggle has moved from the sidebar to each page's top bar.** The
sidebar's bottom slot now belongs to the profile row.

**Release files are named after their version.** A download is
`portforge-0.3.4-alpha-linux-amd64` rather than `portforge-linux-amd64`, so two builds
kept side by side can be told apart, and the Windows and macOS files follow the same
pattern (`portforge-<version>-windows-amd64.exe`, `portforge-<version>-macos-universal.zip`).

### Fixed

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
