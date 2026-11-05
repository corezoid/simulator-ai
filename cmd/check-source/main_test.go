package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleBoundary(t *testing.T) {
	for _, lines := range []int{700, 701} {
		for _, newline := range []bool{false, true} {
			dir := t.TempDir()
			body := strings.Repeat("x\n", lines-1) + "x"
			if newline {
				body += "\n"
			}
			if err := os.WriteFile(filepath.Join(dir, "module.go"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if err := check(dir); (err != nil) != (lines > 700) {
				t.Fatalf("lines=%d newline=%v error=%v", lines, newline, err)
			}
		}
	}
}
