package scenarios

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedLinuxReceiptScenario(t *testing.T) {
	if *runnerBinary == "" || os.Getenv("RUNNER_DEMO_LINUX") != "1" {
		t.Skip("set RUNNER_DEMO_LINUX=1 with a Docker daemon for pinned Linux rehearsal")
	}
	s := newSuite(t)
	name := filepath.Join(s.root, ".local-ci", "api.yaml")
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "runtime: host" || strings.HasPrefix(trimmed, "PATH:") || strings.HasPrefix(trimmed, "RUNNER_DEMO_GO:") || strings.HasPrefix(trimmed, "RUNNER_DEMO_COUNTER:") {
			continue
		}
		lines = append(lines, line)
	}
	workflow := strings.ReplaceAll(strings.Join(lines, "\n"), "golang:1.27.1-bookworm, platform: linux/native", "golang:1.27.1-bookworm@sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b, platform: linux/arm64")
	write(t, name, []byte(workflow))
	policyPath := filepath.Join(s.root, ".runner", "policy.yaml")
	raw, err = os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	var policy map[string]any
	if err = json.Unmarshal(raw, &policy); err != nil {
		t.Fatal(err)
	}
	policy["keys"].(map[string]any)[s.signer].(map[string]any)["platforms"] = []string{"linux/arm64"}
	for _, task := range policy["tasks"].(map[string]any) {
		task.(map[string]any)["platforms"] = []string{"linux/arm64"}
	}
	raw, _ = json.Marshal(policy)
	write(t, policyPath, raw)
	s.git(s.root, "commit", "-qam", "pinned Linux verification")
	s.git(s.root, "branch", "-f", "policy", "HEAD")
	got := s.workflow(s.root, "api:preflight", "--fresh")
	if !got.Proof.Signed {
		t.Fatalf("Linux run did not sign: %+v", got.Proof)
	}
	raw, err = os.ReadFile(got.ReceiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Jobs []struct {
			Runtime struct{ Driver, Platform string } `json:"runtime"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	for _, job := range receipt.Jobs {
		if job.Runtime.Driver != "docker" || job.Runtime.Platform != "linux/arm64" {
			t.Fatalf("wrong actual runtime: %+v", job.Runtime)
		}
	}
	s.expect(s.root, false, "lint", "unit", "build")
	out, errs, code := s.run(s.root, "ci", "finalize", "--project", s.root, "--plan", filepath.Join(s.root, ".runner-ci", "plan.json"), "--evidence-dir", filepath.Join(s.root, ".runner-ci", "evidence"), "--policy", ".runner/policy.yaml", "--policy-ref", "refs/heads/policy", "--receipt-only", "--json")
	if code != 0 || !strings.Contains(string(out), `"receiver_verified":true`) {
		t.Fatalf("Linux finalization: %d %s %s", code, out, errs)
	}
}
