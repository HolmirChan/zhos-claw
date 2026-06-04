package agent

import (
	"testing"

	tools "github.com/sipeed/picoclaw/pkg/tools"
)

func TestRecordToolResult_ResetsOnNormalError(t *testing.T) {
	ts := &turnState{}
	ts.recordToolResult(&tools.ToolResult{BlockedType: "BLOCKED"})
	ts.recordToolResult(&tools.ToolResult{BlockedType: "DENIED"})
	ts.recordToolResult(&tools.ToolResult{BlockedType: "BLOCKED"})
	if ts.consecutiveBlockedCount != 3 {
		t.Fatalf("expected count=3 after 3 blocked, got %d", ts.consecutiveBlockedCount)
	}
	// Normal error resets
	ts.recordToolResult(&tools.ToolResult{IsError: true})
	if ts.consecutiveBlockedCount != 0 {
		t.Fatalf("expected count reset to 0 after normal error, got %d", ts.consecutiveBlockedCount)
	}
}

func TestRecordToolResult_HardAbortsAt5(t *testing.T) {
	ts := &turnState{}
	for i := 0; i < 5; i++ {
		ts.recordToolResult(&tools.ToolResult{BlockedType: "BLOCKED"})
	}
	if !ts.hardAbortRequested() {
		t.Fatal("expected hard abort after 5 consecutive blocked results")
	}
}

func TestRecordToolResult_NoOpAfterHardAbort(t *testing.T) {
	ts := &turnState{}
	for i := 0; i < 5; i++ {
		ts.recordToolResult(&tools.ToolResult{BlockedType: "BLOCKED"})
	}
	ts.recordToolResult(&tools.ToolResult{BlockedType: "BLOCKED"})
	if ts.consecutiveBlockedCount != 5 {
		t.Fatalf("expected count=5 after abort, got %d", ts.consecutiveBlockedCount)
	}
}
