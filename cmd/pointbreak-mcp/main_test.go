package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindWorkspaceWalksUp(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "orders-svc", "internal", "usecase")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findWorkspace(deep); got != deep {
		t.Errorf("no context/ anywhere: want fallback %s, got %s", deep, got)
	}
	if err := os.MkdirAll(filepath.Join(root, "context"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "context", "README.md"), []byte("# rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findWorkspace(deep); got != root {
		t.Errorf("want workspace %s, got %s", root, got)
	}
}
