package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"gohttpauto/internal/automation/azad"
	"gohttpauto/internal/automation/gfx"
	"gohttpauto/internal/automation/markhor"
	"gohttpauto/internal/automation/nox"
	"gohttpauto/internal/automation/seoshope"
	"gohttpauto/internal/automation/toolbaazar"
	"gohttpauto/internal/db"
)

const (
	TaskRunTimeout    = 70 * time.Second
	GFXTaskRunTimeout = 90 * time.Second
)

// MaxTaskRunTimeout is used for stale-job cleanup.
const MaxTaskRunTimeout = GFXTaskRunTimeout

// TaskRunTimeoutFor returns the wall-clock limit for a task UID.
func TaskRunTimeoutFor(taskUID string) time.Duration {
	if gfx.IsGFXTask(taskUID) {
		return GFXTaskRunTimeout
	}
	return TaskRunTimeout
}

var (
	activeMu  sync.Mutex
	activeSet = map[string]bool{}
)

// Submit enqueues a task. Returns false if already running.
func Submit(taskUID, triggeredBy string) bool {
	activeMu.Lock()
	if activeSet[taskUID] {
		activeMu.Unlock()
		return false
	}
	activeSet[taskUID] = true
	activeMu.Unlock()

	go func() {
		defer func() {
			activeMu.Lock()
			delete(activeSet, taskUID)
			activeMu.Unlock()
		}()

		var t db.Task
		err := db.DB.QueryRow(`
			SELECT task_uid, task_name, website_group, automation_type, interval_minutes, is_enabled
			FROM tasks WHERE task_uid=?`, taskUID).
			Scan(&t.TaskUID, &t.TaskName, &t.WebsiteGroup, &t.AutomationType, &t.IntervalMinutes, &t.IsEnabled)
		if err != nil {
			log.Printf("⚠️ [QUEUE] task %s not found", taskUID)
			return
		}

		res, _ := db.DB.Exec(
			`INSERT INTO task_logs (task_uid, status, message, triggered_by, duration_ms) VALUES (?,'running','Enqueued...',?,0)`,
			taskUID, triggeredBy)
		logID, _ := res.LastInsertId()
		start := time.Now()

		status, msg := ExecuteWithTimeout(taskUID, t.AutomationType)

		dur := int(time.Since(start).Milliseconds())
		next := time.Now().Add(time.Duration(t.IntervalMinutes) * time.Minute)
		_, _ = db.DB.Exec(`UPDATE tasks SET last_run_at=?, next_run_at=? WHERE task_uid=?`, start, next, taskUID)
		if logID > 0 {
			_, _ = db.DB.Exec(`UPDATE task_logs SET status=?, message=?, duration_ms=? WHERE id=?`, status, msg, dur, logID)
		}
		log.Printf("🏁 [QUEUE] %s → %s (%dms)", taskUID, status, dur)
	}()
	return true
}

// RunSync executes a task and blocks until finished (used by worker job poller).
func RunSync(taskUID, triggeredBy string) bool {
	activeMu.Lock()
	if activeSet[taskUID] {
		activeMu.Unlock()
		return false
	}
	activeSet[taskUID] = true
	activeMu.Unlock()

	defer func() {
		activeMu.Lock()
		delete(activeSet, taskUID)
		activeMu.Unlock()
	}()

	var t db.Task
	err := db.DB.QueryRow(`
		SELECT task_uid, task_name, website_group, automation_type, interval_minutes, is_enabled
		FROM tasks WHERE task_uid=?`, taskUID).
		Scan(&t.TaskUID, &t.TaskName, &t.WebsiteGroup, &t.AutomationType, &t.IntervalMinutes, &t.IsEnabled)
	if err != nil {
		log.Printf("⚠️ [QUEUE] task %s not found", taskUID)
		return false
	}

	res, _ := db.DB.Exec(
		`INSERT INTO task_logs (task_uid, status, message, triggered_by, duration_ms) VALUES (?,'running','Starting...',?,0)`,
		taskUID, triggeredBy)
	logID, _ := res.LastInsertId()
	start := time.Now()

	status, msg := ExecuteWithTimeout(taskUID, t.AutomationType)

	dur := int(time.Since(start).Milliseconds())
	next := time.Now().Add(time.Duration(t.IntervalMinutes) * time.Minute)
	_, _ = db.DB.Exec(`UPDATE tasks SET last_run_at=?, next_run_at=? WHERE task_uid=?`, start, next, taskUID)
	if logID > 0 {
		_, _ = db.DB.Exec(`UPDATE task_logs SET status=?, message=?, duration_ms=? WHERE id=?`, status, msg, dur, logID)
	}
	log.Printf("🏁 [QUEUE] %s → %s (%dms)", taskUID, status, dur)
	return true
}

// ExecuteWithTimeout runs automation with a hard wall-clock limit (per-task).
func ExecuteWithTimeout(taskUID, automationType string) (status, msg string) {
	timeout := TaskRunTimeoutFor(taskUID)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	type result struct {
		status, msg string
	}
	ch := make(chan result, 1)
	go func() {
		s, m := Execute(ctx, taskUID, automationType)
		ch <- result{s, m}
	}()
	select {
	case r := <-ch:
		return r.status, r.msg
	case <-ctx.Done():
		log.Printf("⏱️ [QUEUE] %s timed out after %s", taskUID, timeout)
		return "failed", fmt.Sprintf("task timeout after %d seconds", int(timeout.Seconds()))
	}
}

// Execute runs automation — HTTP engines plug in here.
func Execute(ctx context.Context, taskUID, automationType string) (status, msg string) {
	switch taskUID {
	case "nox_runSemrush":
		return nox.RunSemrush()
	case "azad_runAzadSemrush":
		return azad.RunSemrush()
	case "toolbaazar_runToolbaazarSemrush":
		return toolbaazar.RunSemrush()
	case "markho_runMarkhoSemrush":
		return markhor.RunSemrush()
	default:
		if seoshope.IsSeoshopeTask(taskUID) {
			return seoshope.Run(taskUID)
		}
		if gfx.IsGFXTask(taskUID) {
			return gfx.Run(ctx, taskUID)
		}
		return "failed", "automation not implemented yet: " + taskUID
	}
}
