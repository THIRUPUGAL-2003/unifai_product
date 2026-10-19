package handlers

import (
	"strings"
	"testing"
)

func TestDemoPlainLineStripsHeaderBreaks(t *testing.T) {
	got := demoPlainLine("Ada\r\nBcc: x@y.com", 120)
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("newline remained: %q", got)
	}
	if got != "Ada Bcc: x@y.com" {
		t.Fatalf("got %q", got)
	}
}

func TestDemoNotesKeepsLineBreaksAndDropsControls(t *testing.T) {
	got := demoNotes("line\r\n two\x00", 2000)
	if strings.Contains(got, "\r") || strings.Contains(got, "\x00") {
		t.Fatalf("control char remained: %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected a newline, got %q", got)
	}
}

func TestDemoEmailCheckRejectsBareAt(t *testing.T) {
	if isValidEmail("a@b") {
		t.Fatal("expected a@b to be rejected")
	}
	if !isValidEmail("jane@company.com") {
		t.Fatal("expected jane@company.com to be accepted")
	}
}
