package initcmd

import (
	"strings"
	"testing"
)

const existing = "# My Agents\n\nKeep this line.\n"

func TestUpsert_AppendsWhenAbsent(t *testing.T) {
	got := UpsertManagedBlock(existing)
	if !strings.Contains(got, "Keep this line.") {
		t.Fatal("must preserve pre-existing content")
	}
	if !strings.Contains(got, BeginSentinel) || !strings.Contains(got, EndSentinel) {
		t.Fatal("must insert the managed block with sentinels")
	}
}

func TestUpsert_IsIdempotent(t *testing.T) {
	once := UpsertManagedBlock(existing)
	twice := UpsertManagedBlock(once)
	if once != twice {
		t.Fatalf("upsert must be idempotent.\nonce:\n%s\ntwice:\n%s", once, twice)
	}
	if strings.Count(twice, BeginSentinel) != 1 {
		t.Fatal("must not duplicate the block")
	}
}

func TestRemove_LeavesEverythingElseUntouched(t *testing.T) {
	withBlock := UpsertManagedBlock(existing)
	got := RemoveManagedBlock(withBlock)
	if strings.Contains(got, BeginSentinel) {
		t.Fatal("block must be gone")
	}
	if !strings.Contains(got, "Keep this line.") {
		t.Fatal("non-block content must survive removal")
	}
}
