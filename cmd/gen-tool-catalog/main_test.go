package main

import (
	"strings"
	"testing"
)

func TestCatalogDocumentsRestrictedProfiles(t *testing.T) {
	data, err := generate()
	if err != nil {
		t.Fatal(err)
	}
	// The same name has profile-specific contracts. Bound/delegated modes must
	// remain visible instead of being accidentally described as the broad profile.
	for _, row := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(row, "| `") {
			continue // Only inventory rows; the header text names the profiles too.
		}
		if strings.Contains(row, "`graph-read (bound)`") && !strings.HasPrefix(row, "| `graph_layer_stats_get` |") {
			t.Fatalf("bound layer inventory widened: %s", row)
		}
		if strings.Contains(row, "`reactions-read (bound)`") && !strings.HasPrefix(row, "| `reaction_stats_get` |") {
			t.Fatalf("bound reaction inventory widened: %s", row)
		}
		if strings.Contains(row, "`delegated-read`") && !strings.HasPrefix(row, "| `actor_get` |") && !strings.HasPrefix(row, "| `form_get` |") {
			t.Fatalf("delegated inventory widened: %s", row)
		}
	}
	if !strings.Contains(string(data), "| `actor_get` | `read`,") || !strings.Contains(string(data), "| `actor_create` |  |") {
		t.Fatal("read/write classification missing")
	}
}
