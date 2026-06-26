package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func getDataHome() string {
	expandTilde := func(path string) string {
		if !strings.HasPrefix(path, "~") {
			return path
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}

	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return expandTilde(dir)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd":
		return filepath.Join(home, ".local", "share")

	case "darwin":
		return filepath.Join(home, "Library", "Application Support")

	case "windows":
		if dir := os.Getenv("APPDATA"); dir != "" {
			return dir
		}
		return filepath.Join(home, "AppData", "Roaming")

	default:
		return filepath.Join(home, ".local", "share")
	}
}
