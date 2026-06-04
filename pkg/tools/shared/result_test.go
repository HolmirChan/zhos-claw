package toolshared

import (
	"strings"
	"testing"
)

func TestContentForLLM_BlockedTypePrefix(t *testing.T) {
	tr := &ToolResult{ForLLM: "Command blocked", BlockedType: BlockedTypeBlocked}
	content := tr.ContentForLLM()
	if !strings.HasPrefix(content, "[BLOCKED] ") {
		t.Fatalf("expected [BLOCKED] prefix, got: %s", content)
	}
	if !strings.Contains(content, "Command blocked") {
		t.Fatalf("expected body preserved, got: %s", content)
	}
}

func TestContentForLLM_EmptyContentNoPrefix(t *testing.T) {
	tr := &ToolResult{ForLLM: "", BlockedType: BlockedTypeBlocked}
	content := tr.ContentForLLM()
	if strings.HasPrefix(content, "[BLOCKED]") {
		t.Fatalf("expected no prefix on empty content, got: %s", content)
	}
}

func TestContentForLLM_NoPrefixOnNormalError(t *testing.T) {
	tr := &ToolResult{ForLLM: "file not found", IsError: true}
	content := tr.ContentForLLM()
	if strings.Contains(content, "[BLOCKED]") || strings.Contains(content, "[DENIED]") {
		t.Fatalf("expected no prefix on normal error, got: %s", content)
	}
}
