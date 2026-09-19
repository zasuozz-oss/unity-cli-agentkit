// Package initcmd installs project-scoped pointers to the kit's skills.
package initcmd

import (
	"os"
	"path/filepath"
)

// KitHome resolves the central kit root: the UNITY_CLI_AGENTKIT_HOME env var if
// set, otherwise the directory above the running binary (<dir>/../ where the
// binary lives in <kit>/bin/utk). The skills/ dir lives at <KitHome>/skills.
func KitHome() (string, error) {
	if env := os.Getenv("UNITY_CLI_AGENTKIT_HOME"); env != "" {
		return env, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(filepath.Dir(exe)), nil // <kit>/bin/utk → <kit>
}

// SkillsDir returns <KitHome>/skills.
func SkillsDir() (string, error) {
	home, err := KitHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "skills"), nil
}
