package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// writePNG lays down a real image at the native size a phone-shaped game view
// captures at, so the resize is exercised end to end rather than mocked.
func writePNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 0x40, 0xff})
		}
	}
	path := filepath.Join(t.TempDir(), "shot.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return path
}

func shotBody(path string, w, h int) string {
	return `{"success":true,"command":"command screenshot","data":{"command":"screenshot","result":` +
		`{"success":true,"path":"` + path + `","view":"game","width":` + strconv.Itoa(w) +
		`,"height":` + strconv.Itoa(h) + `}},"errors":[],"warnings":[]}`
}

func pngSize(t *testing.T, path string) (int, int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height
}

// A 720×1600 capture costs ~1,450 tokens to read and a 512-long-edge one ~520,
// with layout and text placement intact. The tool cannot do it in one call —
// --width alone stretches the image to the view's full height — so the resize
// happens here, after the capture, without a second one.
func TestRun_ScreenshotShrinksToMaxEdge(t *testing.T) {
	path := writePNG(t, 720, 1600)
	bin, _ := fakeUnity(t, shotBody(path, 720, 1600), 0)
	fakeUnityNext(t, `{"success":true,"data":{"result":{"result":0}},"errors":[],"warnings":[]}`)
	t.Setenv("UTK_UNITY_BIN", bin)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"screenshot"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr.String())
	}
	w, h := pngSize(t, path)
	if w != 230 || h != 512 {
		t.Fatalf("resized to %dx%d, want 230x512 (aspect preserved, long edge 512)", w, h)
	}
	// The payload must describe the file that is actually on disk.
	if !strings.Contains(stdout.String(), `"width":230`) {
		t.Errorf("payload still reports the native size: %s", stdout.String())
	}
}

// An explicit size is an instruction, not a default to improve on.
func TestRun_ScreenshotHonoursExplicitSize(t *testing.T) {
	path := writePNG(t, 720, 1600)
	bin, _ := fakeUnity(t, shotBody(path, 720, 1600), 0)
	fakeUnityNext(t, `{"success":true,"data":{"result":{"result":0}},"errors":[],"warnings":[]}`)
	t.Setenv("UTK_UNITY_BIN", bin)
	t.Setenv("UTK_NO_FLAG_CHECK", "1")

	var stdout, stderr bytes.Buffer
	run([]string{"screenshot", "--width", "720", "--height", "1600"}, &stdout, &stderr)
	if w, h := pngSize(t, path); w != 720 || h != 1600 {
		t.Fatalf("resized to %dx%d despite an explicit size", w, h)
	}
}

func TestRun_ScreenshotMaxZeroKeepsNativeSize(t *testing.T) {
	path := writePNG(t, 720, 1600)
	bin, _ := fakeUnity(t, shotBody(path, 720, 1600), 0)
	fakeUnityNext(t, `{"success":true,"data":{"result":{"result":0}},"errors":[],"warnings":[]}`)
	t.Setenv("UTK_UNITY_BIN", bin)

	var stdout, stderr bytes.Buffer
	run([]string{"screenshot", "--max", "0"}, &stdout, &stderr)
	if w, h := pngSize(t, path); w != 720 || h != 1600 {
		t.Fatalf("--max 0 must keep the native size, got %dx%d", w, h)
	}
}

// An overlay canvas is composited after the camera render, so it is missing
// from the capture — which otherwise reports success with a path and a size,
// and is only discovered by opening a blank PNG.
func TestRun_ScreenshotWarnsAboutOverlayCanvas(t *testing.T) {
	path := writePNG(t, 200, 200)
	bin, _ := fakeUnity(t, shotBody(path, 200, 200), 0)
	fakeUnityNext(t, `{"success":true,"data":{"result":{"result":2}},"errors":[],"warnings":[]}`)
	t.Setenv("UTK_UNITY_BIN", bin)

	var stdout, stderr bytes.Buffer
	run([]string{"screenshot"}, &stdout, &stderr)
	if !strings.Contains(stderr.String(), "ScreenSpaceOverlay") {
		t.Fatalf("want an overlay warning, got %q", stderr.String())
	}
	if code := run([]string{"screenshot"}, &stdout, &stderr); code != 0 {
		t.Fatalf("a warning must not fail the command, exit = %d", code)
	}
}
