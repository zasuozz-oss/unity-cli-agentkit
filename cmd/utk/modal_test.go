package main

import "testing"

func TestMainThreadTimeout(t *testing.T) {
	blocked := []string{
		// utk exec, measured against a 6000.0.84f1 Editor sitting behind a dialog
		"Pipeline server returned 400 Bad Request: Internal Server Error. Main thread operation timed out after 60000ms",
		// utk editor refresh, same Editor
		"Pipeline command 'recompile' timed out after 30000ms",
	}
	for _, msg := range blocked {
		if !mainThreadTimeout(msg) {
			t.Errorf("mainThreadTimeout(%q) = false, want true", msg)
		}
	}
	other := []string{
		"No Unity Editor instances found with reachable Pipeline servers.",
		"Assets/Game/Board.cs(12,5): error CS1002: ; expected",
		// A build is not a main-thread call; its timeout must not claim a dialog.
		"Build timed out after 1800000ms",
	}
	for _, msg := range other {
		if mainThreadTimeout(msg) {
			t.Errorf("mainThreadTimeout(%q) = true, want false", msg)
		}
	}
}

func TestDialogKnown(t *testing.T) {
	// Text and buttons as read off a live 6000.0.84f1 Editor.
	scene := dialog{
		text:    "The open scene(s) have been modified externally The following open scene(s) have been changed on disk: Assets/__Game/Scenes/Game.unity  Do you want to reload the scene(s)?",
		buttons: []string{"Ignore", "Reload"},
	}
	if b, _, ok := scene.known(); !ok || b != "Reload" {
		t.Errorf("scene dialog: got (%q, %v), want (\"Reload\", true)", b, ok)
	}
	// A dialog utk has no entry for is reported, never clicked.
	unknown := dialog{text: "Hold on", buttons: []string{"Yes", "No"}}
	if _, _, ok := unknown.known(); ok {
		t.Error("unknown dialog: got ok=true, want false")
	}
	// Matching text but no such button (wording differs by Editor version):
	// clicking blind could hit the wrong option, so it must not be answered.
	renamed := dialog{text: scene.text, buttons: []string{"Ignore", "Reload Scenes"}}
	if _, _, ok := renamed.known(); ok {
		t.Error("dialog without the expected button: got ok=true, want false")
	}
}
