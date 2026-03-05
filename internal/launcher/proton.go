package launcher

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FindSteamRoot returns the Steam installation root directory.
func FindSteamRoot() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".steam", "steam"),
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	return ""
}

// FindProton searches for a Proton binary, preferring GE-Proton.
func FindProton() string {
	steamRoot := FindSteamRoot()
	if steamRoot == "" {
		return ""
	}

	// GE-Proton / custom tools first
	if p := ProtonInDir(filepath.Join(steamRoot, "compatibilitytools.d")); p != "" {
		return p
	}
	// Official Proton in steamapps/common
	if p := ProtonInDir(filepath.Join(steamRoot, "steamapps", "common")); p != "" {
		return p
	}
	return ""
}

// ProtonInDir finds the newest Proton binary in a directory.
func ProtonInDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var candidates []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		nameLower := strings.ToLower(e.Name())
		if strings.HasPrefix(nameLower, "proton") || strings.Contains(nameLower, "ge-proton") {
			bin := filepath.Join(dir, e.Name(), "proton")
			if fileExists(bin) {
				candidates = append(candidates, bin)
			}
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	// Highest lexicographic name = newest version
	best := candidates[0]
	for _, c := range candidates[1:] {
		if filepath.Dir(c) > filepath.Dir(best) {
			best = c
		}
	}
	return best
}

// TryProton builds a Proton exec.Cmd for the given exe.
// compatDataSuffix is appended to the compatdata dir (e.g. "playerr" or "playerr_42").
// Returns nil if no Proton installation is found.
func TryProton(exePath, compatDataSuffix string) *exec.Cmd {
	protonBin := FindProton()
	if protonBin == "" {
		return nil
	}

	home, _ := os.UserHomeDir()
	steamRoot := FindSteamRoot()
	compatData := filepath.Join(home, ".steam", "steam", "steamapps", "compatdata", compatDataSuffix)
	_ = os.MkdirAll(compatData, 0755)

	cmd := exec.Command(protonBin, "run", exePath)
	cmd.Dir = filepath.Dir(exePath)
	cmd.Env = append(os.Environ(),
		"STEAM_COMPAT_DATA_PATH="+compatData,
		"STEAM_COMPAT_CLIENT_INSTALL_PATH="+steamRoot,
	)
	log.Printf("[Launcher] Using Proton: %s", protonBin)
	return cmd
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
