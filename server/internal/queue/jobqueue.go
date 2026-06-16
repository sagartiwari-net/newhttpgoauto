package queue

import (
	"database/sql"
	"errors"
	"log"
	"time"

	"gohttpauto/internal/db"
)

const (
	workerID           = "mac-worker"
	staleQueueAfter    = 15 * time.Minute
	queueMaintainEvery = 1 * time.Minute
)

var ErrTaskBusy = errors.New("task already running or queued")

// JobRow is a queue item for the dashboard.
type JobRow struct {
	ID          int        `json:"id"`
	TaskUID     string     `json:"task_uid"`
	TaskName    string     `json:"task_name"`
	Status      string     `json:"status"`
	TriggeredBy string     `json:"triggered_by"`
	ClaimedBy   string     `json:"claimed_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ClaimedAt   *time.Time `json:"claimed_at,omitempty"`
}

// Enqueue adds a task for the worker if it is not already pending or running.
func Enqueue(taskUID, triggeredBy string) error {
	ExpireStaleJobs()

	var busy int
	err := db.DB.QueryRow(`
		SELECT COUNT(*) FROM job_queue
		WHERE task_uid=? AND status IN ('pending','claimed')`, taskUID).Scan(&busy)
	if err != nil {
		return err
	}
	if busy > 0 {
		return ErrTaskBusy
	}

	_, err = db.DB.Exec(
		`INSERT INTO job_queue (task_uid, triggered_by, status) VALUES (?,?,'pending')`,
		taskUID, triggeredBy)
	if err != nil {
		return err
	}
	log.Printf("📥 [QUEUE] Enqueued %s (by %s) for worker", taskUID, triggeredBy)
	return nil
}

// ListActiveJobs returns pending and claimed jobs.
func ListActiveJobs() ([]JobRow, error) {
	ExpireStaleJobs()
	rows, err := db.DB.Query(`
		SELECT j.id, j.task_uid, COALESCE(t.task_name, j.task_uid), j.status,
		       j.triggered_by, j.claimed_by, j.created_at, j.claimed_at
		FROM job_queue j
		LEFT JOIN tasks t ON t.task_uid = j.task_uid
		WHERE j.status IN ('pending','claimed')
		ORDER BY FIELD(j.status,'claimed','pending'), j.created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []JobRow
	for rows.Next() {
		var j JobRow
		var claimedBy sql.NullString
		var claimedAt sql.NullTime
		if err := rows.Scan(&j.ID, &j.TaskUID, &j.TaskName, &j.Status,
			&j.TriggeredBy, &claimedBy, &j.CreatedAt, &claimedAt); err != nil {
			continue
		}
		if claimedBy.Valid {
			j.ClaimedBy = claimedBy.String
		}
		if claimedAt.Valid {
			t := claimedAt.Time
			j.ClaimedAt = &t
		}
		list = append(list, j)
	}
	if list == nil {
		list = []JobRow{}
	}
	return list, nil
}

// CancelJob removes a pending job from the queue (kill).
func CancelJob(id int) (bool, error) {
	res, err := db.DB.Exec(`
		UPDATE job_queue SET status='cancelled', finished_at=NOW()
		WHERE id=? AND status='pending'`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ExpireStaleJobs marks old pending/claimed jobs as failed.
func ExpireStaleJobs() int {
	sec := int(staleQueueAfter.Seconds())
	res, _ := db.DB.Exec(`
		UPDATE job_queue SET status='failed', finished_at=NOW()
		WHERE status='pending' AND created_at < NOW() - INTERVAL ? SECOND`, sec)
	pending, _ := res.RowsAffected()

	res2, _ := db.DB.Exec(`
		UPDATE job_queue SET status='failed', finished_at=NOW()
		WHERE status='claimed' AND claimed_at IS NOT NULL
		  AND claimed_at < NOW() - INTERVAL ? SECOND`, sec)
	claimed, _ := res2.RowsAffected()

	n := int(pending + claimed)
	if n > 0 {
		log.Printf("⏱️ [QUEUE] Expired %d stale job(s) (>15m)", n)
	}
	_, _ = db.DB.Exec(`
		UPDATE task_logs SET status='failed',
			message=CONCAT(COALESCE(message,''), ' [auto-failed: queue timeout]')
		WHERE status='running' AND created_at < NOW() - INTERVAL ? SECOND`, sec)
	return n
}

// StartQueueMaintenance periodically expires stuck queue rows.
func StartQueueMaintenance() {
	go func() {
		ticker := time.NewTicker(queueMaintainEvery)
		defer ticker.Stop()
		for range ticker.C {
			ExpireStaleJobs()
		}
	}()
	log.Println("🧹 [QUEUE] Stale job cleanup started (15m timeout)")
}

// StartJobPoller runs on worker — picks pending jobs from MySQL and executes locally.
func StartJobPoller() {
	StartQueueMaintenance()
	go func() {
		log.Println("👂 [WORKER] Job poller started (3s interval)")
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			pollOnce()
		}
	}()
}

func pollOnce() {
	ExpireStaleJobs()
	for {
		var id int
		var taskUID, triggeredBy string
		err := db.DB.QueryRow(`
			SELECT id, task_uid, triggered_by FROM job_queue
			WHERE status='pending' ORDER BY id ASC LIMIT 1`).Scan(&id, &taskUID, &triggeredBy)
		if err != nil {
			return
		}
		res, err := db.DB.Exec(`
			UPDATE job_queue SET status='claimed', claimed_by=?, claimed_at=NOW()
			WHERE id=? AND status='pending'`, workerID, id)
		if err != nil {
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return
		}
		log.Printf("▶️ [WORKER] Claimed job #%d: %s (by %s)", id, taskUID, triggeredBy)
		go runClaimedJob(id, taskUID, triggeredBy)
	}
}

func runClaimedJob(id int, taskUID, triggeredBy string) {
	if !RunSync(taskUID, triggeredBy) {
		log.Printf("⚠️ [WORKER] %s already running — job #%d marked failed", taskUID, id)
		_, _ = db.DB.Exec(`UPDATE job_queue SET status='failed', finished_at=NOW() WHERE id=?`, id)
		return
	}
	_, _ = db.DB.Exec(`UPDATE job_queue SET status='done', finished_at=NOW() WHERE id=?`, id)
}
