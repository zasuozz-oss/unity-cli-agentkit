#!/usr/bin/env bash
# SessionStart hook of the unity-cli-agentkit plugin: hands Claude the kit's
# guidance (the same block `utk init` writes into AGENTS.md for Codex), but only
# inside a Unity project, so other projects pay nothing beyond the skill list.
set -u
dir="${CLAUDE_PROJECT_DIR:-$PWD}"
[ -d "$dir/Assets" ] && [ -d "$dir/ProjectSettings" ] || exit 0
# A project set up by a pre-plugin `utk init` already carries the block in
# CLAUDE.md; re-running `utk init` clears it, until then don't say it twice.
grep -qs 'BEGIN unity-cli-agentkit' "$dir/CLAUDE.md" && exit 0
root="${CLAUDE_PLUGIN_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
body=$(cat "$root/internal/initcmd/agents-block.md") || exit 0
# Without com.unity.pipeline no utk command reaches the Editor. The agent can
# install it itself; `utk init` would also write the Codex files, so name the
# official command directly.
if ! grep -qs '"com.unity.pipeline"' "$dir/Packages/manifest.json" && [ ! -d "$dir/Packages/com.unity.pipeline" ]; then
  body="**Setup needed: this Unity project has no \`com.unity.pipeline\` package,
so no \`utk\` command can reach the Editor yet.** Before the first \`utk\` call
of a task that needs the Editor, install it yourself from the project root
(it edits Packages/manifest.json; tell the user you did):

    unity pipeline install --project-path \"\$PWD\" --non-interactive --no-banner

Give it a 120s timeout. \`unity: command not found\` means the official Unity
CLI is missing: stop and ask the user to install it (see utk-cli-core). If
the Editor is already open, ask the user to focus its window once (or reopen
the project) so the package resolves, then check \`utk status\`.

$body"
fi
body="${body//\\/\\\\}"
body="${body//\"/\\\"}"
body="${body//$'\n'/\\n}"
body="${body//$'\r'/\\r}"
body="${body//$'\t'/\\t}"
printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"%s"}}\n' "$body"
