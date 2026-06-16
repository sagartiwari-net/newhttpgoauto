package seoshope

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"gohttpauto/internal/db"
)

const (
	drainWait    = 2 * time.Second
	drainPoll    = 200 * time.Millisecond
	workerID     = "mac-worker"
	taskTimeout  = 8 * time.Minute
)

type jobResult struct {
	status string
	msg    string
}

type pendingJob struct {
	taskUID string
	result  chan jobResult
}

// ProfileRunner serializes SEOShope tasks on one Chrome profile.
// Before closing Chrome it waits briefly and drains any other pending tasks
// for the same profile (in-memory queue + job_queue rows).
type ProfileRunner struct {
	mu         sync.Mutex
	queue      []pendingJob
	processing bool
	session    *Session
}

var defaultRunner = &ProfileRunner{}

// Run blocks until the task finishes. Chrome stays open if another SEOShope
// task is already waiting or arrives during the drain window.
func Run(taskUID string) (string, string) {
	if !IsSeoshopeTask(taskUID) {
		return "failed", "unknown seoshope task: " + taskUID
	}
	resCh := make(chan jobResult, 1)
	defaultRunner.mu.Lock()
	defaultRunner.queue = append(defaultRunner.queue, pendingJob{taskUID: taskUID, result: resCh})
	start := !defaultRunner.processing
	defaultRunner.processing = true
	defaultRunner.mu.Unlock()

	if start {
		go defaultRunner.loop()
	}
	res := <-resCh
	return res.status, res.msg
}

func (r *ProfileRunner) loop() {
	for {
		r.runBatch()

		if r.waitForMoreWork(drainWait) {
			continue
		}

		r.closeSession()
		r.mu.Lock()
		if len(r.queue) > 0 || r.hasPendingDBJobUnlocked() {
			r.mu.Unlock()
			log.Println("[SEOShope] New task arrived during shutdown — keeping processor alive")
			continue
		}
		r.processing = false
		r.mu.Unlock()
		return
	}
}

func (r *ProfileRunner) runBatch() {
	for {
		job := r.popInternal()
		if job != nil {
			r.runOne(job)
			continue
		}
		dbJob := r.claimDBJob()
		if dbJob != nil {
			r.runDBJob(dbJob)
			continue
		}
		return
	}
}

func (r *ProfileRunner) popInternal() *pendingJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) == 0 {
		return nil
	}
	job := r.queue[0]
	r.queue = r.queue[1:]
	return &job
}

func (r *ProfileRunner) runOne(job *pendingJob) {
	status, msg := r.execute(job.taskUID, "queue")
	job.result <- jobResult{status: status, msg: msg}
}

type dbJob struct {
	id          int
	taskUID     string
	triggeredBy string
}

func (r *ProfileRunner) claimDBJob() *dbJob {
	placeholders := strings.Repeat("?,", len(AllTaskUIDs))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]interface{}, 0, len(AllTaskUIDs))
	for _, uid := range AllTaskUIDs {
		args = append(args, uid)
	}

	var id int
	var taskUID, triggeredBy string
	err := db.DB.QueryRow(`
		SELECT id, task_uid, triggered_by FROM job_queue
		WHERE status='pending' AND task_uid IN (`+placeholders+`)
		ORDER BY id ASC LIMIT 1`, args...).Scan(&id, &taskUID, &triggeredBy)
	if err != nil {
		return nil
	}

	res, err := db.DB.Exec(`
		UPDATE job_queue SET status='claimed', claimed_by=?, claimed_at=NOW()
		WHERE id=? AND status='pending'`, workerID, id)
	if err != nil {
		return nil
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil
	}
	log.Printf("[SEOShope] Pre-drained queued job #%d: %s", id, taskUID)
	return &dbJob{id: id, taskUID: taskUID, triggeredBy: triggeredBy}
}

func (r *ProfileRunner) runDBJob(job *dbJob) {
	status, msg := r.execute(job.taskUID, job.triggeredBy)
	_, _ = db.DB.Exec(`UPDATE job_queue SET status='done', finished_at=NOW() WHERE id=?`, job.id)

	res, _ := db.DB.Exec(
		`INSERT INTO task_logs (task_uid, status, message, triggered_by, duration_ms) VALUES (?,?,?,?,0)`,
		job.taskUID, status, msg, job.triggeredBy)
	logID, _ := res.LastInsertId()
	if logID > 0 {
		_, _ = db.DB.Exec(`UPDATE task_logs SET status=?, message=? WHERE id=?`, status, msg, logID)
	}

	var interval int
	_ = db.DB.QueryRow(`SELECT interval_minutes FROM tasks WHERE task_uid=?`, job.taskUID).Scan(&interval)
	if interval > 0 {
		next := time.Now().Add(time.Duration(interval) * time.Minute)
		_, _ = db.DB.Exec(`UPDATE tasks SET last_run_at=?, next_run_at=? WHERE task_uid=?`, time.Now(), next, job.taskUID)
	}
	log.Printf("[SEOShope] Pre-drained %s → %s", job.taskUID, status)
}

func (r *ProfileRunner) execute(taskUID, triggeredBy string) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	defer cancel()

	r.mu.Lock()
	if r.session == nil {
		sess, err := newSession(ctx)
		if err != nil {
			r.mu.Unlock()
			return "failed", "chrome launch failed: " + err.Error()
		}
		r.session = sess
		log.Printf("[SEOShope] Chrome ready for profile %s (triggered by %s → %s)", profileName, triggeredBy, taskUID)
	}
	r.mu.Unlock()

	if taskUID == "seoshope_runSeoshopehome" {
		return runPortalHome(ctx, r.session)
	}
	slot, ok := SlotForTask(taskUID)
	if !ok {
		return "failed", "unknown slot for " + taskUID
	}
	return runSemrushSlot(ctx, r.session, slot)
}

func (r *ProfileRunner) closeSession() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session != nil {
		r.session.Close()
		r.session = nil
	}
}

func (r *ProfileRunner) waitForMoreWork(maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		hasInternal := len(r.queue) > 0
		r.mu.Unlock()
		if hasInternal {
			log.Println("[SEOShope] Another task waiting in queue — keeping Chrome open")
			return true
		}
		if r.hasPendingDBJob() {
			log.Println("[SEOShope] Pending job_queue task found — keeping Chrome open")
			return true
		}
		time.Sleep(drainPoll)
	}
	log.Println("[SEOShope] No more tasks for this profile — closing Chrome")
	return false
}

func (r *ProfileRunner) hasPendingDBJob() bool {
	return r.hasPendingDBJobUnlocked()
}

func (r *ProfileRunner) hasPendingDBJobUnlocked() bool {
	placeholders := strings.Repeat("?,", len(AllTaskUIDs))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]interface{}, 0, len(AllTaskUIDs))
	for _, uid := range AllTaskUIDs {
		args = append(args, uid)
	}
	var id int
	err := db.DB.QueryRow(`
		SELECT id FROM job_queue
		WHERE status='pending' AND task_uid IN (`+placeholders+`)
		ORDER BY id ASC LIMIT 1`, args...).Scan(&id)
	return err == nil
}
