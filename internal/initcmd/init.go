package initcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

// legacySkillPrefix is how ownership was tracked before the kit shipped skills
// under other names (unity-*): every kit skill started with utk-. Installs made
// by those versions have no manifest, so prune and uninstall still honour the
// prefix as a fallback.
const legacySkillPrefix = "utk-"

// manifestName records, inside each project skills dir, exactly which entries
// `utk init` put there. It is the source of truth for what the kit may remove,
// so a kit skill can be named anything without risking a user's own skills.
const manifestName = ".utk-skills.json"

// skillsHostDirs are the per-agent directories that host the kit's skills.
// Claude Code reads .claude/skills; Codex and Antigravity read .agents/skills.
var skillsHostDirs = []string{".claude", ".agents"}

// agentDocs are the instruction files that receive the managed guidance block.
// Claude Code reads CLAUDE.md; Codex and Antigravity read AGENTS.md.
var agentDocs = []string{"CLAUDE.md", "AGENTS.md"}

// Run installs project-local pointers into proj, sourcing skills from kit
// (kit/skills). It refuses any directory that is not a Unity project.
func Run(proj, kit string) error {
	if !DetectUnityProject(proj) {
		return errors.New("not a Unity project (need Assets/ and ProjectSettings/); refusing to modify this directory")
	}
	kitSkills := filepath.Join(kit, "skills")
	names, err := kitSkillNames(kitSkills)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		// A healthy kit always ships skills; zero means the kit's skills/ is
		// missing or corrupted (e.g. dangling/circular symlinks). Surface it
		// instead of reporting a successful install that placed nothing.
		return errors.New("no kit skills found under " + kitSkills + " (kit may be missing or corrupted); reinstall with setup-cli.sh")
	}
	for _, dir := range skillsHostDirs {
		if err := installSkills(proj, dir, kitSkills, names); err != nil {
			return err
		}
	}
	return writeManagedBlocks(proj)
}

// kitSkillNames lists the kit's own skill folders: every directory under
// skills/ holding a SKILL.md. Names carry no meaning (the kit ships utk-* CLI
// skills alongside vendored unity-* advisory ones), so the marker file is what
// separates a skill from stray content.
func kitSkillNames(kitSkills string) ([]string, error) {
	entries, err := os.ReadDir(kitSkills)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		// Follow symlinks: a kit skill may be a real directory or a symlink to
		// one. os.ReadDir does not resolve links (e.IsDir() is false for them),
		// so Stat the target. A broken or circular link errors out and is
		// skipped, so init never installs a dangling pointer.
		info, err := os.Stat(filepath.Join(kitSkills, e.Name()))
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(kitSkills, e.Name(), "SKILL.md")); err != nil {
			continue
		}
		names = append(names, e.Name())
	}
	return names, nil
}

// ownedSkills reports which entries in a project's skills dir belong to the
// kit: the manifest an earlier init wrote, plus any utk-* entry from before
// manifests existed. Anything else in that directory is the user's.
func ownedSkills(dest string) map[string]bool {
	owned := make(map[string]bool)
	if b, err := os.ReadFile(filepath.Join(dest, manifestName)); err == nil {
		var m struct {
			Skills []string `json:"skills"`
		}
		if json.Unmarshal(b, &m) == nil {
			for _, name := range m.Skills {
				owned[name] = true
			}
		}
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return owned
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), legacySkillPrefix) {
			owned[e.Name()] = true
		}
	}
	return owned
}

// writeManifest records the names this init installed into dest.
func writeManifest(dest string, names []string) error {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	b, err := json.MarshalIndent(struct {
		Skills []string `json:"skills"`
	}{sorted}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, manifestName), append(b, '\n'), 0o644)
}

// InstallPipeline installs com.unity.pipeline via the official CLI,
// best-effort with a hard timeout. Swappable in tests. Callers treat an error
// as a warning, never a failed init.
var InstallPipeline = func(proj string) error {
	bin, err := official.Bin()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin,
		"pipeline", "install", "--project-path", proj, "--non-interactive", "--no-banner")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("unity pipeline install: %v: %s", err, out)
	}
	return nil
}

