package initcmd

import (
	"os"
	"path/filepath"
)

// DetectUnityProject reports whether dir is a Unity project root: it must
// contain BOTH Assets/ and ProjectSettings/ directories (spec §4.1). This gate
// guarantees init never touches a non-Unity directory.
func DetectUnityProject(dir string) bool {
	return isDir(filepath.Join(dir, "Assets")) && isDir(filepath.Join(dir, "ProjectSettings"))
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
