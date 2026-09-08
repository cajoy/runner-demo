package scenarios

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

var runnerBinary = flag.String("runner", "", "absolute Runner binary to exercise; empty skips this opt-in suite")

const notesRef = "refs/notes/runner-receipts"
const fixtureOrigin = "https://fixture.invalid/runner-demo.git"

// The proxy counts real Go invocations, then runs the installed Go toolchain.
// Runner also fingerprints this executable as part of the fixture's runtime.
func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == "go" {
		if len(os.Args) > 1 && (os.Args[1] == "vet" || os.Args[1] == "test" || os.Args[1] == "build") {
			f, err := os.OpenFile(os.Getenv("RUNNER_DEMO_COUNTER"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			_, err = fmt.Fprintln(f, os.Args[1])
			f.Close()
			if err != nil {
				os.Exit(2)
			}
		}
		command := exec.Command(os.Getenv("RUNNER_DEMO_GO"), os.Args[1:]...)
		command.Stdin = os.Stdin
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			if code, ok := err.(*exec.ExitError); ok {
				os.Exit(code.ExitCode())
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	}
	if len(os.Args) > 2 && os.Args[1] == "--record-hook" {
		raw, _ := io.ReadAll(os.Stdin)
		record, _ := json.Marshal(struct {
			Args  []string
			Input string
		}{os.Args[3:], string(raw)})
		if err := os.WriteFile(os.Args[2], record, 0600); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	flag.Parse()
	os.Exit(m.Run())
}

type signature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}
type envelope struct {
	PayloadType string      `json:"payloadType"`
	Payload     string      `json:"payload"`
	Signatures  []signature `json:"signatures"`
}
type entry struct {
	Digest   string   `json:"receipt_digest"`
	Envelope envelope `json:"envelope"`
}
type index struct {
	Schema  string  `json:"schema"`
	Commit  string  `json:"commit"`
	Entries []entry `json:"entries"`
}
type taskPlan struct {
	ID            string `json:"id"`
	Decision      string `json:"decision"`
	Reason        string `json:"reason"`
	SourceRunID   string `json:"source_run_id"`
	SourceReceipt string `json:"source_receipt"`
}
type plan struct {
	Tasks []taskPlan `json:"tasks"`
}
type job struct {
	Name        string            `json:"name"`
	Decision    string            `json:"decision"`
	SourceRunID string            `json:"source_run_id"`
	Steps       []json.RawMessage `json:"steps"`
	Artifacts   []struct {
		Archive string `json:"archive"`
	} `json:"artifacts"`
}
type report struct {
	RunID       string                        `json:"run_id"`
	State       string                        `json:"state"`
	ProjectID   string                        `json:"project_id"`
	ReceiptPath string                        `json:"receipt_path"`
	Jobs        []job                         `json:"jobs"`
	Actor       struct{ Kind, Source string } `json:"actor"`
	Proof       struct {
		State  string `json:"state"`
		Signed bool   `json:"signed"`
	} `json:"receipt_status"`
}

type suite struct {
	t                                                    *testing.T
	base, root, remote, commit, counter, signer, hookLog string
	binary                                               string
	private                                              ed25519.PrivateKey
	env                                                  []string
	note                                                 index
	first                                                report
}

func command(t *testing.T, root string, env []string, name string, args ...string) ([]byte, []byte, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 45 * time.Second
	cmd.Dir = root
	cmd.Env = env
	var out, errs bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errs
	err := cmd.Run()
	code := 0
	if err != nil {
		code = 2
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		}
	}
	return out.Bytes(), errs.Bytes(), code
}
func (s *suite) git(root string, args ...string) string {
	s.t.Helper()
	out, errs, code := command(s.t, root, s.env, "git", args...)
	if code != 0 {
		s.t.Fatalf("git %v: exit=%d %s %s", args, code, out, errs)
	}
	return strings.TrimSpace(string(out))
}
func (s *suite) run(root string, args ...string) ([]byte, []byte, int) {
	s.t.Helper()
	return command(s.t, root, s.env, s.binary, args...)
}
func write(t *testing.T, name string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, raw, 0600); err != nil {
		t.Fatal(err)
	}
}
func quoted(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func newSuite(t *testing.T) *suite {
	t.Helper()
	binary, err := filepath.Abs(*runnerBinary)
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(base, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(self, filepath.Join(bin, "go")); err != nil {
		t.Fatal(err)
	}
	s := &suite{t: t, base: base, root: filepath.Join(base, "repo"), remote: filepath.Join(base, "remote.git"), counter: filepath.Join(base, "executed.txt"), binary: binary, private: key, signer: fmt.Sprintf("fixture-%x", pub[:6]), hookLog: filepath.Join(base, "previous-hook.json")}
	s.env = append(os.Environ(), "XDG_DATA_HOME="+filepath.Join(base, "xdg"), "RUNNER_RECEIPT_SIGNER_ID="+s.signer, "RUNNER_RECEIPT_SIGNING_KEY="+base64.StdEncoding.EncodeToString(key.Seed()), "RUNNER_APPROVAL_MODE=required")
	if err := os.MkdirAll(s.root, 0700); err != nil {
		t.Fatal(err)
	}
	s.git(s.root, "init", "-b", "main")
	s.git(s.root, "config", "user.name", "Runner receipt tests")
	s.git(s.root, "config", "user.email", "runner-tests@example.invalid")
	s.git(s.root, "init", "--bare", "-b", "main", s.remote)
	s.configureRemote(s.root)
	repoID := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("runner.repository.v1\x00fixture.invalid/runner-demo")))
	platform := runtime.GOOS + "/" + runtime.GOARCH
	policy := map[string]any{"schema": "runner-receipt-policy/v2", "keys": map[string]any{s.signer: map[string]any{"public_key": base64.StdEncoding.EncodeToString(pub), "repositories": []string{repoID}, "tasks": []string{"lint", "unit", "build"}, "placements": []string{"local", "ci"}, "platforms": []string{platform}}}, "reuse": map[string]any{"effects": []string{"none"}, "max_age": "72h"}, "on_integrity_conflict": "fail"}
	tasks := map[string]any{}
	for _, name := range []string{"lint", "unit", "build"} {
		tasks[name] = map[string]any{"services": []string{"api"}, "platforms": []string{platform}, "verification": map[string]any{"kind": "go", "exclude": []string{"README.md"}}}
	}
	policy["tasks"] = tasks
	policyRaw, _ := json.MarshalIndent(policy, "", "  ")
	workflow := fmt.Sprintf(`schema: local-ci/v2
service: api
default: preflight
runtime: {image: golang:1.27.1-bookworm, platform: linux/native}
env:
  GOENV: off
  GOWORK: off
  GOTOOLCHAIN: local
  CGO_ENABLED: "0"
  GOFLAGS: "-buildvcs=false"
  PATH: %q
  RUNNER_DEMO_GO: %q
  RUNNER_DEMO_COUNTER: %q
tasks:
  lint:
    runtime: host
    verification: {kind: go, exclude: [README.md]}
    run: go vet ./...
  unit:
    runtime: host
    verification: {kind: go, exclude: [README.md]}
    run: go test -count=1 ./...
  build:
    runtime: host
    verification: {kind: go, exclude: [README.md]}
    run: go build -o out/server .
    artifacts: {server: {path: out/server}}
  deploy:
    runtime: host
    placement: local
    needs: [lint, unit, build]
    run: ./out/server
entrypoints:
  preflight: [lint, unit, build]
  deploy: [deploy]
`, bin+string(os.PathListSeparator)+os.Getenv("PATH"), realGo, s.counter)
	files := map[string]string{"README.md": "before\n", "go.mod": "module example.com/receipt-fixture\n\ngo 1.27\n", "main.go": "package main\nimport \"fmt\"\nfunc main(){fmt.Println(\"simulated deploy: verified artifact executed\")}\n", "main_test.go": "package main\nimport \"testing\"\nfunc TestPass(t *testing.T){}\n", ".gitignore": ".local-ci/state/\n.runner-ci/\nout/\n", ".local-ci/api.yaml": workflow, ".local-ci/runner.yaml": "schema: local-ci/v1\nsource: auto\nexecutors:\n  local: {driver: docker, allowed_drivers: [docker, apple-container]}\nconcurrency: {max: 3, on_battery: 3}\nstore: {path: auto}\nplugins: {}\n", ".runner/policy.yaml": string(policyRaw)}
	for name, value := range files {
		write(t, filepath.Join(s.root, filepath.FromSlash(name)), []byte(value))
	}
	s.git(s.root, "add", ".")
	s.git(s.root, "commit", "-qm", "Fixture inputs")
	s.commit = s.git(s.root, "rev-parse", "HEAD")
	s.git(s.root, "branch", "policy")
	hook := filepath.Join(s.root, ".git", "hooks", "pre-push")
	write(t, hook, []byte("#!/bin/sh\nexec "+quoted(self)+" --record-hook "+quoted(s.hookLog)+" \"$@\"\n"))
	if err := os.Chmod(hook, 0700); err != nil {
		t.Fatal(err)
	}
	s.setup(s.root)
	s.git(s.root, "push") // Code publication before proof is valid.
	s.resetCounters()
	s.first = s.workflow(s.root, "api:preflight")
	s.assertCounters(map[string]int{"vet": 1, "test": 1, "build": 1})
	if !s.first.Proof.Signed || s.first.Proof.State != "pending_delivery" {
		t.Fatalf("automatic signing missing: %+v", s.first.Proof)
	}
	raw := s.git(s.root, "notes", "--ref="+notesRef, "show", "HEAD")
	if err := json.Unmarshal([]byte(raw), &s.note); err != nil {
		t.Fatal(err)
	}
	if len(s.note.Entries) != 1 {
		t.Fatalf("initial proof entries=%d", len(s.note.Entries))
	}
	return s
}

