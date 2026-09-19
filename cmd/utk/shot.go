package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

// overlayProbe counts active Screen Space - Overlay canvases in the open
// scenes and, when there are any, queues a second capture that includes them.
// Resources.FindObjectsOfTypeAll is used instead of FindObjectsOfType
// (obsolete since 2023) / FindObjectsByType (absent before it) so the snippet
// compiles on every Editor version; the scene check drops the prefab and asset
// copies it also returns.
//
// ScreenCapture.CaptureScreenshot grabs the composited game view — overlay UI
// included — but only lands after the next rendered frame, which is why the
// pipeline's own tool renders a camera instead. Waiting for the file is free
// here: it is on local disk, so the wait costs no Editor round-trip.
const overlayProbe = `int n = 0; foreach (var c in UnityEngine.Resources.FindObjectsOfTypeAll<UnityEngine.Canvas>()) ` +
	`if (c.gameObject.scene.IsValid() && c.isActiveAndEnabled && c.renderMode == UnityEngine.RenderMode.ScreenSpaceOverlay) n++; `

// overlayRecapture is appended to the probe when the caller took the default
// size — the only case ScreenCapture can answer, since it has no width/height.
const overlayRecapture = `if (n > 0) UnityEngine.ScreenCapture.CaptureScreenshot(%q); `

// overlayWait bounds the wait for that frame. A game view that is closed or
// never repaints writes nothing, and the camera-rendered capture is still a
// valid answer — so a timeout degrades to today's warning rather than failing.
const overlayWait = 6 * time.Second

// fixOverlayShot replaces the capture with one that contains the overlay UI,
// when the scene has any. The screenshot tool renders through the view camera
// and an overlay canvas is composited by the UI system after that, so a screen
// whose visible content is all overlay UI captures as an empty frame — while
// the payload reports success with a path and a size like any other call. The
// failure is only visible by opening the PNG.
//
// The returned payload keeps the original path (the composited PNG is renamed
// over it) with the size corrected, so callers need not care which capture
// answered.
func fixOverlayShot(payload []byte, view, projectPath string, sized bool, stderr io.Writer) []byte {
	if view != "" && view != "game" {
		return payload
	}
	var d map[string]any
	if json.Unmarshal(payload, &d) != nil {
		return payload
	}
	path, _ := d["path"].(string)
	if path == "" {
		return payload
	}
	// Capture beside the target and rename on success: a game view that never
	// repaints must leave the camera-rendered PNG intact.
	tmp := filepath.Join(filepath.Dir(path), ".utk-overlay-"+filepath.Base(path))
	os.Remove(tmp)
	probe := overlayProbe + "return n;"
	if !sized {
		probe = overlayProbe + fmt.Sprintf(overlayRecapture, tmp) + "return n;"
	}
	args := []string{"command", "eval", probe, "--json", "--no-banner"}
	if projectPath != "" {
		args = append(args, "--project-path", projectPath)
	}
	raw, code := official.Capture(args, io.Discard)
	if code != 0 {
		return payload
	}
	env, err := official.Parse(raw)
	if err != nil || !env.Success {
		return payload
	}
	var res struct {
		Result *int `json:"result"`
	}
	if json.Unmarshal(env.Payload(true), &res) != nil || res.Result == nil || *res.Result == 0 {
		return payload
	}
	w, h, ok := 0, 0, false
	if !sized {
		w, h, ok = waitForPNG(tmp, overlayWait)
	}
	if !ok {
		os.Remove(tmp)
		fmt.Fprintf(stderr, "utk: WARNING: %d active ScreenSpaceOverlay canvas(es); overlay UI is composited\n", *res.Result)
		fmt.Fprintln(stderr, "  after the camera render, so it is missing from this capture (often a blank frame).")
		if sized {
			fmt.Fprintln(stderr, "  Drop --width/--height to get the composited capture instead (native size).")
		} else {
			fmt.Fprintln(stderr, "  The game view rendered no frame to re-capture — open/focus that window, or read")
			fmt.Fprintln(stderr, "  the layout with `utk get_scene_hierarchy`.")
		}
		return payload
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return payload
	}
	d["width"], d["height"] = w, h
	out, err := json.Marshal(d)
	if err != nil {
		return payload
	}
	return out
}

// waitForPNG waits for a complete PNG to appear at path, returning its size.
// Decoding the header is what makes "complete" checkable: the file exists from
// the moment Unity opens it, and a size read a millisecond too early would
// describe a truncated write.
func waitForPNG(path string, budget time.Duration) (w, h int, ok bool) {
	deadline := time.Now().Add(budget)
	for {
		if f, err := os.Open(path); err == nil {
			cfg, err := png.DecodeConfig(f)
			f.Close()
			if err == nil {
				return cfg.Width, cfg.Height, true
			}
		}
		if time.Now().After(deadline) {
			return 0, 0, false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// shrinkShot rewrites the captured PNG so its long edge is at most max pixels,
// and corrects the size the payload reports.
//
// The tool scales only what it is told to: --width on its own stretches the
// image to the view's full height, and the native size is not knowable until
// after the capture — so asking for a smaller one costs a second capture.
// Resizing the file afterwards costs neither. A failure is never fatal: the
// full-size screenshot is still a valid answer.
func shrinkShot(payload []byte, max int, stderr io.Writer) []byte {
	if max <= 0 {
		return payload
	}
	var d map[string]any
	if json.Unmarshal(payload, &d) != nil {
		return payload
	}
	path, _ := d["path"].(string)
	w, hOK := toInt(d["width"])
	h, wOK := toInt(d["height"])
	if path == "" || !hOK || !wOK || (w <= max && h <= max) {
		return payload
	}
	nw, nh, err := resizePNG(path, max)
	if err != nil {
		fmt.Fprintln(stderr, "utk: screenshot: keeping native size:", err)
		return payload
	}
	d["width"], d["height"] = nw, nh
	out, err := json.Marshal(d)
	if err != nil {
		return payload
	}
	return out
}

func toInt(v any) (int, bool) {
	f, ok := v.(float64)
	return int(f), ok
}

// resizePNG rewrites path scaled so its long edge is max, returning the new
// size. The box filter averages every source pixel a destination pixel covers;
// nearest-neighbour is shorter but drops UI text below legibility at 0.4×,
// which defeats the point of taking the screenshot.
func resizePNG(path string, max int) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		return 0, 0, err
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	long := sw
	if sh > long {
		long = sh
	}
	if sw <= 0 || sh <= 0 || long <= max {
		return sw, sh, nil
	}
	dw, dh := sw*max/long, sh*max/long
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		y0, y1 := y*sh/dh, (y+1)*sh/dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < dw; x++ {
			x0, x1 := x*sw/dw, (x+1)*sw/dw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					r, g, bl, a = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca)
				}
			}
			n := uint64((y1 - y0) * (x1 - x0))
			dst.Set(x, y, color.RGBA64{uint16(r / n), uint16(g / n), uint16(bl / n), uint16(a / n)})
		}
	}
	// Write beside the target and rename, so a failure mid-encode cannot leave
	// a truncated PNG where the payload says a screenshot is.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".utk-shot-*.png")
	if err != nil {
		return 0, 0, err
	}
	if err := png.Encode(tmp, dst); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return 0, 0, err
	}
	tmp.Close()
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return 0, 0, err
	}
	return dw, dh, nil
}
