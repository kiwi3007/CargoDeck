package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProtonToolName derives the Steam compat tool name from a runner.
// Returns "" if no compat tool registration is needed (Wine, Native).
func ProtonToolName(runner *Runner) string {
	switch runner.Type {
	case RunnerProton:
		// runner.BinPath is e.g. /path/to/GE-Proton9-1/proton
		// filepath.Dir → /path/to/GE-Proton9-1, filepath.Base → "GE-Proton9-1"
		return filepath.Base(filepath.Dir(runner.BinPath))
	case RunnerUMU:
		if runner.ProtonPath != "" {
			return filepath.Base(runner.ProtonPath)
		}
		return "GE-Proton"
	default:
		return ""
	}
}

// SetCompatTool writes a CompatToolMapping entry in localconfig.vdf so Steam
// uses the correct Proton version when launching the non-Steam game shortcut.
// The operation is idempotent: if the appID entry already exists, it returns nil.
func SetCompatTool(cfgDir string, appID uint32, toolName string) error {
	vdfPath := filepath.Join(cfgDir, "localconfig.vdf")
	appIDStr := fmt.Sprintf("%d", appID)

	data, err := os.ReadFile(vdfPath)
	if os.IsNotExist(err) {
		return os.WriteFile(vdfPath, []byte(minimalLocalConfig(appIDStr, toolName)), 0644)
	}
	if err != nil {
		return fmt.Errorf("read localconfig.vdf: %w", err)
	}

	content := string(data)

	// Idempotent: skip if appID already has an entry anywhere in the file.
	if strings.Contains(content, `"`+appIDStr+`"`) {
		return nil
	}

	// Try to insert into existing CompatToolMapping section.
	mappingIdx := strings.Index(content, `"CompatToolMapping"`)
	if mappingIdx != -1 {
		afterKey := content[mappingIdx+len(`"CompatToolMapping"`):]
		braceIdx := strings.Index(afterKey, "{")
		if braceIdx == -1 {
			return fmt.Errorf("malformed localconfig.vdf: no { after CompatToolMapping")
		}
		insertAt := mappingIdx + len(`"CompatToolMapping"`) + braceIdx + 1
		content = content[:insertAt] + "\n" + compatEntry(appIDStr, toolName) + content[insertAt:]
	} else {
		// Insert an entire CompatToolMapping section inside the Steam section.
		steamIdx := strings.Index(content, `"Steam"`)
		if steamIdx == -1 {
			return fmt.Errorf("cannot find Steam section in localconfig.vdf")
		}
		afterSteam := content[steamIdx+len(`"Steam"`):]
		braceIdx := strings.Index(afterSteam, "{")
		if braceIdx == -1 {
			return fmt.Errorf("malformed localconfig.vdf: no { after Steam")
		}
		insertAt := steamIdx + len(`"Steam"`) + braceIdx + 1
		content = content[:insertAt] + "\n" + compatSection(appIDStr, toolName) + content[insertAt:]
	}

	return os.WriteFile(vdfPath, []byte(content), 0644)
}

// compatEntry returns a single appID block to insert inside an existing CompatToolMapping section.
func compatEntry(appIDStr, toolName string) string {
	return fmt.Sprintf(
		"\t\t\t\t%q\n"+
			"\t\t\t\t{\n"+
			"\t\t\t\t\t\"name\"\t\t%q\n"+
			"\t\t\t\t\t\"config\"\t\t\"\"\n"+
			"\t\t\t\t\t\"Priority\"\t\t\"250\"\n"+
			"\t\t\t\t}\n",
		appIDStr, toolName,
	)
}

// compatSection returns a full CompatToolMapping section to insert inside the Steam section.
func compatSection(appIDStr, toolName string) string {
	return fmt.Sprintf(
		"\t\t\t\"CompatToolMapping\"\n"+
			"\t\t\t{\n"+
			"\t\t\t\t%q\n"+
			"\t\t\t\t{\n"+
			"\t\t\t\t\t\"name\"\t\t%q\n"+
			"\t\t\t\t\t\"config\"\t\t\"\"\n"+
			"\t\t\t\t\t\"Priority\"\t\t\"250\"\n"+
			"\t\t\t\t}\n"+
			"\t\t\t}\n",
		appIDStr, toolName,
	)
}

// minimalLocalConfig builds a minimal localconfig.vdf for a fresh Steam install.
func minimalLocalConfig(appIDStr, toolName string) string {
	return fmt.Sprintf(
		"\"UserLocalConfigStore\"\n"+
			"{\n"+
			"\t\"Software\"\n"+
			"\t{\n"+
			"\t\t\"Valve\"\n"+
			"\t\t{\n"+
			"\t\t\t\"Steam\"\n"+
			"\t\t\t{\n"+
			"\t\t\t\t\"CompatToolMapping\"\n"+
			"\t\t\t\t{\n"+
			"\t\t\t\t\t%q\n"+
			"\t\t\t\t\t{\n"+
			"\t\t\t\t\t\t\"name\"\t\t%q\n"+
			"\t\t\t\t\t\t\"config\"\t\t\"\"\n"+
			"\t\t\t\t\t\t\"Priority\"\t\t\"250\"\n"+
			"\t\t\t\t\t}\n"+
			"\t\t\t\t}\n"+
			"\t\t\t}\n"+
			"\t\t}\n"+
			"\t}\n"+
			"}\n",
		appIDStr, toolName,
	)
}
