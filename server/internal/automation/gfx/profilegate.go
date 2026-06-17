package gfx

import (
	"context"
	"fmt"
	"sync"
)

var profileBusy sync.Map // account website_id -> struct{}

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

// ReleaseProfileBusy marks a profile free after a GFX job finishes.
func ReleaseProfileBusy(accountID string) {
	profileBusy.Delete(accountID)
}