func (s *suite) configureRemote(root string) {
	s.git(root, "remote", "add", "origin", fixtureOrigin)
	s.git(root, "config", "url.file://"+s.remote+".insteadOf", fixtureOrigin)
}

func (s *suite) setup(root string) {
	s.t.Helper()
	out, errs, code := s.run(root, "receipts", "setup", "--project", root, "--signer", s.signer, "--policy", ".runner/policy.yaml", "--policy-ref", "refs/heads/policy")
	if code != 0 {
		s.t.Fatalf("setup: %d %s %s", code, out, errs)
	}
	// Guard observers are detached and bounded. Let them finish before
	// TempDir removes their fixture, so cleanup cannot race a status write.
	owner := s.t
	owner.Cleanup(func() {
		time.Sleep(250 * time.Millisecond)
		file, err := os.OpenFile(filepath.Join(root, ".git", "runner-receipts.json.observer.lock"), os.O_RDWR, 0600)
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			owner.Error(err)
			return
		}
		defer file.Close()
		deadline := time.Now().Add(35 * time.Second)
		for {
			err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			if err == nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				return
			}
			if time.Now().After(deadline) {
				owner.Error("delivery observer did not stop")
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
}
func (s *suite) workflow(root, selector string, extra ...string) report {
	s.t.Helper()
	args := append([]string{"run", "--project", root, "--actor", "agent", "--json"}, extra...)
	args = append(args, selector)
	out, errs, code := s.run(root, args...)
	if code != 0 {
		var failed report
		if json.Unmarshal(out, &failed) == nil && failed.ReceiptPath != "" {
			directory := filepath.Dir(failed.ReceiptPath)
			entries, _ := os.ReadDir(directory)
			for _, entry := range entries {
				if strings.Contains(entry.Name(), "progress") || entry.Name() == "run.json" {
					raw, _ := os.ReadFile(filepath.Join(directory, entry.Name()))
					s.t.Logf("%s: %s", entry.Name(), raw)
				}
			}
		}
		s.t.Fatalf("workflow %s: %d %s %s", selector, code, out, errs)
	}
	var result report
	if err := json.Unmarshal(out, &result); err != nil {
		s.t.Fatal(err)
	}
	return result
}
func (s *suite) resetCounters() { write(s.t, s.counter, nil) }
func (s *suite) assertCounters(want map[string]int) {
	s.t.Helper()
	raw, err := os.ReadFile(s.counter)
	if err != nil {
		s.t.Fatal(err)
	}
	got := map[string]int{}
	for _, line := range strings.Fields(string(raw)) {
		got[line]++
	}
	for _, name := range []string{"vet", "test", "build"} {
		if got[name] != want[name] {
			s.t.Fatalf("actual %s invocations=%d want=%d; all=%v", name, got[name], want[name], got)
		}
	}
}

func (s *suite) clone(t *testing.T, proof bool) string {
	t.Helper()
	s.t = t
	root := filepath.Join(t.TempDir(), "clone")
	s.git(s.root, "clone", s.root, root)
	s.git(root, "config", "user.name", "Runner receipt tests")
	s.git(root, "config", "user.email", "runner-tests@example.invalid")
	s.git(root, "remote", "set-url", "origin", fixtureOrigin)
	s.git(root, "config", "url.file://"+s.remote+".insteadOf", fixtureOrigin)
	s.git(root, "branch", "policy", s.commit)
	if proof {
		s.writeIndex(root, s.copyNote())
	}
	return root
}
func (s *suite) writeIndex(root string, note index) {
	s.t.Helper()
	raw, _ := json.Marshal(note)
	s.writeNote(root, raw)
}

func (s *suite) copyNote() index {
	raw, _ := json.Marshal(s.note)
	var result index
	if err := json.Unmarshal(raw, &result); err != nil {
		s.t.Fatal(err)
	}
	return result
}
func (s *suite) writeNote(root string, raw []byte) {
	s.t.Helper()
	file := filepath.Join(root, ".runner-ci", "fixture-note.json")
	write(s.t, file, raw)
	s.git(root, "notes", "--ref="+notesRef, "add", "-f", "-F", file, s.commit)
}
func (s *suite) payload() map[string]any {
	raw, err := base64.StdEncoding.DecodeString(s.note.Entries[0].Envelope.Payload)
	if err != nil {
		s.t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		s.t.Fatal(err)
	}
	return value
}
func (s *suite) signed(value map[string]any) entry {
	raw, err := json.Marshal(value)
	if err != nil {
		s.t.Fatal(err)
	}
	kind := s.note.Entries[0].Envelope.PayloadType
	pae := append([]byte(fmt.Sprintf("DSSEv1 %d %s %d ", len(kind), kind, len(raw))), raw...)
	return entry{Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(raw)), Envelope: envelope{PayloadType: kind, Payload: base64.StdEncoding.EncodeToString(raw), Signatures: []signature{{KeyID: s.signer, Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(s.private, pae))}}}}
}
func (s *suite) mutate(root string, mutator func(map[string]any)) {
	value := s.payload()
	mutator(value)
	note := s.note
	note.Entries = []entry{s.signed(value)}
	s.writeIndex(root, note)
}
func (s *suite) plan(root string, fetch bool) (plan, string, int) {
	s.t.Helper()
	out, errs, code := s.run(root, "ci", "plan", "api:preflight", "--project", root, "--policy", ".runner/policy.yaml", "--policy-ref", "refs/heads/policy", "--branch", "feature", "--fetch="+fmt.Sprint(fetch), "--output", filepath.Join(root, ".runner-ci", "plan.json"), "--github-summary", filepath.Join(root, ".runner-ci", "summary.md"), "--json")
	var result plan
	if code == 0 {
		if err := json.Unmarshal(out, &result); err != nil {
			s.t.Fatal(err)
		}
	}
	return result, string(errs), code
}
func (s *suite) expect(root string, fetch bool, reused ...string) {
	s.t.Helper()
	p, errs, code := s.plan(root, fetch)
	if code != 0 {
		s.t.Fatalf("plan: %d %s", code, errs)
	}
	got := []string{}
	for _, task := range p.Tasks {
		if task.Decision == "reused" {
			got = append(got, task.ID)
		}
	}
	sort.Strings(got)
	sort.Strings(reused)
	if strings.Join(got, ",") != strings.Join(reused, ",") {
		s.t.Fatalf("reused=%v want=%v tasks=%+v", got, reused, p.Tasks)
	}
}

