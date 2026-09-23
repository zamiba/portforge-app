# PortForge

A desktop application for managing and launching game ports. PortForge handles downloading, building, and installing ports — including those that require compilation from source — and keeps your ROM library organized so games that depend on original ROM files can find them automatically.

Built with [Wails v2](https://wails.io) (Go backend, Vue 3 frontend).

<p align="center" style="text-align: center">
  <img src="./assets/images/portforge_screenshot.png">
</p>

---

## ALPHA NOTICE!

This program is still at a very early stage. There are going to be bugs, I honestly have no idea to what degree it will work on Windows or Mac, and while I will try to avoid it, future updates may require manual moving of user data files.

---

## Support this project

This project, along with everything else that I make here, is and will remain fully open source, with no proprietary components and no paywalled features.

I freelance full time, so every bit of support is time that I can spend on this and my other projects.

- [Patreon](https://patreon.com/zamiba)

---

## Get started

### Download a build

Builds for each release are on the [releases page](https://github.com/zamiba/portforge-app/releases).

File names carry the version, shown here as `<version>`, so a `portforge-0.3.4-alpha-linux-amd64` on disk is still identifiable later.

| Platform | File |
| -------- | ---- |
| Windows 10+ | `portforge-<version>-windows-amd64.exe` |
| macOS 12+ | `portforge-<version>-macos-universal.zip` |
| Linux, webkit2gtk **4.0** | `portforge-<version>-linux-amd64` |
| Linux, webkit2gtk **4.1** | `portforge-<version>-linux-amd64-webkit4_1` |

**Which Linux build?** PortForge draws its interface with your system's WebKitGTK, and distributions ship one of two incompatible series of it. To see which you have:

```bash
ldconfig -p | grep libwebkit2gtk
```

`libwebkit2gtk-4.0.so.37` means the plain build, `libwebkit2gtk-4.1.so.0` the `-webkit4_1` one. Debian and Ubuntu are usually on 4.0; Arch, Fedora 37+ and most rolling distributions are on 4.1. If neither is listed, install it first — the package is `webkit2gtk-4.1` on Arch, `webkit2gtk4.1` on Fedora, and `libwebkit2gtk-4.1-0` on Debian and Ubuntu.

The Linux downloads are plain binaries, so mark one executable and run it:

```bash
chmod +x portforge-*-linux-amd64
./portforge-*-linux-amd64
```

Builds are unsigned. **macOS** will refuse to open the app the first time: right-click it and choose **Open**, or run `xattr -d com.apple.quarantine portforge.app` first. **Windows** shows a SmartScreen warning — choose **More info → Run anyway**.

### Or build from source

You need [Go 1.26+](https://go.dev), [Node.js 18+](https://nodejs.org), and the [Wails CLI v2](https://wails.io/docs/gettingstarted/installation), plus the GTK and WebKitGTK development headers for your distribution.

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
git clone --recurse-submodules https://github.com/zamiba/portforge-app
cd portforge-app
wails build
```

The binary is written to `build/bin/`. On a system with webkit2gtk 4.1, build with `wails build -tags webkit2_41` instead.

The catalog is a submodule, so `--recurse-submodules` matters — without it the app starts with an empty library.

### Development

```bash
wails dev
```

Starts a Vite dev server with hot reload for the frontend. The app is also accessible in a browser at `http://localhost:34115`.

In dev mode the app reads the MediaItems catalog from the `mediaitems/` folder at the project root instead of the OS config directory.

### If the window renders slowly

WebKitGTK is sluggish on some Linux systems in a way PortForge cannot do anything about. As a workaround, `portforge -server` serves the interface over HTTP and prints a URL to open in a normal browser instead of opening a window; `-addr` chooses the address it listens on. Picking folders still needs the desktop window, so use it after first-run setup.

### Missing library errors

PortForge does not pin any version of libjxl or any other image codec — those come from your system's WebKitGTK. An error like `libjxl.so.0.11: cannot open shared object file` means your WebKitGTK and libjxl packages are out of step with one another, which a full system update normally resolves.

---

## On MediaItems

`MediaItem` is a filesystem-native open standard for cataloguing, archiving, and interacting with any form of media of my own invention. Documentation covering the standard will be released "soon™".

---

## Roadmap

- Make a Flatpak build for Linux
- Add support for per-game executable arguments
- Add support for editing per-game executable arguments from the PortForge UI
- Add support for managing different versions of games
- Add support for managing mods without reinstalling
- Add support for building games for mobile platforms like Android and Nintendo Switch
- Expand documentation to include how to dump ROMs
- Add support for getting older PC games to run
- Add support for dumping disc-based games with `redumper` (has been moved to it's own TBA project)
- Offer to report an install or build failure as a GitHub issue, on approval and with nothing identifying in it

---

## Features

- **Library view** — browse available game ports with cover art and banner images
- **One-click install** — download, extract, build, and launch from a single button
- **Build system** — supports ports that must be compiled from source (e.g. SM64 ports), with per-platform build specs, multi-variant installs, and user-configurable args
- **ROM management** — each game page lists the ROMs that game needs, with the filename, size and checksums of every accepted dump and whether you already have it; add missing files by drag-and-drop or from the page itself. PortForge matches by checksum rather than filename and indexes the library in SQLite for fast lookups
- **Install variants** — choose ROM region, texture pack, or other options before installing
- **Library updates** — when the MediaItems library is synced, installed games are compared against the new version; an **Update** button appears on the game page when the library has changed
- **Play time tracking** — session length is recorded and added to a time counter
- **Uninstall** — remove installed files via the UI, with optional custom uninstall steps per game
- **Cross-platform** — Windows, macOS, and Linux

---

## First-run setup

On first launch PortForge asks for one folder:

| Folder               | Purpose                                                                          |
| -------------------- | -------------------------------------------------------------------------------- |
| **User data folder** | Writable. PortForge stores your ROM files, installed games, and save state here. |

It then downloads the port catalog — a few tens of megabytes, mostly artwork — and builds
its index before showing the library. The catalog is managed by PortForge and stored in
the OS configuration directory.

Afterwards, PortForge checks for a newer catalog every time it starts and fetches one in
the background when there is; a notice appears once the library has been refreshed. The
check is a single small request and the download only happens when something changed.
**Check for a newer catalog whenever PortForge starts** on the **Settings** page turns
it off, in which case **Refresh catalog** on the same page is how you update by hand.
The choice is kept in `preferences.json` in the configuration directory.

---

## MediaItems library

The library is a folder tree of _MediaItems_ — JSON files that describe games, ROMs, and platforms. PortForge reads this folder in the OS configuration directory and syncs it to the index on demand; it is never directly user-editable.

```
mediaitems/
├── VideoGameFanPort/
│   └── Super Mario 64 Render96 · 2020/
│       ├── .mediaitem.json     ← game metadata, download URL, ROM dependencies
│       ├── .forge.json        ← build spec (optional)
│       └── .artwork/
│           ├── Cover · en · 1.jpg
│           └── Banner · en · 1.jpg
└── N64CartRom/
    └── Super Mario 64 (USA) · N64/
        └── .mediaitem.json     ← ROM title, platform, expected checksums
```

ROM item types name the platform, the medium and the form: `N64CartRom`, `NESCartRom`, `GBCartRom`, `GBCCartRom`, `GameCubeDiscImage`, `PS1DiscImage`, `Xbox360DiscImage`.

A SQLite index (`library.db` in the config directory) caches the listing fields from these JSON files and the presence of user ROM files so lookups are fast without re-parsing files on every page open. The index is rebuilt automatically after each sync, and can be refreshed manually at any time via **Refresh index** on the **Settings** page.

---

## Profiles

Saves and settings go to a profile: a folder shared with the other MediaItem programs on
this machine, so what each records about a person sits together and the whole thing can
be copied or synced to another device however you like. PortForge does not sync or merge
profiles itself; when a play session ends it tells the shared profiles module that the
profile changed, and that module does whatever you have set up — nothing, by default. If
the profile folder is a git repository, each session ends in a commit (`portforge: game
ended: <port>`), pushed if `MediaItem/profile-sync.json` names a remote; the same file can
name `rclone-copy`, `rclone-sync` or `syncthing` backends. See the
[`go-mediaitems-profiles`](https://github.com/zamiba/go-mediaitems-profiles) README for
the file's shape. What came back is shown briefly in the app; failures are logged too.
First-run setup asks whose saves these are: name a profile of your own, or let PortForge
make one named `portforge`, which is an ordinary profile you can rename or replace later.
The last row of the sidebar says who is playing; clicking it (or *Manage profiles…* in
Settings) opens the profile modal, where you switch, rename, create and delete profiles
and give one a picture. A switch takes effect at once and is refused while a game is
running. Deleting removes the folder from this device for every program that used it —
the modal says so and asks twice — and the active profile cannot be deleted.

Profiles live at `MediaItem/profiles/<profile>/` in the configuration directory, beside
the storage-unit list, and inside one a port's data is at
`MediaItems/VideoGameFanPort/<port>/`. A port that takes a flag for its save location
receives that folder at launch (`${profilePath}` in its spec); one that keeps its saves
beside itself, or in a per-user folder such as `~/.local/share/<name>`, has those places
(`userDataPaths`) linked into the same folder, so the game writes where it always did
and the data lands in the profile. Both are described in
[`docs/build-system.md`](docs/build-system.md). Data is never merged: if a path holds
data on both sides, PortForge leaves both alone and says so on the game page.

## User library

When a game is installed or a ROM is imported, PortForge copies the relevant MediaItem folder from the library into the user data directory under `library/`. This snapshot is used to detect when the catalog has been updated since you last installed: if the catalog version of a MediaItem differs from your local copy after a sync, an **Update** button appears on the game's detail page.

---

## ROM library

ROMs are stored in the user data folder and identified by MD5 checksum rather than filename.

They appear in two places, showing the same thing:

- **On a game's page**, under **Required ROMs** — what this port needs, and whether you have it.
- **On the ROMs page**, grouped by port — every ROM the ports in your catalog can use, and how much of that you already hold.

A port declares one requirement per thing it needs, and any one of a requirement's accepted dumps satisfies it: a three-disc game asks for each disc and takes any region for each. Opening a requirement lists every accepted dump with its filename, size, extension and checksums, marking the ones already in your library.

This is not a general ROM browser. Only ROMs that some port in the catalog can use are listed, and a ROM you own that nothing references does not appear.

There are two ways to add a ROM:

- **Drag and drop** the files onto any PortForge window. Each file is matched against every known ROM in the catalog, and PortForge offers to copy or move it into place.
- **Add file…** on a game page or on the ROMs page, which opens a file picker.

Either way matching is by checksum, so files are recognised whatever they are named — which also means it does not matter which page you add them from.

---

## Build system

Games that need more than a download come with a `.forge.json` file alongside their `.mediaitem.json`. The spec defines dependencies, user-configurable args, and one or more builds, each an ordered list of install steps. The superseded `.install.json` name is still read for entries that have not been converted.

Specs are executed by **[Forge](https://github.com/zamiba/forge)** — a standalone, cross-platform build engine that PortForge embeds as a library. Forge's README documents the spec format in full; **[docs/build-system.md](docs/build-system.md)** covers the PortForge-specific parts, chiefly the `rom` provider that pulls ROMs out of your library mid-build.

### Quick example

```json
[
  {
    "targetPlatforms": ["Linux"],
    "dependencies": ["make", "gcc", "python3", "unzip"],
    "args": {
      "region": {
        "type": "choice",
        "label": "ROM region",
        "options": [
          { "value": "us", "label": "US (NTSC)" },
          { "value": "eu", "label": "European (PAL)" }
        ]
      }
    },
    "steps": [
      { "step": "createDir", "path": ".build" },
      { "step": "fetch", "url": "https://example.com/source.zip", "dest": ".build/source.zip" },
      { "step": "extract", "src": ".build/source.zip", "dest": ".build" },
      { "step": "copy", "from": "rom", "src": "Super Mario 64 (USA)", "dest": ".build/source/baserom.${region}.z64" },
      { "step": "run", "cmd": "make", "args": ["VERSION=${region}"] },
      { "step": "move", "src": ".build/source/build/${region}_pc", "dest": "install" },
      { "step": "deletePath", "path": ".build" },
      { "step": "defineExecutable", "executable": "install/sm64.${region}.f3dex2e", "title": "Play" }
    ]
  }
]
```

A spec may only run commands it declares in `dependencies`, and every path it touches must stay inside the game's own data folder.

---

## Project structure

PortForge is one program in a suite built around [MediaItems](#on-mediaitems). The build engine lives in its own repository so other tools can reuse it:

| Repository                                                             | Role                                                           |
| ---------------------------------------------------------------------- | -------------------------------------------------------------- |
| [portforge-app](https://github.com/zamiba/portforge-app)               | This app: library UI, ROM management, install and launch       |
| [forge](https://github.com/zamiba/forge)                               | The build engine that executes `.forge.json` specs, plus a CLI |
| [portforge-mediaitems](https://github.com/zamiba/portforge-mediaitems) | The MediaItems catalog of games, versions and ROMs             |

```
portforge/
├── app.go                  ← backend: catalog, launch, ROM management, storage
├── install.go              ← bridges the forge engine: ROM provider, event forwarding
├── main.go                 ← Wails entry point, -server flag, DragAndDrop configuration
├── server.go               ← -server mode: static assets, RPC, SSE
├── shim.go                 ← rebuilds the Wails bindings for a browser
├── events.go               ← one event path for both the window and -server
├── migrate.go              ← one-time upgrades (library path, renamed item folders)
├── storage.go              ← free space and the library's own footprint
├── thumbs.go               ← artwork resizing and the thumbnail cache
├── dev.go                  ← dev-mode overrides (catalog path, build tag: dev)
├── launch_unix.go          ← platform-specific launch logic
├── launch_windows.go
├── storageunits/           ← the storage location list, shared across the suite
├── models/
│   └── models.go           ← shared data types (VideoGameVersion, InstallState, etc.)
│                             install spec types live in the forge engine
├── metadata/
│   └── loader.go           ← reads the MediaItems library and user state files
├── store/
│   └── store.go            ← SQLite library index (media_items, rom_formats)
├── frontend/
│   └── src/
│       ├── App.vue                  ← shell, navigation, global install state
│       ├── components/
│       │   ├── Sidebar.vue
│       │   ├── GameLibrary.vue
│       │   ├── GameDetail.vue       ← game page: install, launch, required ROMs
│       │   ├── RomLibrary.vue       ← the ROMs page
│       │   ├── RomRequirements.vue  ← the requirement list both pages render
│       │   └── Settings.vue
│       ├── composables/             ← ROM requirement rules, theme
│       ├── lib/                     ← artwork URLs and display widths
│       └── styles/                  ← palette, fonts, shared primitives
├── docs/
│   └── build-system.md     ← PortForge-specific parts of the build system
├── CHANGELOG.md
└── wails.json
```

---

## Credits

- The developers and contributors of the various game ports
- [The Wails project](https://wails.io/) and its contributors
- [No-Intro](https://no-intro.org/) and [Redump](http://redump.org/) for cataloging original game media
- The contributors to [SteamGridDB](https://www.steamgriddb.com/) for game artwork
- The contributors to [EmuMovies](https://emumovies.com/) for game artwork
- The contributors to [LaunchBox Games Database](https://gamesdb.launchbox-app.com//) for game artwork
- Claude by Anthropic for coding assistance
- The entire Open Source ecosystem and the community behind it

---

## License

- The source code is released under the GPL-3.0 license
- All game specific artwork belongs to the respective rights holders
