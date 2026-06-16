package queue

import (
	"gohttpauto/internal/automation/gfx"
)

// IsMacWorkerTask reports tasks that must run on the Mac worker (GFX Chrome only).
func IsMacWorkerTask(taskUID string) bool {
	return gfx.IsGFXTask(taskUID)
}

// Dispatch runs a task from the panel server.
// Non-GFX tasks start immediately on this machine; GFX tasks go to the Mac worker queue.
func Dispatch(taskUID, triggeredBy string) (local bool, err error) {
	if IsMacWorkerTask(taskUID) {
		if err := Enqueue(taskUID, triggeredBy); err != nil {
			return false, err
		}
		return false, nil
	}
	if !Submit(taskUID, triggeredBy) {
		return false, ErrTaskBusy
	}
	return true, nil
}
