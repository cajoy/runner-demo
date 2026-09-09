package receiptverify

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
)

// This receiver deliberately supports the demo's reviewed api-linux recipe.
// These definition/configuration digests are from Runner v0.8.28 (adca9b1),
// reproduced by the signed Linux proof in testdata. A different recipe runs
// normally until its receiver contract is reviewed. No receipt selects these
// expectations. Source content is recomputed independently for every checkout.
const (
	RepositoryID = "sha256:2bc11e6cd4da217b19a07b858421fa8198af7d44470acc614917952ca18cc7ca"
	ImageDigest  = "sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b"
	Image        = "golang:1.27.1-bookworm@" + ImageDigest
	ConfigDigest = "sha256:22c519c9cb0ee0e254cd264261f763063d550d64806c2ebae9185601c87daa7b"
	// Bind the receiver's fixed recipe to the ordinary CI commands too. A
	// stricter/new workflow must not accidentally inherit older task proof.
	WorkflowDigest = "sha256:a3626fe9f7cd8774c4a99e1f696ca01dbe648a5b542a5643e024732de9d8cbe6"
)

var TaskNames = []string{"lint", "unit", "build"}
var Exclusions = []string{"README.md", "CLAUDE.md", "docs"}
var Environment = map[string]string{
	"GOENV": "off", "GOWORK": "off", "GOTOOLCHAIN": "local", "CGO_ENABLED": "0",
	"GOFLAGS": "-buildvcs=false -trimpath -mod=readonly",
}
var definitions = map[string]string{
	"lint":  "sha256:f837b0ce1395c8e689fd8e7b0022363e3cd25991aa6147b9726d56dc2a40be1c",
	"unit":  "sha256:b8e4ceaac33eabb4c31dd42ed392ac79cd1643ed22d6627311cb331f33b541aa",
	"build": "sha256:f17346f8353609fa13413c279b44eec1eb732ecdb43ebb62c163df99335e0d52",
}

func hashBytes(raw []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) }
func digest(domain string, value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	} // All callers use closed JSON data types.
	return hashBytes(append([]byte(domain+"\x00"), raw...))
}
func contractDigest(c Contract) string {
	c.Exclude = slices.Clone(c.Exclude)
	sort.Strings(c.Exclude)
	return digest("runner.verification-contract.v1", c)
}
func inputDigest(i Identity) string {
	i.DependencyOutputs = slices.Clone(i.DependencyOutputs)
	sort.Strings(i.DependencyOutputs)
	i.Digest = ""
	return digest("runner.task-input.v2", i)
}
func identity(task, files string) Identity {
	i := Identity{Schema: "runner-task-input/v2", RepositoryID: RepositoryID, Service: "api-linux", Task: task,
		ContractDigest: contractDigest(Contract{"go", Exclusions}), FilesDigest: files, DefinitionDigest: definitions[task],
		EnvironmentDigest: digest("runner.task-environment.v2", Environment), Runtime: Runtime{Driver: "docker", Platform: "linux/arm64", ImageDigest: ImageDigest}}
	i.Digest = inputDigest(i)
	return i
}
