package receiptverify

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"sort"
	"strings"
	"time"
)

const maxNoteBytes = 1 << 20

func decode(raw []byte, value any) error {
	if len(raw) == 0 || len(raw) > maxNoteBytes {
		return errors.New("receipt_size_invalid")
	}
	// Reject duplicate keys as well as unknown fields and trailing JSON.
	tokens := json.NewDecoder(bytes.NewReader(raw))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 32 {
			return errors.New("receipt_json_depth")
		}
		token, err := tokens.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				seen := map[string]bool{}
				for tokens.More() {
					key, err := tokens.Token()
					if err != nil {
						return err
					}
					name, ok := key.(string)
					if !ok || seen[name] {
						return errors.New("receipt_duplicate_key")
					}
					seen[name] = true
					if err := walk(depth + 1); err != nil {
						return err
					}
				}
			case '[':
				for tokens.More() {
					if err := walk(depth + 1); err != nil {
						return err
					}
				}
			default:
				return errors.New("receipt_json_invalid")
			}
			_, err = tokens.Token()
		}
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return errors.New("receipt_trailing_json")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(value)
}

func LoadPolicy(raw []byte) (Policy, error) {
	var p Policy
	if err := decode(raw, &p); err != nil {
		return p, errors.New("receipt_policy_invalid")
	}
	if err := p.validate(); err != nil {
		return p, err
	}
	return p, nil
}
func (p Policy) validate() error {
	age, err := time.ParseDuration(p.Reuse.MaxAge)
	if p.Schema != "runner-receipt-policy/v2" || err != nil || age <= 0 || !slices.Equal(p.Reuse.Effects, []string{"none"}) || (p.OnIntegrityConflict != "fail" && p.OnIntegrityConflict != "quarantine") {
		return errors.New("receipt_policy_invalid")
	}
	for _, k := range p.Keys {
		key, err := base64.StdEncoding.Strict().DecodeString(k.PublicKey)
		if err != nil || len(key) != ed25519.PublicKeySize || len(k.Repositories) == 0 || len(k.Placements) == 0 || len(k.Platforms) == 0 || (!k.NotBefore.IsZero() && !k.NotAfter.IsZero() && !k.NotBefore.Before(k.NotAfter)) {
			return errors.New("receipt_policy_key_invalid")
		}
	}
	for pattern, b := range p.Branches {
		if _, err := path.Match(pattern, "main"); err != nil || pattern == "" || (b.OnValid != "reuse" && b.OnValid != "rerun_all" && b.OnValid != "spot_check") || b.Rate < 0 || b.Rate > 100 {
			return errors.New("receipt_policy_branch_invalid")
		}
	}
	return nil
}
func (p Policy) branchMode(branch string) string {
	if exact, ok := p.Branches[branch]; ok {
		return exact.OnValid
	}
	patterns := []string{}
	for pattern := range p.Branches {
		patterns = append(patterns, pattern)
	}
	sort.Slice(patterns, func(i, j int) bool {
		if len(patterns[i]) != len(patterns[j]) {
			return len(patterns[i]) > len(patterns[j])
		}
		return patterns[i] < patterns[j]
	})
	for _, pattern := range patterns {
		if ok, _ := path.Match(pattern, branch); ok {
			return p.Branches[pattern].OnValid
		}
	}
	return "reuse"
}

