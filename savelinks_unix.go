//go:build !windows

package main

import "os"

// makeLink points link at target. Symlinks carry directories and files alike,
// and a link to a file the game has not created yet is fine: writing through
// it creates the target.
func makeLink(target, link string, _ bool) error {
	return os.Symlink(target, link)
}

// readLink reports where path points, if it is a link.
func readLink(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return "", false
	}
	target, err := os.Readlink(path)
	if err != nil {
		return "", false
	}
	return target, true
}
