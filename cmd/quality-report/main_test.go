package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const success = `{"quality":"success","race":"success","security":"success","windows":"success"}`
const commit = "0123456789abcdef0123456789abcdef01234567"

func fixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	var checksums strings.Builder
	for _, name := range archiveNames {
		content := []byte("synthetic archive " + name)
		if err := os.WriteFile(filepath.Join(dir, name), content, 0600); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&checksums, "%x  %s\n", sha256.Sum256(content), name)
	}
	text := checksums.String()
	writeChecksums(t, dir, text)
	return dir, text
}
func writeChecksums(t *testing.T, dir, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceRequiresEveryCIJob(t *testing.T) {
	dir, _ := fixture(t)
	for _, value := range []string{"failure", "cancelled", "skipped", "neutral", ""} {
		t.Run(value, func(t *testing.T) {
			results := strings.Replace(success, `"race":"success"`, `"race":"`+value+`"`, 1)
			if _, err := evidence(dir, commit, results); err == nil {
				t.Fatal("accepted non-success CI")
			}
		})
	}
	for _, results := range []string{"", `{}`, `null`, strings.Replace(success, `,"race":"success"`, "", 1), strings.Replace(success, `"race":`, `"other":`, 1)} {
		if _, err := evidence(dir, commit, results); err == nil {
			t.Fatalf("accepted missing results: %s", results)
		}
	}
	if _, err := evidence(dir, "moving-main", success); err == nil {
		t.Fatal("accepted non-SHA revision")
	}
}

func TestEvidenceRejectsIncompleteOrChangedArchives(t *testing.T) {
	for _, kind := range []string{"tampered", "missing", "duplicate", "unlisted", "extra", "bad-hash"} {
		t.Run(kind, func(t *testing.T) {
			dir, checksums := fixture(t)
			path := filepath.Join(dir, archiveNames[0])
			switch kind {
			case "tampered":
				if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "duplicate":
				checksums += strings.Split(checksums, "\n")[0] + "\n"
			case "unlisted":
				checksums = strings.Replace(checksums, archiveNames[0], "../outside.tar.gz", 1)
			case "extra":
				checksums += strings.Repeat("0", 64) + "  extra.zip\n"
			case "bad-hash":
				checksums = "z" + checksums[1:]
			}
			writeChecksums(t, dir, checksums)
			if _, err := evidence(dir, commit, success); err == nil {
				t.Fatal("accepted invalid release evidence")
			}
		})
	}
}

func TestReportNeverInventsReviewOrRating(t *testing.T) {
	dir, _ := fixture(t)
	r, err := evidence(dir, commit, success)
	if err != nil {
		t.Fatal(err)
	}
	if r.Commit != commit || len(r.Archives) != 5 || r.Review != "not_performed" {
		t.Fatalf("incorrect evidence: %+v", r)
	}
	md := string(markdown(r))
	if !strings.Contains(md, "No numerical quality rating") || !strings.Contains(md, "not performed by this workflow") {
		t.Fatal("missing review/rating boundary")
	}
	for _, a := range r.Archives {
		if !strings.Contains(md, a.SHA256) {
			t.Fatal("missing archive evidence")
		}
	}
}
