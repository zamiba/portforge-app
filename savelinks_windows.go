package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// makeLink points link at target. Directories become junctions, which need no
// privilege; a symlink would need Developer Mode or administrator rights,
// which a player should not have to turn on to save a game. Single files have
// no unprivileged equivalent — a hard link cannot cross volumes, and the
// profile is on the system drive while the game may not be — so they are
// reported as unsupported and stay with the game.
func makeLink(target, link string, isDir bool) error {
	if !isDir {
		return fmt.Errorf("%w: single files stay beside the game on Windows", errLinkUnsupported)
	}
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mklink /J: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// readLink reports where path points, if it is a junction or a symlink. Go
// reports a junction as an irregular file rather than a symlink, but Readlink
// still resolves it; the \??\ prefix it comes back with is stripped so the
// target compares equal to the path the junction was made with.
func readLink(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0 {
		return "", false
	}
	target, err := os.Readlink(path)
	if err != nil {
		return "", false
	}
	target = strings.TrimPrefix(target, `\??\`)
	target = strings.TrimPrefix(target, `\\?\`)
	return target, true
}
