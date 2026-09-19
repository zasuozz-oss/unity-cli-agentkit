package main

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

// activeSceneIs reports whether path already names the active scene.
//
// set_active_scene is the one tool that fails for having nothing to do: asked
// for the scene that is already active it answers "Failed to set '…' as the
// active scene" and exits 6, so the otherwise idempotent `open_scene &&
// set_active_scene` pair succeeds once and breaks on every rerun. Since the
// call has already failed by the time this runs, the extra round-trip costs
// nothing on the path that works.
func activeSceneIs(path string) bool {
	if path == "" {
		return false
	}
	raw, code := official.Capture(
		[]string{"command", "list_open_scenes", "--json", "--no-banner"}, io.Discard)
	if code != 0 {
		return false
	}
	env, err := official.Parse(raw)
	if err != nil || !env.Success {
		return false
	}
	var d struct {
		Scenes []struct {
			Name     string `json:"name"`
			Path     string `json:"path"`
			IsActive bool   `json:"isActive"`
		} `json:"scenes"`
	}
	if json.Unmarshal(env.Payload(true), &d) != nil {
		return false
	}
	// The tool takes either a project path or a bare scene name, so match on
	// both spellings of whatever was asked for.
	want := strings.TrimSuffix(path, ".unity")
	for _, s := range d.Scenes {
		if s.IsActive && (strings.TrimSuffix(s.Path, ".unity") == want || s.Name == want) {
			return true
		}
	}
	return false
}