// installSkills copies each kit skill into <proj>/<agentDir>/skills/<name>,
// leaving any other skill in that directory untouched. Only utk-* entries are
// (re)created or pruned, so a project's existing skills survive. Copying
// (rather than symlinking) keeps each project self-contained: it never points
// back into the central store, so the store can never be corrupted through a
// project, and it works the same on every OS. The trade-off is that updating a
// kit skill requires re-running `utk init` to refresh the copies.
func installSkills(proj, agentDir, kitSkills string, names []string) error {
	dest := filepath.Join(proj, agentDir, "skills")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	// Guard against self-corruption: if the project's skills dir resolves to the
	// kit's own store (e.g. .claude/skills is a symlink to <kit>/skills), copying
	// each skill into it would copy the store entries onto themselves. The skills
	// are already visible through that link, so there is nothing to install — skip.
	if destReal, err := filepath.EvalSymlinks(dest); err == nil {
		if storeReal, err := filepath.EvalSymlinks(kitSkills); err == nil && destReal == storeReal {
			return nil
		}
	}
	// Prune kit skills the kit no longer ships. A previous init copied them in,
	// and nothing else ever removes them — leaving an agent reading a skill for a
	// verb that no longer exists. Only entries the kit installed are considered.
	current := make(map[string]bool, len(names))
	for _, name := range names {
		current[name] = true
	}
	for name := range ownedSkills(dest) {
		if !current[name] {
			if err := os.RemoveAll(filepath.Join(dest, name)); err != nil {
				return err
			}
		}
	}
	for _, name := range names {
		dst := filepath.Join(dest, name)
		// Resolve symlinked kit skills to their real directory so we copy content,
		// not a dangling link.
		src, err := filepath.EvalSymlinks(filepath.Join(kitSkills, name))
		if err != nil {
			return err
		}
		// Idempotent: replace only this entry (our own skill), never the dir.
		if _, err := os.Lstat(dst); err == nil {
			if err := os.RemoveAll(dst); err != nil {
				return err
			}
		}
		if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
			return err
		}
	}
	return writeManifest(dest, names)
}

// writeManagedBlocks upserts the guidance block into every agent doc.
func writeManagedBlocks(proj string) error {
	for _, name := range agentDocs {
		path := filepath.Join(proj, name)
		var content string
		if b, err := os.ReadFile(path); err == nil {
			content = string(b)
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(path, []byte(UpsertManagedBlock(content)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Uninstall removes only the kit's own skills (utk-* entries) and the managed
// block from every agent doc. It never touches com.unity.pipeline — removing a
// package the user may depend on is not utk's call. A project's other skills
// and any content outside the managed block are preserved.
func Uninstall(proj string) error {
	for _, dir := range skillsHostDirs {
		if err := removeKitSkills(filepath.Join(proj, dir, "skills")); err != nil {
			return err
		}
	}
	for _, name := range agentDocs {
		path := filepath.Join(proj, name)
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		// A doc init created itself holds nothing but the managed block, so
		// writing the stripped text back leaves a 0-byte CLAUDE.md behind —
		// the same litter removeKitSkills avoids by dropping an emptied dir.
		rest := RemoveManagedBlock(string(b))
		if strings.TrimSpace(rest) == "" {
			if err := os.Remove(path); err != nil {
				return err
			}
			continue
		}
		if err := os.WriteFile(path, []byte(rest), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// removeKitSkills deletes only the entries the kit installed (per the manifest,
// plus legacy utk-* ones), then removes dest itself if that leaves it empty.
// Skills the kit never installed are left in place.
func removeKitSkills(dest string) error {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	for name := range ownedSkills(dest) {
		if err := os.RemoveAll(filepath.Join(dest, name)); err != nil {
			return err
		}
	}
	if err := os.Remove(filepath.Join(dest, manifestName)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if remaining, _ := os.ReadDir(dest); len(remaining) == 0 {
		_ = os.Remove(dest)
		// …and the .claude/.agents wrapper too, if skills/ was all it held.
		// os.Remove refuses a non-empty dir, so a real user dir is safe.
		_ = os.Remove(filepath.Dir(dest))
	}
	return nil
}