func TestReceiptScenarios(t *testing.T) {
	if *runnerBinary == "" {
		t.Skip("opt in with test-cases/run.sh or -args -runner /absolute/path/to/runner")
	}
	s := newSuite(t)
	t.Run("no-receipt", func(t *testing.T) { s.expect(s.clone(t, false), false) })
	t.Run("full-receipt", func(t *testing.T) { s.expect(s.clone(t, true), false, "lint", "unit", "build") })
	t.Run("partial", func(t *testing.T) {
		root := s.clone(t, true)
		s.mutate(root, func(value map[string]any) {
			kept := []any{}
			for _, task := range value["tasks"].([]any) {
				if task.(map[string]any)["id"] != "build" {
					kept = append(kept, task)
				}
			}
			value["tasks"] = kept
		})
		s.expect(root, false, "lint", "unit")
	})
	t.Run("failed-task", func(t *testing.T) {
		root := s.clone(t, true)
		s.mutate(root, func(value map[string]any) {
			for _, task := range value["tasks"].([]any) {
				if task.(map[string]any)["id"] == "unit" {
					task.(map[string]any)["state"] = "failed"
				}
			}
		})
		s.expect(root, false, "lint", "build")
	})
	t.Run("empty-index", func(t *testing.T) {
		root := s.clone(t, true)
		note := s.copyNote()
		note.Entries = []entry{}
		s.writeIndex(root, note)
		s.expect(root, false)
	})
	t.Run("not-pushed", func(t *testing.T) { s.expect(s.clone(t, false), true) })
	t.Run("stale-commit", func(t *testing.T) {
		root := s.clone(t, true)
		s.mutate(root, func(value map[string]any) { value["subject"].(map[string]any)["commit"] = strings.Repeat("a", 40) })
		if _, errs, code := s.plan(root, false); code == 0 {
			t.Fatal("stale subject accepted")
		} else {
			t.Log(errs)
		}
	})
	t.Run("unknown-signer", func(t *testing.T) {
		root := s.clone(t, true)
		note := s.copyNote()
		raw, _ := json.Marshal(note)
		_ = json.Unmarshal(raw, &note)
		note.Entries[0].Envelope.Signatures[0].KeyID = "unknown"
		s.writeIndex(root, note)
		s.expect(root, false)
	})
	t.Run("wrong-platform", func(t *testing.T) {
		root := s.clone(t, true)
		s.mutate(root, func(value map[string]any) {
			for _, task := range value["tasks"].([]any) {
				task.(map[string]any)["platform"] = "other/other"
			}
		})
		s.expect(root, false)
	})
	t.Run("config-drift", func(t *testing.T) {
		root := s.clone(t, true)
		name := filepath.Join(root, ".local-ci", "api.yaml")
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		write(t, name, bytes.ReplaceAll(raw, []byte("-buildvcs=false"), []byte("-buildvcs=false -trimpath")))
		s.git(root, "commit", "-qam", "Change build flags")
		s.expect(root, false)
	})
	t.Run("other-workflow", func(t *testing.T) {
		root := s.clone(t, true)
		s.mutate(root, func(value map[string]any) { value["workflow"].(map[string]any)["service"] = "other" })
		s.expect(root, false)
	})
	t.Run("corrupt-note", func(t *testing.T) {
		root := s.clone(t, true)
		s.writeNote(root, []byte("not JSON"))
		if _, _, code := s.plan(root, false); code == 0 {
			t.Fatal("corrupt note accepted")
		}
	})
	t.Run("multi-entry", func(t *testing.T) {
		root := s.clone(t, true)
		note := s.copyNote()
		note.Entries = nil
		for _, task := range s.payload()["tasks"].([]any) {
			value := s.payload()
			value["tasks"] = []any{task}
			note.Entries = append(note.Entries, s.signed(value))
		}
		s.writeIndex(root, note)
		s.expect(root, false, "lint", "unit", "build")
		raw, err := os.ReadFile(filepath.Join(root, ".runner-ci", "summary.md"))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"lint", "unit", "build"} {
			if !strings.Contains(string(raw), "| "+name+" |") {
				t.Fatalf("summary omitted %s", name)
			}
		}
	})
	t.Run("pushed-later", func(t *testing.T) {
		root := s.clone(t, false)
		s.expect(root, true)
		s.git(s.root, "push")
		s.expect(root, true, "lint", "unit", "build")
		if _, err := os.Stat(s.hookLog); err != nil {
			t.Fatal("previous pre-push hook did not run")
		}
	})
	t.Run("preflight-to-deploy", func(t *testing.T) {
		s.t = t
		s.resetCounters()
		result := s.workflow(s.root, "api:deploy")
		s.assertCounters(map[string]int{})
		for _, job := range result.Jobs {
			if job.Name == "deploy" {
				if len(job.Steps) != 1 {
					t.Fatal("deployment simulation was skipped")
				}
			} else if job.Decision != "reused" || job.SourceRunID != s.first.RunID {
				t.Fatalf("lost original reuse: %+v", job)
			}
		}
	})
	t.Run("readme-only-commit", func(t *testing.T) {
		s.t = t
		write(t, filepath.Join(s.root, "README.md"), []byte("after\n"))
		s.git(s.root, "commit", "-qam", "README only")
		s.resetCounters()
		result := s.workflow(s.root, "api:preflight")
		s.assertCounters(map[string]int{})
		for _, job := range result.Jobs {
			if job.SourceRunID != s.first.RunID {
				t.Fatal("original proof provenance was rewritten")
			}
		}
	})
}
