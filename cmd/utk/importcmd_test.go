package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPrepImportArgs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "hero.png")
	if err := os.WriteFile(src, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("positional file and dest become flags", func(t *testing.T) {
		got, code := prepImportArgs([]string{src, "Textures/hero.png", "--confirm", "true"}, &bytes.Buffer{})
		want := []string{"--source", src, "--path", "Textures/hero.png", "--confirm", "true"}
		if code != 0 || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v (code %d), want %v", got, code, want)
		}
	})

	t.Run("dest defaults to the file name", func(t *testing.T) {
		got, code := prepImportArgs([]string{src}, &bytes.Buffer{})
		want := []string{"--source", src, "--path", "hero.png"}
		if code != 0 || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v (code %d), want %v", got, code, want)
		}
	})

	t.Run("flag value is not mistaken for a dest", func(t *testing.T) {
		got, code := prepImportArgs([]string{"--dry_run", "true", src}, &bytes.Buffer{})
		want := []string{"--source", src, "--path", "hero.png", "--dry_run", "true"}
		if code != 0 || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v (code %d), want %v", got, code, want)
		}
	})

	t.Run("missing file refused locally", func(t *testing.T) {
		var errBuf bytes.Buffer
		if _, code := prepImportArgs([]string{filepath.Join(dir, "nope.png")}, &errBuf); code != 2 {
			t.Fatalf("code = %d, want 2 (stderr: %s)", code, errBuf.String())
		}
	})

	t.Run("no positionals refused", func(t *testing.T) {
		if _, code := prepImportArgs([]string{"--confirm", "true"}, &bytes.Buffer{}); code != 2 {
			t.Fatalf("want usage error")
		}
	})

	t.Run("explicit --source passes through", func(t *testing.T) {
		in := []string{"--source", "/nowhere/x.png", "--path", "x.png"}
		got, code := prepImportArgs(in, &bytes.Buffer{})
		if code != 0 || !reflect.DeepEqual(got, in) {
			t.Fatalf("got %v (code %d), want passthrough", got, code)
		}
	})
}

func TestEvalTimeoutCap(t *testing.T) {
	raw := []byte(`{"errors":[{"code":"COMMAND_FAILED","message":"Bad Request\nTimeout must be between 1ms and 30000ms"}]}`)
	if got := evalTimeoutCap(raw); got != 30000 {
		t.Fatalf("cap = %d, want 30000", got)
	}
	if got := evalTimeoutCap([]byte(`{"success":true}`)); got != 0 {
		t.Fatalf("cap on unrelated output = %d, want 0", got)
	}
}

func TestExecTimeoutArgs(t *testing.T) {
	want := []string{"--timeout", "70", "--", "--timeout", "60000"}
	if got := execTimeoutArgs(60000); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
