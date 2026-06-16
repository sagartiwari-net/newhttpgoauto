package queue

import (
	"log"
	"sync"
	"time"

	"gohttpauto/internal/db"
)

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

		status, msg := Execute(taskUID, t.AutomationType)

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

// Execute runs automation — HTTP engines plug in here.
func Execute(taskUID, automationType string) (status, msg string) {
	switch taskUID {
	case "nox_runSemrush":
		return runNoxSemrushHTTP()
	default:
		return "failed", "automation not implemented yet: " + taskUID
	}
}
