package seoshope

import (
	"context"
	"log"
	"sync"
	"time"
)

const taskTimeout = 8 * time.Minute

// runMu ensures only one Chrome uses the SEOShope profile at a time.
var runMu sync.Mutex

// Run executes one task with its own Chrome lifecycle: launch → work → close.
// The same UserDataDir profile is reused so saved cookies speed up login.
func Run(taskUID string) (string, string) {
	if !IsSeoshopeTask(taskUID) {
		return "failed", "unknown seoshope task: " + taskUID
	}

	runMu.Lock()
	defer runMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	defer cancel()

	log.Printf("[SEOShope] Task %s — opening Chrome (profile=%s)", taskUID, profileName)
	session, err := newSession(ctx)
	if err != nil {
		return "failed", "chrome launch failed: " + err.Error()
	}
	defer session.Close()

	if taskUID == "seoshope_runSeoshopehome" {
		return runPortalHome(ctx, session)
	}
	slot, ok := SlotForTask(taskUID)
	if !ok {
		return "failed", "unknown slot for " + taskUID
	}
	return runSemrushSlot(ctx, session, slot)
}
