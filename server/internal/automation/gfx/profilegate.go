package gfx

import (
	"context"
	"fmt"
	"sync"
)

var (
	profileBusy sync.Map // account website_id -> struct{}
	pollWake    = make(chan struct{}, 1)
)

// PollWake returns a channel notified when a profile becomes free.
func PollWake() <-chan struct{} {
	return pollWake
}

// ResetProfileGate clears in-memory profile locks (call on worker boot).
func ResetProfileGate() {
	profileBusy.Range(func(k, _ any) bool {
		profileBusy.Delete(k)
		return true
	})
}

// AccountForTask resolves which GFX pool account (Chrome profile) a task uses.
func AccountForTask(ctx context.Context, taskUID string) (string, error) {
	if !IsGFXTask(taskUID) {
		return "", fmt.Errorf("not a gfx task: %s", taskUID)
	}
	slot, err := ResolveSlot(ctx, taskUID)
	if err != nil {
		return "", err
	}
	return slot.Account.WebsiteID, nil
}

// IsProfileBusy reports whether a Chrome profile is already running a GFX job.
func IsProfileBusy(accountID string) bool {
	_, ok := profileBusy.Load(accountID)
	return ok
}

// TryAcquireProfileBusy marks a profile as in-use. Returns false if already busy.
func TryAcquireProfileBusy(accountID string) bool {
	_, loaded := profileBusy.LoadOrStore(accountID, struct{}{})
	return !loaded
}

// ReleaseProfileBusy marks a profile free and wakes the job poller immediately.
func ReleaseProfileBusy(accountID string) {
	profileBusy.Delete(accountID)
	notifyPollWake()
}

func notifyPollWake() {
	select {
	case pollWake <- struct{}{}:
	default:
	}
}
