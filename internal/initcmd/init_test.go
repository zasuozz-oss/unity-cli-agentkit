package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeProject(t *testing.T) (proj, kit string) {
	t.Helper()
	proj = t.TempDir()
	os.Mkdir(filepath.Join(proj, "Assets"), 0o755)
	os.Mkdir(filepath.Join(proj, "ProjectSettings"), 0o755)
	kit = t.TempDir()
	os.MkdirAll(filepath.Join(kit, "skills", "utk-cli-core"), 0o755)
	// SKILL.md is what marks a directory as a skill (names carry no meaning
	// since the kit also ships vendored unity-* skills).
	os.WriteFile(filepath.Join(kit, "skills", "utk-cli-core", "SKILL.md"), []byte("kit"), 0o644)
	return proj, kit
}

func TestRun_RejectsNonUnityDir(t *testing.T) {
	if err := Run(t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("expected refusal outside a Unity project")
	}
}

// The kit ships skills whose names say nothing about ownership (vendored
// unity-*). Only the ones this kit installed may be pruned or uninstalled: a
// same-shaped name the user put there themselves must survive both.
func TestRun_OwnsVendoredNamesViaManifestOnly(t *testing.T) {
	proj, kit := makeProject(t)
	skill := filepath.Join(kit, "skills", "unity-ugui-layout")
	os.MkdirAll(skill, 0o755)
	os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("vendored"), 0o644)
	// A user skill sharing the vendored naming, which the kit never installed.
	mine := filepath.Join(proj, ".claude", "skills", "unity-mine")
	os.MkdirAll(mine, 0o755)
	os.WriteFile(filepath.Join(mine, "SKILL.md"), []byte("mine"), 0o644)

	if err := Run(proj, kit); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(proj, ".claude", "skills", "unity-ugui-layout", "SKILL.md")); err != nil || string(b) != "vendored" {
		t.Fatalf("vendored skill should be installed, got %q err %v", b, err)
	}
	// Kit drops the vendored skill: the next init must prune the project's copy…
	os.RemoveAll(skill)
	if err := Run(proj, kit); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(proj, ".claude", "skills", "unity-ugui-layout")); !os.IsNotExist(err) {
		t.Fatalf("stale vendored skill must be pruned, err = %v", err)
	}
	// …and the user's own unity-* skill must survive prune and uninstall alike.
	if _, err := os.Stat(mine); err != nil {
		t.Fatalf("user skill must survive prune: %v", err)
	}
	if err := Uninstall(proj); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(mine, "SKILL.md")); err != nil || string(b) != "mine" {
		t.Fatalf("user skill must survive uninstall, got %q err %v", b, err)
	}
}

// A kit whose skills/ has no skill entries (or only broken symlinks) must fail
// loudly rather than report a successful install that placed nothing.
func TestRun_FailsWhenKitHasNoSkills(t *testing.T) {
	proj := t.TempDir()
	os.Mkdir(filepath.Join(proj, "Assets"), 0o755)
	os.Mkdir(filepath.Join(proj, "ProjectSettings"), 0o755)
	kit := t.TempDir()
	os.MkdirAll(filepath.Join(kit, "skills"), 0o755) // empty skills dir
	if err := Run(proj, kit); err == nil {
		t.Fatal("expected error when kit ships no skills")
	}
}

// skipIfNoSymlinkPrivilege skips a symlink-dependent test on Windows without
// Developer Mode / admin rights, where os.Symlink is denied outright; any
// other failure is a real one.
func skipIfNoSymlinkPrivilege(t *testing.T, err error) {
	t.Helper()
	if strings.Contains(err.Error(), "privilege") {
		t.Skip("symlinks require elevated privileges on this host:", err)
	}
	t.Fatal(err)
}

