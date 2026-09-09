// Package receiptverify is the demo's standalone, read-only receipt receiver.
// It uses the Go standard library and does not invoke or import Runner.
package receiptverify

import "time"

const (
	ReceiptSchema = "runner-portable-receipt/v3"
	IndexSchema   = "runner-commit-receipt-index/v1"
	PayloadType   = "application/vnd.cajoy.runner.portable-receipt+json;version=3"
	NotesRef      = "refs/notes/runner-receipts"
)

type Runtime struct {
	Driver          string `json:"driver"`
	Platform        string `json:"platform"`
	ImageDigest     string `json:"image_digest,omitempty"`
	ToolchainDigest string `json:"toolchain_digest,omitempty"`
}

// Field order and omitempty are part of Runner's v2 identity hashing contract.
type Identity struct {
	Schema            string   `json:"schema"`
	RepositoryID      string   `json:"repository_id"`
	Service           string   `json:"service"`
	Task              string   `json:"task"`
	ContractDigest    string   `json:"contract_digest"`
	FilesDigest       string   `json:"files_digest"`
	DefinitionDigest  string   `json:"definition_digest"`
	EnvironmentDigest string   `json:"environment_digest"`
	Runtime           Runtime  `json:"runtime"`
	DependencyOutputs []string `json:"dependency_outputs,omitempty"`
	Digest            string   `json:"digest"`
}

type Receipt struct {
	Schema        string `json:"schema"`
	SourceReceipt struct {
		Schema string `json:"schema"`
		RunID  string `json:"run_id"`
		SHA256 string `json:"sha256"`
	} `json:"source_receipt"`
	Subject struct {
		RepositoryID string `json:"repository_id"`
		Commit       string `json:"commit"`
		Tree         string `json:"tree"`
	} `json:"subject"`
	Workflow struct {
		Service      string `json:"service"`
		Entrypoint   string `json:"entrypoint"`
		ConfigDigest string `json:"config_digest"`
	} `json:"workflow"`
	Terminal struct {
		Status            string `json:"status"`
		MutationStarted   bool   `json:"mutation_started"`
		AuthorityRequired bool   `json:"authority_required"`
	} `json:"terminal"`
	Producer struct {
		Actor *struct {
			Kind   string `json:"kind"`
			Source string `json:"source"`
		} `json:"actor,omitempty"`
		Runner       string `json:"runner"`
		RunnerCommit string `json:"runner_commit"`
		Placement    string `json:"placement"`
		WorkerID     string `json:"worker_id"`
		Invoker      *struct {
			Name         string `json:"name"`
			Version      string `json:"version"`
			Commit       string `json:"commit"`
			ConfigDigest string `json:"config_digest"`
			Dirty        bool   `json:"dirty,omitempty"`
		} `json:"invoker,omitempty"`
	} `json:"producer"`
	Tasks     []Task    `json:"tasks"`
	CreatedAt time.Time `json:"created_at"`
}

type Task struct {
	Content            *Identity `json:"content,omitempty"`
	ID                 string    `json:"id"`
	DisplayName        string    `json:"display_name,omitempty"`
	State              string    `json:"state"`
	Effect             string    `json:"effect"`
	InputDigest        string    `json:"input_digest"`
	ExecutionDigest    string    `json:"execution_digest"`
	Platform           string    `json:"platform,omitempty"`
	RuntimeImageDigest string    `json:"runtime_image_digest,omitempty"`
	Decision           string    `json:"decision,omitempty"`
	Reason             string    `json:"reason,omitempty"`
	SourceReceipt      string    `json:"source_receipt,omitempty"`
	Outputs            []struct {
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		Digest   string `json:"digest"`
		Platform string `json:"platform,omitempty"`
	} `json:"outputs"`
}

type Signature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}
type Envelope struct {
	PayloadType string      `json:"payloadType"`
	Payload     string      `json:"payload"`
	Signatures  []Signature `json:"signatures"`
}
type Entry struct {
	ReceiptDigest string   `json:"receipt_digest"`
	Envelope      Envelope `json:"envelope"`
}
type Index struct {
	Schema  string  `json:"schema"`
	Commit  string  `json:"commit"`
	Entries []Entry `json:"entries"`
}
type Note struct {
	Commit string
	Raw    []byte
}

type Contract struct {
	Kind    string   `json:"kind"`
	Exclude []string `json:"exclude,omitempty"`
}
type TaskPolicy struct {
	Services     []string `json:"services,omitempty"`
	Verification Contract `json:"verification,omitempty"`
	Platforms    []string `json:"platforms"`
}
type KeyPolicy struct {
	PublicKey    string    `json:"public_key"`
	Repositories []string  `json:"repositories"`
	Tasks        []string  `json:"tasks,omitempty"`
	Placements   []string  `json:"placements"`
	Platforms    []string  `json:"platforms"`
	NotBefore    time.Time `json:"not_before,omitempty"`
	NotAfter     time.Time `json:"not_after,omitempty"`
}
type ReusePolicy struct {
	Effects []string `json:"effects"`
	MaxAge  string   `json:"max_age,omitempty"`
}
type BranchPolicy struct {
	OnValid string `json:"on_valid"`
	Rate    int    `json:"rate,omitempty"`
}
type Policy struct {
	Schema              string                  `json:"schema"`
	Keys                map[string]KeyPolicy    `json:"keys"`
	Reuse               ReusePolicy             `json:"reuse"`
	Branches            map[string]BranchPolicy `json:"branches,omitempty"`
	Tasks               map[string]TaskPolicy   `json:"tasks,omitempty"`
	OnIntegrityConflict string                  `json:"on_integrity_conflict"`
}

type Expected struct{ Commit, Tree, FilesDigest, Branch string }
type Decision struct {
	Task      string    `json:"task"`
	Decision  string    `json:"decision"`
	Reason    string    `json:"reason"`
	Receipt   string    `json:"receipt,omitempty"`
	Run       string    `json:"run,omitempty"`
	Commit    string    `json:"commit,omitempty"`
	Signer    string    `json:"signer,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}
type Result struct {
	Tasks    []Decision `json:"tasks"`
	Warnings []string   `json:"warnings,omitempty"`
}
