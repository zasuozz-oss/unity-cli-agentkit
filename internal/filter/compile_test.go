package filter

import (
	"strings"
	"testing"
)

// The blob `unity command eval` returns for a failed compile, as measured on
// 1.0.0-beta.3: warnings and errors at the same indent, CS codes stripped.
const evalBlob = "Compilation Failed\n" +
	"  Identifier expected (line 1, col 55)\n" +
	"  'System' is a namespace but is used like a type (line 1, col 49)\n" +
	"  The name 'Nope' does not exist in the current context (line 1, col 64)\n" +
	"  Unreachable code detected (line 2, col 12)\n" +
	"  The variable 'i' is assigned but its value is never used (line 1, col 33)"

func TestError_CompileFailure(t *testing.T) {
	got, fatal := Error(evalBlob)
	if !fatal {
		t.Fatal("three real errors must stay fatal")
	}
	for _, want := range []string{
		"3 errors, 1 warning",
		"did not fail the compile",
		"method body",          // the contract hint, worth two round-trips of guessing
		"UnityEngine.UI.Image", // spelled out, since `using` cannot be
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Warnings must not sit among the errors, where they read as build breakers —
	// and eval's own wrapper tail ("Unreachable code detected", on every single
	// failure) must not be reported at all.
	if strings.Contains(got, "Unreachable code") {
		t.Errorf("wrapper artifact reported:\n%s", got)
	}
}

// An obsolete API and an unreachable statement are the two warnings that most
// often show up alone. Reported as errors they fail a build that compiled.
func TestError_WarningsOnlyIsNotFatal(t *testing.T) {
	in := "Compilation Failed\n" +
		"  'TMP_Text.enableWordWrapping' is obsolete: 'Use textWrappingMode instead.' (line 3, col 9)\n" +
		"  Unreachable code detected (line 9, col 12)"
	got, fatal := Error(in)
	if fatal {
		t.Fatalf("no error-class diagnostic, must not fail: %q", got)
	}
	if !strings.Contains(got, "0 errors, 1 warning") {
		t.Errorf("want a 0-error count, got:\n%s", got)
	}
}

// One bad type name reports at every site that used it. Past the first handful
// the rest describe the same cause and none of them is the one to fix.
func TestError_CompileFailureCaps(t *testing.T) {
	var b strings.Builder
	b.WriteString("Compilation Failed")
	for i := 0; i < 40; i++ {
		b.WriteString("\n  The name 'Nope" + string(rune('a'+i%26)) + string(rune('a'+i/26)) +
			"' does not exist in the current context (line " + strings.Repeat("1", 1) + ", col 4)")
	}
	got, fatal := Error(b.String())
	if !fatal {
		t.Fatal("40 errors are fatal")
	}
	if n := strings.Count(got, "does not exist"); n > errorCap {
		t.Fatalf("listed %d errors, cap is %d", n, errorCap)
	}
	if !strings.Contains(got, "more errors") {
		t.Errorf("truncation must be visible:\n%s", got)
	}
}

func TestParseDiag(t *testing.T) {
	d, ok := ParseDiag("Assets/Scripts/Foo.cs(12,20): error CS0246: The type or namespace name 'Bar' could not be found")
	if !ok {
		t.Fatal("a console compile error must parse")
	}
	if d.Severity != "error" || d.Code != "CS0246" || d.Site() != "Assets/Scripts/Foo.cs(12,20)" {
		t.Fatalf("parsed wrong: %+v", d)
	}
	// The same mistake at another site must land in the same group.
	other, _ := ParseDiag("Assets/Scripts/Baz.cs(3,9): error CS0246: The type or namespace name 'Bar' could not be found")
	if other.Key() != d.Key() {
		t.Fatalf("same cause, different key:\n%q\n%q", d.Key(), other.Key())
	}
	if _, ok := ParseDiag("NullReferenceException: Object reference not set"); ok {
		t.Fatal("a runtime log is not a compiler diagnostic")
	}
}