// When a project's skills dir is a symlink to the kit's own store, init must
// NOT write into the store (which would copy entries onto themselves). The
// store entries must stay intact (the real-world shared-store corruption).
func TestRun_DoesNotCorruptStoreWhenSkillsDirLinksToIt(t *testing.T) {
	proj, kit := makeProject(t)
	kitSkills := filepath.Join(kit, "skills")
	// Make the project's .claude/skills a symlink to the kit store, as the
	// corrupting setup did.
	claudeDir := filepath.Join(proj, ".claude")
	os.MkdirAll(claudeDir, 0o755)
	if err := os.Symlink(kitSkills, filepath.Join(claudeDir, "skills")); err != nil {
		skipIfNoSymlinkPrivilege(t, err)
	}
	if err := Run(proj, kit); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	// The store's kit skill must remain a real directory, not a self-symlink.
	storeSkill := filepath.Join(kitSkills, "utk-cli-core")
	info, err := os.Lstat(storeSkill)
	if err != nil {
		t.Fatalf("store skill missing: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("store skill was turned into a symlink (corrupted): %s", storeSkill)
	}
}

// A kit skill provided as a symlink to a real directory must be discovered and
// have its content copied (os.ReadDir does not follow links, so detection must
// Stat; the copy must follow the link to the real content, not copy a link).
func TestRun_DiscoversSymlinkedKitSkill(t *testing.T) {
	proj := t.TempDir()
	os.Mkdir(filepath.Join(proj, "Assets"), 0o755)
	os.Mkdir(filepath.Join(proj, "ProjectSettings"), 0o755)
	kit := t.TempDir()
	// Real skill content lives elsewhere; the kit exposes it via a symlink.
	realSkill := filepath.Join(t.TempDir(), "utk-cli-core")
	os.MkdirAll(realSkill, 0o755)
	os.WriteFile(filepath.Join(realSkill, "SKILL.md"), []byte("hi"), 0o644)
	os.MkdirAll(filepath.Join(kit, "skills"), 0o755)
	if err := os.Symlink(realSkill, filepath.Join(kit, "skills", "utk-cli-core")); err != nil {
		skipIfNoSymlinkPrivilege(t, err)
	}
	if err := Run(proj, kit); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	// The content must be copied through the link.
	b, err := os.ReadFile(filepath.Join(proj, ".claude", "skills", "utk-cli-core", "SKILL.md"))
	if err != nil || string(b) != "hi" {
		t.Fatalf("symlinked kit skill content should be copied, got %q err %v", b, err)
	}
}

func TestRun_InstallsSkillCopiesAndManagedBlocks(t *testing.T) {
	proj, kit := makeProject(t)
	// Give the kit skill a file so we can assert the copy carried content.
	os.WriteFile(filepath.Join(kit, "skills", "utk-cli-core", "SKILL.md"), []byte("kit"), 0o644)
	if err := Run(proj, kit); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	// Each agent dir gets a real copy of the skill (a directory, not a symlink).
	for _, dir := range []string{".claude", ".agents"} {
		skill := filepath.Join(proj, dir, "skills", "utk-cli-core")
		info, err := os.Lstat(skill)
		if err != nil {
			t.Fatalf("expected %s: %v", skill, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s should be a real copy, not a symlink", skill)
		}
		b, err := os.ReadFile(filepath.Join(skill, "SKILL.md"))
		if err != nil || string(b) != "kit" {
			t.Fatalf("%s should contain copied content, got %q err %v", skill, b, err)
		}
	}
	// Managed block in both CLAUDE.md and AGENTS.md
	for _, doc := range []string{"CLAUDE.md", "AGENTS.md"} {
		b, err := os.ReadFile(filepath.Join(proj, doc))
		if err != nil || !contains(string(b), BeginSentinel) {
			t.Fatalf("expected %s with managed block, got %q err %v", doc, b, err)
		}
	}
}

func TestRun_PreservesExistingSkills(t *testing.T) {
	proj, kit := makeProject(t)
	// A skill the project already had, unrelated to the kit.
	userSkill := filepath.Join(proj, ".claude", "skills", "my-skill")
	if err := os.MkdirAll(userSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(userSkill, "SKILL.md"), []byte("mine"), 0o644)

	if err := Run(proj, kit); err != nil {
		t.Fatal(err)
	}
	// User skill untouched
	if b, err := os.ReadFile(filepath.Join(userSkill, "SKILL.md")); err != nil || string(b) != "mine" {
		t.Fatalf("user skill must be preserved, got %q err %v", b, err)
	}
	// Kit skill installed alongside it
	if _, err := os.Lstat(filepath.Join(proj, ".claude", "skills", "utk-cli-core")); err != nil {
		t.Fatalf("kit skill should be installed: %v", err)
	}
}

// A skill the kit used to ship (utk-asset-search, dropped with `utk scan`)
// stays in the project forever unless init prunes it, and it documents a verb
// that no longer exists. Re-running init must clear it.
func TestRun_PrunesKitSkillsTheKitNoLongerShips(t *testing.T) {
	proj, kit := makeProject(t)
	stale := filepath.Join(proj, ".claude", "skills", "utk-gone")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Run(proj, kit); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale kit skill must be removed, err = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(proj, ".claude", "skills", "utk-cli-core")); err != nil {
		t.Fatalf("current kit skill should still be installed: %v", err)
	}
}

func TestRun_Idempotent(t *testing.T) {
	proj, kit := makeProject(t)
	if err := Run(proj, kit); err != nil {
		t.Fatal(err)
	}
	if err := Run(proj, kit); err != nil {
		t.Fatalf("second Run must succeed: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
	if count(string(b), BeginSentinel) != 1 {
		t.Fatal("managed block duplicated on re-run")
	}
}

func TestUninstall_RemovesKitSkillsButKeepsOthers(t *testing.T) {
	proj, kit := makeProject(t)
	os.WriteFile(filepath.Join(proj, "AGENTS.md"), []byte("# Mine\nkeep\n"), 0o644)
	// A pre-existing user skill that uninstall must not touch.
	userSkill := filepath.Join(proj, ".claude", "skills", "my-skill")
	os.MkdirAll(userSkill, 0o755)
	Run(proj, kit)
	if err := Uninstall(proj); err != nil {
		t.Fatal(err)
	}
	// Kit skill removed in every agent dir.
	for _, dir := range []string{".claude", ".agents"} {
		if _, err := os.Lstat(filepath.Join(proj, dir, "skills", "utk-cli-core")); !os.IsNotExist(err) {
			t.Fatalf("%s kit skill should be gone", dir)
		}
	}
	// User skill survives.
	if _, err := os.Stat(userSkill); err != nil {
		t.Fatalf("user skill must survive uninstall: %v", err)
	}
	// Managed block removed from AGENTS.md but user content kept.
	b, _ := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
	if contains(string(b), BeginSentinel) || !contains(string(b), "keep") {
		t.Fatalf("uninstall must drop block but keep user content; got %q", b)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && indexOf(s, sub) >= 0 }
func count(s, sub string) int {
	n, i := 0, 0
	for {
		j := indexOf(s[i:], sub)
		if j < 0 {
			return n
		}
		n++
		i += j + len(sub)
	}
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// A doc init created itself holds nothing but the managed block; stripping it
// used to leave a 0-byte CLAUDE.md behind.
func TestUninstall_RemovesDocsItCreated(t *testing.T) {
	proj, kit := makeProject(t)
	Run(proj, kit)
	if err := Uninstall(proj); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"CLAUDE.md", "AGENTS.md", ".claude", ".agents"} {
		if _, err := os.Stat(filepath.Join(proj, name)); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone, not left empty", name)
		}
	}
}
