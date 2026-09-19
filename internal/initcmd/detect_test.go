package initcmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectUnityProject(t *testing.T) {
	good := t.TempDir()
	os.Mkdir(filepath.Join(good, "Assets"), 0o755)
	os.Mkdir(filepath.Join(good, "ProjectSettings"), 0o755)
	if !DetectUnityProject(good) {
		t.Fatal("expected true for dir with Assets/ and ProjectSettings/")
	}

	bad := t.TempDir()
	os.Mkdir(filepath.Join(bad, "Assets"), 0o755) // missing ProjectSettings
	if DetectUnityProject(bad) {
		t.Fatal("expected false when ProjectSettings/ is missing")
	}
}