func Fallback(reason string) Result {
	r := Result{}
	for _, name := range TaskNames {
		r.Tasks = append(r.Tasks, Decision{Task: name, Decision: "run", Reason: reason})
	}
	return r
}
func (r Result) Verified() []string {
	result := []string{}
	for _, task := range r.Tasks {
		if task.Decision == "reuse" {
			result = append(result, task.Task)
		}
	}
	return result
}
func (r Result) Markdown() string {
	var b strings.Builder
	b.WriteString("## Receipt verification\n\nPlatform: `linux/arm64`, pinned Go 1.27.1 image.\n\n")
	b.WriteString("| Task | Decision | Reason | Signer | Original run | Receipt |\n| --- | --- | --- | --- | --- | --- |\n")
	cell := func(s string) string {
		return strings.NewReplacer("|", "&#124;", "\n", " ", "\r", " ", "`", "", "<", "&lt;", ">", "&gt;").Replace(s)
	}
	for _, t := range r.Tasks {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", t.Task, t.Decision, cell(t.Reason), cell(t.Signer), cell(t.Run), cell(t.Receipt))
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(&b, "\nReceipt rejected: %s.\n", cell(warning))
	}
	b.WriteString("\nA reuse decision verifies previous proof. Each run decision executes the ordinary workflow step. Build artifacts are not transferred by receipts.\n")
	return b.String()
}
func pae(payloadType string, raw []byte) []byte {
	return append([]byte(fmt.Sprintf("DSSEv1 %d %s %d ", len(payloadType), payloadType, len(raw))), raw...)
}
func validHash(value string) bool {
	if !strings.HasPrefix(value, "sha256:") {
		return false
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(raw) == 32 && value == strings.ToLower(value)
}
func validOID(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 20 && value == strings.ToLower(value)
}

// Verify grants skips only for original, trusted, compatible executions.
// All integrity errors discard reuse and let the normal checks establish fresh
// evidence. This receiver never treats a rejected receipt as a passing check.
func Verify(notes []Note, p Policy, expected Expected, now time.Time) Result {
	result := Fallback("no_verified_receipt")
	if err := p.validate(); err != nil {
		return Fallback(err.Error())
	}
	if !validOID(expected.Commit) || !validOID(expected.Tree) || !validHash(expected.FilesDigest) {
		return Fallback("source_identity_unavailable")
	}
	if mode := p.branchMode(expected.Branch); mode != "reuse" {
		return Fallback("branch_policy_" + mode)
	}
	age, _ := time.ParseDuration(p.Reuse.MaxAge)
	states, runs := map[string]string{}, map[string]string{}
	count, total := 0, 0
	for _, note := range notes {
		total += len(note.Raw)
		if len(notes) > 1024 || total > 16<<20 {
			return Fallback("receipt_lookup_limit")
		}
		var index Index
		if err := decode(note.Raw, &index); err != nil || index.Schema != IndexSchema || !validOID(note.Commit) || index.Commit != note.Commit || len(index.Entries) > 128 {
			return Fallback("receipt_index_invalid")
		}
		for _, entry := range index.Entries {
			count++
			if count > 1024 {
				return Fallback("receipt_lookup_limit")
			}
			e := entry.Envelope
			if e.PayloadType != PayloadType {
				result.Warnings = append(result.Warnings, "receipt_version_unsupported")
				continue
			}
			if len(e.Signatures) != 1 {
				return Fallback("receipt_envelope_invalid")
			}
			raw, err := base64.StdEncoding.Strict().DecodeString(e.Payload)
			if err != nil || entry.ReceiptDigest != hashBytes(raw) {
				return Fallback("receipt_digest_invalid")
			}
			var r Receipt
			if err := decode(raw, &r); err != nil || r.Schema != ReceiptSchema || r.Subject.Commit != note.Commit || !validOID(r.Subject.Tree) || !validHash(r.SourceReceipt.SHA256) || r.SourceReceipt.RunID == "" || r.CreatedAt.IsZero() || len(r.Tasks) == 0 {
				return Fallback("receipt_payload_invalid")
			}
			k, ok := p.Keys[e.Signatures[0].KeyID]
			if !ok {
				result.Warnings = append(result.Warnings, "receipt_signer_untrusted")
				continue
			}
			key, _ := base64.StdEncoding.Strict().DecodeString(k.PublicKey)
			sig, err := base64.StdEncoding.Strict().DecodeString(e.Signatures[0].Sig)
			if err != nil || !ed25519.Verify(key, pae(e.PayloadType, raw), sig) {
				return Fallback("receipt_signature_invalid")
			}
			if r.Subject.RepositoryID != RepositoryID || r.Workflow.Service != "api-linux" || r.Workflow.ConfigDigest != ConfigDigest || (r.Subject.Commit == expected.Commit && r.Subject.Tree != expected.Tree) {
				result.Warnings = append(result.Warnings, "subject_or_workflow_mismatch")
				continue
			}
			if now.Sub(r.CreatedAt) > age || r.CreatedAt.After(now.Add(5*time.Minute)) || (!k.NotBefore.IsZero() && (r.CreatedAt.Before(k.NotBefore) || now.Before(k.NotBefore))) || (!k.NotAfter.IsZero() && (r.CreatedAt.After(k.NotAfter) || !now.Before(k.NotAfter))) {
				result.Warnings = append(result.Warnings, "receipt_or_signer_expired")
				continue
			}
			if !slices.Contains(k.Repositories, RepositoryID) || !slices.Contains(k.Placements, r.Producer.Placement) || r.Terminal.MutationStarted || r.Terminal.AuthorityRequired {
				result.Warnings = append(result.Warnings, "receipt_scope_denied")
				continue
			}
			if previous, ok := runs[r.SourceReceipt.RunID]; ok && previous != r.SourceReceipt.SHA256 {
				return Fallback("receipt_conflicting_source_run")
			}
			runs[r.SourceReceipt.RunID] = r.SourceReceipt.SHA256
			seen := map[string]bool{}
			for _, task := range r.Tasks {
				if seen[task.ID] {
					return Fallback("receipt_duplicate_task")
				}
				seen[task.ID] = true
				pos := slices.Index(TaskNames, task.ID)
				if pos == -1 {
					continue
				}
				want := identity(task.ID, expected.FilesDigest)
				reason := taskReason(task, p, k, want)
				if reason == "" || reason == "outcome_not_passed" {
					if previous, ok := states[task.ID]; ok && previous != task.State {
						return Fallback("receipt_conflicting_outcomes")
					}
					states[task.ID] = task.State
				}
				if reason != "" {
					if result.Tasks[pos].Decision != "reuse" {
						result.Tasks[pos].Reason = reason
					}
					continue
				}
				if result.Tasks[pos].Decision == "reuse" && !r.CreatedAt.After(result.Tasks[pos].CreatedAt) {
					continue
				}
				actor := ""
				if r.Producer.Actor != nil {
					actor = r.Producer.Actor.Kind + "/" + r.Producer.Actor.Source
				}
				result.Tasks[pos] = Decision{Task: task.ID, Decision: "reuse", Reason: "verified_content_receipt", Receipt: entry.ReceiptDigest, Run: r.SourceReceipt.RunID, Commit: r.Subject.Commit, Signer: e.Signatures[0].KeyID, Actor: actor, CreatedAt: r.CreatedAt}
			}
		}
	}
	sort.Strings(result.Warnings)
	result.Warnings = slices.Compact(result.Warnings)
	return result
}

func taskReason(t Task, p Policy, key KeyPolicy, want Identity) string {
	allowed, ok := p.Tasks[t.ID]
	if !ok || allowed.Verification.Kind != "go" || !slices.Contains(allowed.Services, "api-linux") || contractDigest(allowed.Verification) != want.ContractDigest {
		return "verification_contract_denied"
	}
	if (len(key.Tasks) > 0 && !slices.Contains(key.Tasks, t.ID)) || !slices.Contains(key.Platforms, t.Platform) || !slices.Contains(allowed.Platforms, t.Platform) {
		return "signer_or_task_scope_denied"
	}
	if t.Effect != "none" || t.Decision != "executed" || t.SourceReceipt != "" {
		return "original_verification_required"
	}
	if t.Content == nil || t.Content.Digest != inputDigest(*t.Content) || t.InputDigest != t.Content.Digest || !validHash(t.ExecutionDigest) {
		return "receipt_identity_invalid"
	}
	if t.Platform != want.Runtime.Platform || t.RuntimeImageDigest != want.Runtime.ImageDigest || t.Content.Runtime != want.Runtime {
		return "runtime_changed"
	}
	if t.Content.ContractDigest != want.ContractDigest || t.Content.DefinitionDigest != want.DefinitionDigest || t.Content.EnvironmentDigest != want.EnvironmentDigest {
		return "configuration_changed"
	}
	if t.Content.FilesDigest != want.FilesDigest {
		return "task_inputs_changed"
	}
	if t.Content.Digest != want.Digest {
		return "task_identity_changed"
	}
	if t.State != "passed" {
		return "outcome_not_passed"
	}
	return ""
}
