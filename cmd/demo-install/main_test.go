package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallRejectsTamperingAndPreservesExistingBinary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "tampered") }))
	defer server.Close()
	name := filepath.Join(t.TempDir(), "runner")
	if err := os.WriteFile(name, []byte("old"), 0700); err != nil {
		t.Fatal(err)
	}
	p := pin{version: "v1.2.3", file: "runner_v1.2.3_linux_amd64", sha: fmt.Sprintf("%x", sha256.Sum256([]byte("trusted")))}
	if err := install(context.Background(), server.Client(), server.URL, name, p); err == nil {
		t.Fatal("tampered download installed")
	}
	if raw, err := os.ReadFile(name); err != nil || string(raw) != "old" {
		t.Fatalf("existing binary changed: %q %v", raw, err)
	}
}

func TestReadRepositoryPin(t *testing.T) {
	raw, err := os.ReadFile("../../.local-ci/toolchain.lock")
	if err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"darwin-arm64", "linux-amd64", "linux-arm64"} {
		p, err := readPin(string(raw), platform)
		if err != nil || !strings.HasPrefix(p.version, "v") {
			t.Fatalf("%s: %+v %v", platform, p, err)
		}
	}
}
