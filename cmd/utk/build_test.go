package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// `utk build` must answer with the build's verdict, not the queue receipt, and
// a build that did not succeed must reach $?.
func TestRun_BuildWaitsForTheVerdict(t *testing.T) {
	queued := `{"success":true,"data":{"result":{"buildId":"build_1","status":"queued"}}}`
	done := `{"success":true,"data":{"result":{"status":"completed","buildId":"build_1","result":"Failed","totalErrors":1,` +
		`"errors":["Assets/Foo.cs(3,5): error CS0103"],"files":[{"path":"a","role":"x","sizeBytes":1}]}}}`
	bin, _ := fakeUnity(t, queued, 0)
	t.Setenv("UTK_UNITY_BIN", bin)
	t.Setenv("UTK_NO_FLAG_CHECK", "1")
	fakeUnityNext(t, done)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"build", "--confirm", "true"}, &stdout, &stderr); code != 1 {
		t.Errorf("exit = %d, want 1 for a failed build (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "CS0103") || strings.Contains(out, `"queued"`) {
		t.Errorf("want the verdict with its errors, got %q", out)
	}
	if strings.Contains(out, `"files"`) {
		t.Errorf("file inventory must be trimmed: %q", out)
	}
}

// --wait false is utk's flag: it must keep the hand-off AND never reach the
// build tool, whose schema does not know it.
func TestRun_BuildNoWaitKeepsHandoff(t *testing.T) {
	queued := `{"success":true,"data":{"result":{"buildId":"build_1","status":"queued"}}}`
	bin, argv := fakeUnity(t, queued, 0)
	t.Setenv("UTK_UNITY_BIN", bin)
	t.Setenv("UTK_NO_FLAG_CHECK", "1")
	fakeUnityNext(t, `{"success":false,"errors":[{"code":"X","message":"polled when it should not have"}]}`)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"build", "--confirm", "true", "--wait", "false"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "queued") {
		t.Errorf("stdout = %q", stdout.String())
	}
	if strings.Contains(readArgv(t, argv), "--wait") {
		t.Error("--wait leaked to the unity CLI")
	}
}

func TestTrimBuildReport(t *testing.T) {
	// In flight: untouched.
	in := []byte(`{"status":"building","buildId":"b"}`)
	if got := trimBuildReport(in); !bytes.Equal(got, in) {
		t.Errorf("in-flight status changed: %s", got)
	}
	// Completed: inventory gone, errors capped, scalars kept.
	errs := make([]string, buildErrorCap+3)
	for i := range errs {
		errs[i] = "e"
	}
	eb, _ := json.Marshal(errs)
	got := trimBuildReport([]byte(`{"status":"completed","result":"Failed","totalSizeBytes":5,"files":[1,2],"packedAssets":[],"errors":` + string(eb) + `}`))
	var d map[string]any
	if err := json.Unmarshal(got, &d); err != nil {
		t.Fatal(err, string(got))
	}
	if _, ok := d["files"]; ok || d["totalSizeBytes"] != float64(5) {
		t.Errorf("wrong trim: %s", got)
	}
	if l := d["errors"].([]any); len(l) != buildErrorCap+1 || !strings.Contains(l[buildErrorCap].(string), "+3 more") {
		t.Errorf("errors not capped: %v", l)
	}
}
