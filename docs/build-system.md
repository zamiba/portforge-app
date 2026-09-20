# PortForge Build System

Games that need more than a download to become playable carry a `.forge.json` file alongside their `.mediaitem.json`. PortForge executes it with **[Forge](https://github.com/zamiba/forge)**, a standalone build engine it embeds as a library.

**The spec format — steps, args, interpolation, conditions, and the security model — is documented in [Forge's README](https://github.com/zamiba/forge#readme).** This page covers only what is specific to PortForge.

**A game is buildable if it has a spec file at all** — not if one targets this machine. PortForge builds for platforms it does not run on, so the host platform is the default selection in the platform picker rather than a filter on what is offered.

**Spec files are read as `.forge.json`, falling back to `.install.json`** for catalog entries that have not been converted. `metadata.SpecFileNames` holds the order.

**Version and platform selection is PortForge's decision, not the engine's.** `GetSpecVersions` returns every declared version newest first, each with the platforms it builds for and one marked `default`; the game page picks from those and passes the result to `InstallVersion`. An unspecified version resolves to the file's `defaultVersion`, then to the newest declared — never to "whichever build sorts first", which is ambiguous once a build can span versions.

---

## Where specs live and where they run

```
mediaitems/VideoGameVersion/<ItemTitle>/     ← catalog (read-only)
├── .mediaitem.json
├── .forge.json                              ← the spec
└── .artwork/

<userDataFolder>/VideoGameVersion/<ItemTitle>/   ← the run's root directory
├── install.log                              ← transcript of the last run
├── .state/meta.json                         ← written after a successful install
└── install/                                 ← conventional destination (not enforced)
```

Steps start in the user data folder's copy of the item, **not** in the catalog folder. All step paths resolve inside that directory; a path that escapes it — absolute or via `..` — fails the run. There is no automatically created build directory: the spec manages its own layout with `createDir`, `move`, and `deletePath`.

---

## The `rom` provider

PortForge registers one provider, `rom`, which resolves a `copy` step against the user's ROM library:

```json
{ "step": "copy", "from": "rom", "src": "Super Mario 64 (USA)", "dest": ".build/baserom.us.z64" }
```

`src` is matched against the `title` of one of the version's `romDependencies` in `.mediaitem.json`. PortForge looks up every known format of that ROM by MD5 and copies whichever one the user actually has.

Omit `src` to accept **any** of the version's ROM dependencies that is present locally — useful when a port accepts several regional dumps interchangeably:

```json
{ "step": "copy", "from": "rom", "dest": ".build/rom.z64" }
```

Combine `src` with an arg to let the user pick the region:

```json
"args": {
  "region": {
    "type": "choice",
    "label": "ROM region",
    "options": [
      { "value": "Super Mario 64 (USA)", "label": "US (NTSC)" },
      { "value": "Super Mario 64 (Europe) (En,Fr,De)", "label": "European (PAL)" }
    ]
  }
},
"steps": [
  { "step": "copy", "from": "rom", "src": "${args.region}", "dest": ".build/baserom.z64" }
]
```

A step can also be handed the ROM's path instead of a copy: `${romPath}` is the path of any present dependency and `${romPath.<name>}` that of the requirement called `<name>` — the same lookup a `copy` makes, without the copy. This is for a port whose own installer reads the disc:

```json
{ "step": "run", "cmd": "install/nectar-launcher", "args": ["--rom", "${romPath}", "--install-dir", "install", "--extract-only"] }
```

The same reference in a `defineExecutable` step's `args` is not resolved at install time. It is recorded as written and resolved every time the game is launched, against the library as it is then — so the disc is found after a storage unit moves, and a ROM added after installing works without reinstalling. A port that takes its disc on the command line boots straight into the game:

```json
{ "step": "defineExecutable", "executable": "install/melee", "title": "Play", "args": ["${romPath}"] }
```

Launching such a port without the ROM present is refused with a message naming what is missing.

A `copy from:"rom"` step fails the install when no matching ROM is in the library.

---

## The `profile` provider

`${profilePath}` in a `defineExecutable` step's `args` is the folder the active profile keeps for this port — saves, configuration, whatever the port writes — resolved every time the game is launched, like `${romPath}`. PortForge supplies only the path; the port's own flag says what it is for, which is why ports with different flags need no different handling:

```json
{ "step": "defineExecutable", "executable": "install/game", "title": "Play", "args": ["--save-dir", "${profilePath}"] }
```

The folder is created the first time the game starts, so a port need not tolerate its absence. It mirrors the port's own folder on the storage unit: `<profiles>/<profile>/MediaItems/VideoGameFanPort/<port item title>/`. `${profilePath}` takes no qualifier; `${profilePath.x}` is a launch error.

This is the arrangement to prefer for a port that accepts one. A port whose save location cannot be redirected keeps `userDataPaths` (below), which PortForge will link into the same profile folder in a later release.

---

## Dependencies and the `run` step

A spec's `dependencies` array serves two purposes: PortForge verifies each command is on `PATH` before starting and reports the missing ones, and the same array is the allowlist for `run`. A spec may only invoke commands it has declared, so the array is a complete inventory of what an install can execute.

```json
"dependencies": ["make", "gcc", "python3", "unzip"],
"steps": [
  { "step": "run", "cmd": "make", "args": ["VERSION=${args.region}", "-j4"] }
]
```

`msys2` is a special dependency: on Windows it requires an MSYS2 installation and prepends its `mingw64`, `usr/local/bin`, and `usr/bin` directories to `PATH` for every subprocess in the run.

Uninstall sequences skip the pre-flight check — the tools that built a game may be long gone by the time it is removed — but the allowlist still applies.

---

## Install results

A successful install must declare at least one `defineExecutable` step; PortForge fails the install otherwise, since there would be nothing to launch. The first becomes the default Play button and the rest appear in a dropdown. Paths are recorded relative to the item's data folder, and `args`, if given, are what the executable is started with.

The declared executables, the spec's `version`, and a timestamp are written to `.state/meta.json`, alongside play-time tracking that accumulates across sessions.

On failure PortForge preserves the folder for debugging and offers a **Delete build folder** button, which removes the spec's `buildPaths` if it declares any and otherwise falls back to `.build/` and `build/`.

## User data

A port that writes save games or a configuration file inside its own `install/` folder
would lose them to the usual `deletePath install` teardown. Forge's file-level
`userDataPaths` names what must survive, and PortForge honours it in three places:

- **Uninstalling.** Preserved paths stay; everything else in `install/` goes. This holds
  even for a spec that declares no `uninstallSteps` at all — PortForge supplies the
  default teardown as a step so the same rule applies to it.
- **Reinstalling or updating.** A build runs over whatever the previous one left behind,
  so declared user data is moved out of the tree for the duration and moved back
  afterwards. **The user's copy wins** over a default the build ships at the same path:
  losing an edited config to an update is worse than carrying an outdated one forward.
- **A failed build.** Restoration happens on the way out either way, so a build that dies
  halfway does not take the player's saves with it.

Paths interpolate `$name` like any other, `$platform` and `$version` included, and are
reported by `forge check` if they reference something the spec never declares.

This covers ports that keep user data beside their own files, and it is also how those
ports reach the profile. Each declared path is **linked** into the active profile: the
real files live at the same relative path under the profile's folder for the port
(`<profile>/MediaItems/VideoGameFanPort/<port>/install/saves`, say) and the port's own
path is a symlink — a junction on Windows — pointing at them. The game writes where it
always did. PortForge makes the links after an install (moving in whatever an older
install left), before each launch (so a profile switch takes effect) and after each
session (so data a game created for the first time is in the profile before it is sent
on). Nothing is ever merged: a path with data on both sides is left as it is and reported
on the game page, and the game runs with its saves where they were. Single files cannot
be linked without privileges on Windows and stay beside the game there.

A port that accepts a flag pointing its save directory elsewhere is the simpler
arrangement — give it `${profilePath}` (above) and declare nothing here. The two can
coexist in one spec.

---

---

## Authoring specs with the CLI

Forge ships a CLI that runs the same engine PortForge embeds, which is the fastest way to iterate on a spec without launching the app:

```bash
forge check --spec ".forge.json"                    # validate and check dependencies
forge args  --spec ".forge.json"                    # see the arg prompts PortForge will show
forge run   --spec ".forge.json" --dir ./scratch \
            --platform Windows --version "1.0.2" \
            --arg region=us --provider rom=~/roms   # run it, standing in for PortForge's ROM library
```

`--provider rom=DIR` substitutes a plain directory for PortForge's checksum-matched ROM library, so `src` is treated as a filename beneath `DIR`.
