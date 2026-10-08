package handlers

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPythonFileOKRejectsUnixVenvStub(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "python")
	if err := os.WriteFile(stub, []byte("symlink..\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if pythonFileOK(stub) {
		t.Fatal("a tiny non-exe python file must not be selected")
	}
}

func TestFindGuardPythonSkipsBrokenVenv(t *testing.T) {
	got := findGuardPython()
	if runtime.GOOS != "windows" {
		return
	}
	if got == "" {
		t.Fatal("expected a system Python")
	}
	if strings.Contains(strings.ToLower(got), `.venv-guard\bin\python`) {
		t.Fatalf("selected the unix venv stub: %s", got)
	}
	if strings.Contains(strings.ToLower(got), `\windowsapps\`) {
		t.Fatalf("selected the Windows Store alias: %s", got)
	}
}
