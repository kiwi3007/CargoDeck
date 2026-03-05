package api

import "github.com/kiwi3007/playerr/internal/launcher"

// shortcutEntry is an alias so games.go doesn't need to change.
type shortcutEntry = launcher.ShortcutEntry

func addSteamShortcut(entry shortcutEntry) (uint32, error) {
	return launcher.AddSteamShortcut(entry)
}

func findSteamUserConfigDir() string {
	return launcher.FindSteamUserConfigDir()
}
