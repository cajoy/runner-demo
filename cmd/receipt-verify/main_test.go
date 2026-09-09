package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/runner-portable-ci-demo/internal/receiptverify"
)

func TestOutputsDefaultToRunningAllChecks(t *testing.T) {
	root := t.TempDir()
	output, summary := filepath.Join(root, "output"), filepath.Join(root, "summary")
	if err := emit(receiptverify.Fallback("receipt_signature_invalid"), output, summary, filepath.Join(root, "evidence.json")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil || string(raw) != "verified=[]\n" {
		t.Fatalf("output=%q err=%v", raw, err)
	}
	raw, err = os.ReadFile(summary)
	if err != nil || !strings.Contains(string(raw), "| build | run | receipt_signature_invalid |") {
		t.Fatalf("summary=%q err=%v", raw, err)
	}
}

func TestEvidenceWriteFailureCannotPublishSkipOutputs(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "output")
	result := receiptverify.Result{Tasks: []receiptverify.Decision{{Task: "lint", Decision: "reuse"}}}
	if err := emit(result, output, "", root); err == nil {
		t.Fatal("expected evidence write failure")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("skip output written before evidence succeeded")
	}
}
