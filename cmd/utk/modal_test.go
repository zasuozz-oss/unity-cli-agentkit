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
	if k, ok := scene.known(); !ok || k.button != "Reload" || !k.retry {
		t.Errorf("scene dialog: got (%+v, %v), want Reload with retry", k, ok)
	}
	// The save prompt is answered only with Cancel, and never retried: the
	// action that raised it would raise it again.
	save := dialog{
		text:    "Scene(s) Have Been Modified Do you want to save the changes you made in the scenes: Assets/Scenes/Main.unity  Your changes will be lost if you don't save them.",
		buttons: []string{"Save", "Don't Save", "Cancel"},
	}
	if k, ok := save.known(); !ok || k.button != "Cancel" || k.retry || k.next == "" {
		t.Errorf("save dialog: got (%+v, %v), want Cancel, no retry, with next step", k, ok)
	}
	if _, ok := (dialog{text: save.text, buttons: []string{"Save", "Don't Save"}}).known(); ok {
		t.Error("save dialog without Cancel: got ok=true, want false")
	}
	// A dialog utk has no entry for is reported, never clicked.
	unknown := dialog{text: "Hold on", buttons: []string{"Yes", "No"}}
	if _, ok := unknown.known(); ok {
		t.Error("unknown dialog: got ok=true, want false")
	}
	// Matching text but no such button (wording differs by Editor version):
	// clicking blind could hit the wrong option, so it must not be answered.
	renamed := dialog{text: scene.text, buttons: []string{"Ignore", "Reload Scenes"}}
	if _, ok := renamed.known(); ok {
		t.Error("dialog without the expected button: got ok=true, want false")
	}
}
