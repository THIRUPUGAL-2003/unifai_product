package handlers

import "testing"

func TestLoopbackPACProxy(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:18103":     true,
		"127.0.0.1:8003":      true,
		"localhost:18103":     true,
		"[::1]:18103":         true,
		"":                    false,
		"127.0.0.1":           false,
		"127.0.0.1:0":         false,
		"127.0.0.1:70000":     false,
		"127.0.0.1:abc":       false,
		"10.0.0.5:8080":       false,
		"evil.example:80":     false,
		"127.0.0.1.nip.io:80": false,
	}
	for addr, want := range cases {
		if got := loopbackPACProxy(addr); got != want {
			t.Errorf("loopbackPACProxy(%q) = %v, want %v", addr, got, want)
		}
	}
}
