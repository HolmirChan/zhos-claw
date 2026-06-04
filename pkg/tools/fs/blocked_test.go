package fstools

import (
	"fmt"
	"os"
	"testing"
)

func TestErrorResultFromFS_DeniedBlockedType(t *testing.T) {
	err := fmt.Errorf("access denied: path is outside the workspace: %w", ErrWorkspaceBoundary)
	result := errorResultFromFS(err)
	if result.BlockedType != BlockedTypeDenied {
		t.Fatalf("expected BlockedTypeDenied for ErrWorkspaceBoundary, got %s", result.BlockedType)
	}
}

func TestErrorResultFromFS_NoBlockedTypeOnNormalError(t *testing.T) {
	err := fmt.Errorf("file not found: %w", os.ErrNotExist)
	result := errorResultFromFS(err)
	if result.BlockedType != "" {
		t.Fatalf("expected no BlockedType for normal error, got %s", result.BlockedType)
	}
}

func TestErrorResultFromFS_NoBlockedTypeOnOSPermission(t *testing.T) {
	err := fmt.Errorf("failed to read file: permission denied: %w", os.ErrPermission)
	result := errorResultFromFS(err)
	if result.BlockedType != "" {
		t.Fatalf("expected no BlockedType for OS permission, got %s", result.BlockedType)
	}
}
