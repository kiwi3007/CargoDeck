package launcher

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// RunnerType identifies how a wine/proton runner should be invoked.
type RunnerType string

const (
	RunnerUMU    RunnerType = "umu"    // umu-run (standalone Proton, no Steam)
	RunnerProton RunnerType = "proton" // proton binary (GE-Proton/Lutris/Heroic)
	RunnerWine   RunnerType = "wine"   // system wine
	RunnerNative RunnerType = "native" // Windows: run exe directly
)

// Runner holds a discovered wine/proton runner.
type Runner struct {
	Type       RunnerType
	BinPath    string // path to umu-run, proton binary, or wine binary
	ProtonPath string // proton installation dir (for UMU); empty otherwise
}

// FindRunner returns the best available runner on this system.
// On Windows, always returns RunnerNative.
// On Linux/macOS: UMU > Lutris Proton > Heroic Proton > GE-Proton > Wine > nil.
func FindRunner() *Runner {
	if runtime.GOOS == "windows" {
		return &Runner{Type: RunnerNative}
	}

	home, _ := os.UserHomeDir()

	// 1. UMU-launcher (umu-run in PATH)
	if umuBin, err := exec.LookPath("umu-run"); err == nil {
		protonPath := findBestProtonDir(home)
		log.Printf("[Runner] Using UMU-launcher: %s (proton=%s)", umuBin, protonPath)
		return &Runner{Type: RunnerUMU, BinPath: umuBin, ProtonPath: protonPath}
	}

	// 2. Lutris Proton
	if p := ProtonInDir(filepath.Join(home, ".local", "share", "lutris", "runners", "proton")); p != "" {
		log.Printf("[Runner] Using Lutris Proton: %s", p)
		return &Runner{Type: RunnerProton, BinPath: p}
	}

	// 3. Heroic Proton
	if p := ProtonInDir(filepath.Join(home, ".config", "heroic", "tools", "proton")); p != "" {
		log.Printf("[Runner] Using Heroic Proton: %s", p)
		return &Runner{Type: RunnerProton, BinPath: p}
	}

	// 4. GE-Proton / official Proton in Steam compatibilitytools.d
	steamRoot := FindSteamRoot()
	if steamRoot != "" {
		if p := ProtonInDir(filepath.Join(steamRoot, "compatibilitytools.d")); p != "" {
			log.Printf("[Runner] Using GE-Proton: %s", p)
			return &Runner{Type: RunnerProton, BinPath: p}
		}
		if p := ProtonInDir(filepath.Join(steamRoot, "steamapps", "common")); p != "" {
			log.Printf("[Runner] Using Steam Proton: %s", p)
			return &Runner{Type: RunnerProton, BinPath: p}
		}
	}

	// 5. System Wine
	if wineBin, err := exec.LookPath("wine"); err == nil {
		log.Printf("[Runner] Using system Wine: %s", wineBin)
		return &Runner{Type: RunnerWine, BinPath: wineBin}
	}

	log.Printf("[Runner] No runner found")
	return nil
}

// RunWith builds an exec.Cmd to run exePath via this runner.
// wineprefix is used as WINEPREFIX / STEAM_COMPAT_DATA_PATH.
// extraArgs are passed to the exe.
func (r *Runner) RunWith(exePath, wineprefix string, extraArgs ...string) *exec.Cmd {
	switch r.Type {
	case RunnerUMU:
		cmd := exec.Command(r.BinPath, append([]string{exePath}, extraArgs...)...)
		cmd.Dir = filepath.Dir(exePath)
		cmd.Env = append(os.Environ(),
			"WINEPREFIX="+wineprefix,
			"PROTONPATH="+r.ProtonPath,
			"GAMEID=0",
		)
		return cmd

	case RunnerProton:
		args := append([]string{"run", exePath}, extraArgs...)
		cmd := exec.Command(r.BinPath, args...)
		cmd.Dir = filepath.Dir(exePath)
		// GE-Proton needs the real Steam root to find the Linux Runtime container.
		steamRoot := FindSteamRoot()
		if steamRoot == "" {
			steamRoot = fakeSteamRootDir()
			_ = os.MkdirAll(steamRoot, 0755)
		}
		_ = os.MkdirAll(wineprefix, 0755)
		cmd.Env = append(os.Environ(),
			"STEAM_COMPAT_DATA_PATH="+wineprefix,
			"STEAM_COMPAT_CLIENT_INSTALL_PATH="+steamRoot,
		)
		return cmd

	case RunnerWine:
		cmd := exec.Command(r.BinPath, append([]string{exePath}, extraArgs...)...)
		cmd.Dir = filepath.Dir(exePath)
		cmd.Env = append(os.Environ(),
			"WINEPREFIX="+wineprefix,
		)
		return cmd

	case RunnerNative:
		cmd := exec.Command(exePath, extraArgs...)
		cmd.Dir = filepath.Dir(exePath)
		return cmd
	}

	// Fallback — should never happen
	return exec.Command(exePath, extraArgs...)
}

// fakeSteamRootDir returns a stable path to a fake Steam root directory.
// Used as STEAM_COMPAT_CLIENT_INSTALL_PATH to satisfy Proton without pointing at real Steam.
func fakeSteamRootDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "playerr-agent", "fake-steam-root")
}

// findBestProtonDir finds the best Proton installation dir for UMU.
// Returns the dir (not the proton binary) so UMU can locate the runtime.
func findBestProtonDir(home string) string {
	candidates := []string{
		filepath.Join(home, ".local", "share", "Steam", "compatibilitytools.d"),
		filepath.Join(home, ".steam", "steam", "compatibilitytools.d"),
		filepath.Join(home, ".local", "share", "lutris", "runners", "proton"),
		filepath.Join(home, ".config", "heroic", "tools", "proton"),
	}
	for _, dir := range candidates {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var best string
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			nameLower := strings.ToLower(e.Name())
			if strings.HasPrefix(nameLower, "proton") || strings.Contains(nameLower, "ge-proton") {
				candidate := filepath.Join(dir, e.Name())
				if best == "" || e.Name() > filepath.Base(best) {
					best = candidate
				}
			}
		}
		if best != "" {
			return best
		}
	}
	return ""
}

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
	best := candidates[0]
	for _, c := range candidates[1:] {
		if filepath.Dir(c) > filepath.Dir(best) {
			best = c
		}
	}
	return best
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
