package simulation

import (
	"os"
	"strconv"
	"strings"
)

// defaultRunSlots is how many simulation_run calls one process runs at once. A run may use a full
// CPU for up to MaxTimeLimit; without a cap a few calls could starve every other tool call. Further
// calls are refused at once rather than queued, so they do not hold connections either.
const defaultRunSlots = 2

var runSlots = newRunSlots(os.Getenv("SIMULATOR_AI_MAX_CONCURRENT_SIMULATIONS"))

// newRunSlots reads the cap: empty or unparsable is the default, 0 or less is no cap.
func newRunSlots(v string) chan struct{} {
	n := defaultRunSlots
	if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		n = parsed
	}
	if n <= 0 {
		return nil
	}
	return make(chan struct{}, n)
}

// acquireRunSlot takes a slot without waiting; release returns it.
func acquireRunSlot() (release func(), ok bool) {
	slots := runSlots
	if slots == nil {
		return func() {}, true
	}
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, true
	default:
		return nil, false
	}
}
