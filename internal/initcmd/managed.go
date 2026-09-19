package initcmd

import (
	_ "embed"
	"strings"
)

const (
	BeginSentinel = "<!-- BEGIN unity-cli-agentkit (auto-managed — do not edit by hand) -->"
	EndSentinel   = "<!-- END unity-cli-agentkit -->"
)

// blockBody is the managed-block content written into CLAUDE.md and AGENTS.md by `utk init`.
// It lives in a reviewable, editable file (agents-block.md) and is embedded into
// the binary at build time so the kit stays single-binary and zero-dependency.
// Edit agents-block.md to change what init copies into projects.
//
//go:embed agents-block.md
var blockBody string

func managedBlock() string {
	return BeginSentinel + "\n" + strings.TrimRight(blockBody, "\n") + "\n" + EndSentinel
}

// UpsertManagedBlock inserts the managed block, or replaces an existing one
// in place, touching nothing outside the sentinels (spec §4.2). Idempotent.
func UpsertManagedBlock(content string) string {
	block := managedBlock()
	begin := strings.Index(content, BeginSentinel)
	end := strings.Index(content, EndSentinel)
	if begin >= 0 && end > begin {
		return content[:begin] + block + content[end+len(EndSentinel):]
	}
	if content == "" {
		return block + "\n"
	}
	sep := "\n"
	if !strings.HasSuffix(content, "\n") {
		sep = "\n\n"
	} else if !strings.HasSuffix(content, "\n\n") {
		sep = "\n"
	}
	return content + sep + block + "\n"
}

// RemoveManagedBlock deletes exactly the sentinel-delimited block (and the
// blank line preceding it, if any), leaving all other content byte-identical.
func RemoveManagedBlock(content string) string {
	begin := strings.Index(content, BeginSentinel)
	end := strings.Index(content, EndSentinel)
	if begin < 0 || end < begin {
		return content
	}
	before := content[:begin]
	before = strings.TrimRight(before, "\n")
	after := content[end+len(EndSentinel):]
	after = strings.TrimLeft(after, "\n")
	if before == "" {
		return after
	}
	if after == "" {
		return before + "\n"
	}
	return before + "\n\n" + after
}
