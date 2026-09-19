#!/usr/bin/env bash
#
# setup-cli.sh — build and install the `utk` CLI.
#
# Produces the self-contained layout that `utk init` expects:
#
#   <home>/bin/utk            the single binary
#   <home>/skills/            bundled skills
#
# Home defaults to ~/.unity-cli-agentkit; override with UNITY_CLI_AGENTKIT_HOME.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HOME_DIR="${UNITY_CLI_AGENTKIT_HOME:-$HOME/.unity-cli-agentkit}"
MIN_GO="1.26"

info()  { printf '==> %s\n' "$*"; }
err()   { printf 'error: %s\n' "$*" >&2; }

# 1. Require Go >= MIN_GO.
if ! command -v go >/dev/null 2>&1; then
  err "Go is not installed (need >= $MIN_GO). See https://go.dev/dl/"
  exit 1
fi
GO_VER="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
if [ -z "$GO_VER" ]; then
  GO_VER="$(go version | sed -E 's/.*go([0-9.]+).*/\1/')"
fi
if [ "$(printf '%s\n%s\n' "$MIN_GO" "$GO_VER" | sort -V | head -n1)" != "$MIN_GO" ]; then
  err "Go $GO_VER is too old; need >= $MIN_GO"
  exit 1
fi
info "Go $GO_VER OK"

# 2. Build the binary. utk is stdlib-only, so the build needs no module download.
#    GOEXE is ".exe" on Windows and empty elsewhere. Without it the binary is
#    named "utk" with no extension, which PowerShell refuses to run (PATHEXT).
EXE="$(go env GOEXE)"
info "Building utk -> $HOME_DIR/bin/utk$EXE"
mkdir -p "$HOME_DIR/bin"
( cd "$REPO_DIR" && go build -o "$HOME_DIR/bin/utk$EXE" ./cmd/utk )

# 3. Sync skills into the kit home.
info "Installing skills into $HOME_DIR"
rm -rf "$HOME_DIR/skills"
cp -R "$REPO_DIR/skills" "$HOME_DIR/skills"

# 4. Report, then make `utk` runnable without manual PATH editing.
#    No smoke run: every utk verb now needs either the official `unity` binary
#    or a Unity project, and neither is guaranteed at install time. A successful
#    `go build` is the check.
info "Installed: $HOME_DIR/bin/utk$EXE"

BIN_DIR="$HOME_DIR/bin"
EXPORT_LINE="export PATH=\"$BIN_DIR:\$PATH\""

# On Windows this script runs under Git Bash but the caller is almost always
# PowerShell, which never reads a shell profile. Writing .bashrc there would
# report success and change nothing, so print the command that does work.
if [ -n "$EXE" ]; then
  WIN_BIN="$(cygpath -w "$BIN_DIR" 2>/dev/null || printf '%s' "$BIN_DIR")"
  info "Windows: add utk to PATH by running this ONCE in PowerShell:"
  printf '    [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path","User") + ";%s", "User")\n' "$WIN_BIN"
  info "then open a new terminal and run: utk status"
  exit 0
fi

# Already on PATH for this shell? Nothing more to do.
case ":$PATH:" in
  *":$BIN_DIR:"*)
    info "Ready. '$BIN_DIR' is already on PATH; run: utk status"
    exit 0
    ;;
esac

# Persist to the right shell profile (idempotent) so new shells find utk.
#
# For zsh we target .zshenv, not .zshrc: .zshrc is sourced only for
# *interactive* shells, so a PATH set there is invisible to non-interactive
# shells (scripts, `zsh -c`, agent/tool subprocesses). .zshenv is sourced for
# every zsh invocation, which is what we want for a CLI on PATH.
shell_name="$(basename "${SHELL:-}")"
case "$shell_name" in
  zsh)  PROFILE="${ZDOTDIR:-$HOME}/.zshenv" ;;
  bash) PROFILE="$HOME/.bashrc" ;;
  *)    PROFILE="$HOME/.profile" ;;
esac

MARKER="# unity-cli-agentkit (utk)"
if [ -f "$PROFILE" ] && grep -qF "$MARKER" "$PROFILE"; then
  info "PATH entry already present in $PROFILE"
else
  printf '\n%s\n%s\n' "$MARKER" "$EXPORT_LINE" >> "$PROFILE"
  info "Added utk to PATH in $PROFILE"
fi

# A script cannot change the PATH of the shell that launched it, so tell the
# user how to use utk right now without opening a new terminal.
info "To use utk in THIS terminal now, run:"
printf '    source %s\n' "$PROFILE"
info "(new terminals pick it up automatically)"
