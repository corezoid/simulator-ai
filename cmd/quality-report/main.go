// quality-report validates release evidence without contacting an API or model.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/corezoid/simulator-ai/internal/version"
)

var archiveNames = []string{"simulator-ai-linux-amd64.tar.gz", "simulator-ai-linux-arm64.tar.gz", "simulator-ai-darwin-amd64.tar.gz", "simulator-ai-darwin-arm64.tar.gz", "simulator-ai-windows-amd64.zip"}
var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type archive struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}
type report struct {
	SchemaVersion int               `json:"schemaVersion"`
	Commit        string            `json:"commit"`
	Version       string            `json:"version"`
	RunURL        string            `json:"runUrl"`
	Rehearsal     bool              `json:"rehearsal"`
	Checks        map[string]string `json:"checks"`
	Review        string            `json:"review"`
	Archives      []archive         `json:"archives"`
}

func evidence(dir, commit, results string) (report, error) {
	r := report{SchemaVersion: 1, Commit: commit, Version: version.Current, Review: "not_performed"}
	if !shaPattern.MatchString(commit) {
		return r, errors.New("expected full commit SHA")
	}
	if err := json.Unmarshal([]byte(results), &r.Checks); err != nil {
		return r, fmt.Errorf("CI results: %w", err)
	}
	if len(r.Checks) != 4 {
		return r, errors.New("expected all four CI job results")
	}
	for _, name := range []string{"quality", "race", "security", "windows"} {
		if r.Checks[name] != "success" {
			return r, fmt.Errorf("CI job %s did not succeed", name)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "checksums.txt"))
	if err != nil {
		return r, err
	}
	checksums := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			return r, errors.New("malformed checksum entry")
		}
		if _, err = hex.DecodeString(fields[0]); err != nil {
			return r, errors.New("invalid SHA256")
		}
		if _, exists := checksums[fields[1]]; exists {
			return r, errors.New("duplicate checksum entry")
		}
		checksums[fields[1]] = fields[0]
	}
	if len(checksums) != len(archiveNames) {
		return r, errors.New("expected exactly five release archives")
	}
	for _, name := range archiveNames {
		expected, ok := checksums[name]
		if !ok {
			return r, fmt.Errorf("missing checksum for %s", name)
		}
		digest, err := fileHash(filepath.Join(dir, name))
		if err != nil {
			return r, err
		}
		if digest != expected {
			return r, fmt.Errorf("checksum mismatch for %s", name)
		}
		r.Archives = append(r.Archives, archive{Name: name, SHA256: digest})
	}
	return r, nil
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- fixed release archive names under the operator-owned output directory.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func markdown(r report) []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "# Release quality report\n\nCommit: `%s`  \nVersion: `%s`  \n[Workflow run](%s)  \nRehearsal: `%t`\n\n", r.Commit, r.Version, r.RunURL, r.Rehearsal)
	out.WriteString("| Required CI job | Result |\n| --- | --- |\n")
	for _, name := range []string{"quality", "race", "security", "windows"} {
		fmt.Fprintf(&out, "| %s | %s |\n", name, r.Checks[name])
	}
	out.WriteString("\nReview: **not performed by this workflow**. No numerical quality rating is assigned.\n")
	out.WriteString("These results cover automated repository checks, not architectural review or live deployment.\n\n")
	out.WriteString("## Verified release archives\n\n| Archive | SHA256 |\n| --- | --- |\n")
	for _, a := range r.Archives {
		fmt.Fprintf(&out, "| %s | `%s` |\n", a.Name, a.SHA256)
	}
	return []byte(out.String())
}

func run() error {
	commitBytes, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return err
	}
	commit := strings.TrimSpace(string(commitBytes))
	if os.Getenv("GITHUB_SHA") != commit {
		return errors.New("workflow SHA differs from checkout")
	}
	r, err := evidence("dist", commit, os.Getenv("QUALITY_RESULTS"))
	if err != nil {
		return err
	}
	runID := os.Getenv("GITHUB_RUN_ID")
	if !regexp.MustCompile(`^[0-9]+$`).MatchString(runID) {
		return errors.New("missing GitHub run ID")
	}
	repo := os.Getenv("GITHUB_REPOSITORY")
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(repo) {
		return errors.New("invalid GitHub repository")
	}
	r.RunURL = "https://github.com/" + repo + "/actions/runs/" + runID
	switch os.Getenv("GITHUB_EVENT_NAME") {
	case "workflow_dispatch":
		r.Rehearsal = true
	case "push":
		if os.Getenv("GITHUB_REF") != "refs/tags/v"+r.Version {
			return errors.New("release tag differs from plugin version")
		}
	default:
		return errors.New("expected a release tag or manual rehearsal")
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile("dist/quality-report.json", append(data, '\n'), 0644); err != nil {
		return err
	}
	return os.WriteFile("dist/quality-report.md", markdown(r), 0644)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
