package main

import "testing"

// test_status hands its report back as a JSON string, not an object; reading it
// as one makes every poll look unfinished and the run never returns.
func TestTestRunFinished(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    bool
	}{
		{"running", `"{\"status\":\"running\"}"`, false},
		{"completed", `"{\"status\":\"completed\",\"summary\":{}}"`, true},
		{"no run in progress", `"{\"status\":\"no_tests\"}"`, true},
		{"unwrapped object", `{"status":"running"}`, false},
		{"garbage keeps polling", `"nope"`, false},
	}
	for _, c := range cases {
		if got := testRunFinished([]byte(c.payload)); got != c.want {
			t.Errorf("%s: testRunFinished = %v, want %v", c.name, got, c.want)
		}
	}
}
