package queue

import (
	"database/sql"
	"errors"
	"log"
	"time"

	"gohttpauto/internal/config"
	"gohttpauto/internal/db"
)

const (
	stalePendingAfter  = 15 * time.Minute
	queueMaintainEvery = 15 * time.Second
	queuePollInterval  = 1 * time.Second
	workerHeartbeatKey = "worker:heartbeat"
	workerAliveWindow  = 90 * time.Second
	workerHeartbeatEvery = 5 * time.Second
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

func workerID() string {
	if config.Global != nil && config.Global.WorkerID != "" {
		return config.Global.WorkerID
	}
	return "mac-worker"
}

// WorkerStatus is returned by the queue API so the panel can show if the Mac worker is online.
type WorkerStatus struct {
	WorkerID string     `json:"worker_id"`
	Alive    bool       `json:"alive"`
	LastSeen *time.Time `json:"last_seen,omitempty"`
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

// GetWorkerStatus reports whether the Mac worker has polled recently.
func GetWorkerStatus() WorkerStatus {
	wid := workerID()
	st := WorkerStatus{WorkerID: wid}
	var lockedAt sql.NullTime
	err := db.DB.QueryRow(`
		SELECT locked_at FROM system_locks WHERE lock_key=?`, workerHeartbeatKey).Scan(&lockedAt)
	if err != nil || !lockedAt.Valid {
		return st
	}
	t := lockedAt.Time
	st.LastSeen = &t
	st.Alive = time.Since(t) <= workerAliveWindow
	return st
}

func touchWorkerHeartbeat() {
	wid := workerID()
	_, err := db.DB.Exec(`
		INSERT INTO system_locks (lock_key, lock_state, locked_by, locked_at, expires_at)
		VALUES (?, 1, ?, NOW(), NOW() + INTERVAL 2 MINUTE)
		ON DUPLICATE KEY UPDATE lock_state=1, locked_by=?, locked_at=NOW(), expires_at=NOW() + INTERVAL 2 MINUTE`,
		workerHeartbeatKey, wid, wid)
	if err != nil {
		log.Printf("⚠️ [WORKER] heartbeat update failed: %v", err)
	}
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

// ExpireStaleJobs marks old pending/claimed jobs and running logs as failed.
func ExpireStaleJobs() int {
	cancelOrphanPending()
	pendingSec := int(stalePendingAfter.Seconds())
	runSec := int(TaskRunTimeout.Seconds())

	res, _ := db.DB.Exec(`
		UPDATE job_queue SET status='failed', finished_at=NOW()
		WHERE status='pending' AND created_at < NOW() - INTERVAL ? SECOND`, pendingSec)
	pending, _ := res.RowsAffected()

	res2, _ := db.DB.Exec(`
		UPDATE job_queue SET status='failed', finished_at=NOW()
		WHERE status='claimed' AND claimed_at IS NOT NULL
		  AND claimed_at < NOW() - INTERVAL ? SECOND`, runSec)
	claimed, _ := res2.RowsAffected()

	logs, _ := db.DB.Exec(`
		UPDATE task_logs SET status='failed',
			message=CONCAT(COALESCE(message,''), ' [auto-failed: task timeout]'),
			duration_ms=TIMESTAMPDIFF(MICROSECOND, created_at, NOW()) DIV 1000
		WHERE status='running' AND created_at < NOW() - INTERVAL ? SECOND`, runSec)

	n := int(pending + claimed)
	logN, _ := logs.RowsAffected()
	if n > 0 || logN > 0 {
		log.Printf("⏱️ [QUEUE] Expired %d stale job(s), %d running log(s)", n, logN)
	}
	return n + int(logN)
}

// cancelOrphanPending fails pending jobs that are not GFX (panel runs those on the server now).
func cancelOrphanPending() {
	res, _ := db.DB.Exec(`
		UPDATE job_queue SET status='failed', finished_at=NOW()
		WHERE status='pending' AND task_uid NOT LIKE 'gfx\\_%' ESCAPE '\\'`)
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("🧹 [QUEUE] Cleared %d non-GFX pending job(s) (run on panel server)", n)
	}
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
	log.Println("🧹 [QUEUE] Stale job cleanup started (70s run timeout)")
}

// StartJobPoller runs on worker — picks pending jobs from MySQL and executes locally.
func StartJobPoller() {
	StartQueueMaintenance()
	go startWorkerHeartbeat()
	go func() {
		log.Printf("👂 [WORKER] Job poller started (%s interval, id=%s)", queuePollInterval, workerID())
		pollOnce()
		ticker := time.NewTicker(queuePollInterval)
		defer ticker.Stop()
		for range ticker.C {
			pollOnce()
		}
	}()
}

func startWorkerHeartbeat() {
	touchWorkerHeartbeat()
	go func() {
		ticker := time.NewTicker(workerHeartbeatEvery)
		defer ticker.Stop()
		for range ticker.C {
			touchWorkerHeartbeat()
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
			WHERE status='pending' AND task_uid LIKE 'gfx\\_%' ESCAPE '\\'
			ORDER BY id ASC LIMIT 1`).Scan(&id, &taskUID, &triggeredBy)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				log.Printf("⚠️ [WORKER] poll pending jobs failed: %v", err)
			}
			return
		}
		res, err := db.DB.Exec(`
			UPDATE job_queue SET status='claimed', claimed_by=?, claimed_at=NOW()
			WHERE id=? AND status='pending'`, workerID(), id)
		if err != nil {
			log.Printf("⚠️ [WORKER] claim job #%d failed: %v", id, err)
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
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ [WORKER] panic in job #%d (%s): %v", id, taskUID, r)
			_, _ = db.DB.Exec(`UPDATE job_queue SET status='failed', finished_at=NOW() WHERE id=?`, id)
		}
	}()

	ok := RunSync(taskUID, triggeredBy)
	if !ok {
		log.Printf("⚠️ [WORKER] %s skipped — job #%d marked failed", taskUID, id)
		_, _ = db.DB.Exec(`UPDATE job_queue SET status='failed', finished_at=NOW() WHERE id=?`, id)
		return
	}
	_, _ = db.DB.Exec(`UPDATE job_queue SET status='done', finished_at=NOW() WHERE id=?`, id)
}
