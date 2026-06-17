package dbseed

import (
	"log"

	"gohttpauto/internal/db"
)

// TaskDef is a built-in automation registered in code and synced to DB on startup.
type TaskDef struct {
	UID            string
	Name           string
	Group          string
	AutomationType string
	IntervalMin    int
}

// DefaultTasks — add new automations here when implemented in internal/automation/.
var DefaultTasks = []TaskDef{
	{UID: "nox_runSemrush", Name: "Semrush (NoxTools)", Group: "nox", AutomationType: "http", IntervalMin: 20},
	{UID: "azad_runAzadSemrush", Name: "Semrush (Azad)", Group: "azad", AutomationType: "http", IntervalMin: 60},
	{UID: "toolbaazar_runToolbaazarSemrush", Name: "Semrush (Toolbaazar)", Group: "toolbaazar", AutomationType: "http", IntervalMin: 60},
	{UID: "markho_runMarkhoSemrush", Name: "Semrush (Markhor)", Group: "markhor", AutomationType: "http", IntervalMin: 60},
	{UID: "seoshope_runSemrush", Name: "Semrush (SEOShope)", Group: "seoshope", AutomationType: "chrome_hybrid", IntervalMin: 60},
	{UID: "seoshope_runSemrush2", Name: "Semrush 2 (SEOShope)", Group: "seoshope", AutomationType: "chrome_hybrid", IntervalMin: 60},
	{UID: "seoshope_runSemrush3", Name: "Semrush 3 (SEOShope)", Group: "seoshope", AutomationType: "chrome_hybrid", IntervalMin: 60},
	{UID: "seoshope_runSemrush4", Name: "Semrush 4 (SEOShope)", Group: "seoshope", AutomationType: "chrome_hybrid", IntervalMin: 60},
	{UID: "seoshope_runSemrush5", Name: "Semrush 5 (SEOShope)", Group: "seoshope", AutomationType: "chrome_hybrid", IntervalMin: 60},
	{UID: "seoshope_runSemrush6", Name: "Semrush 6 (SEOShope)", Group: "seoshope", AutomationType: "chrome_hybrid", IntervalMin: 60},
	{UID: "seoshope_runSemrush7", Name: "Semrush 7 (SEOShope)", Group: "seoshope", AutomationType: "chrome_hybrid", IntervalMin: 60},
	{UID: "seoshope_runSeoshopehome", Name: "SEOShope Portal Login", Group: "seoshope", AutomationType: "chrome_hybrid", IntervalMin: 60},
}

// EnsureTasks upserts built-in tasks (inserts new rows; updates name/type on existing).
func EnsureTasks() {
	all := append([]TaskDef{}, DefaultTasks...)
	all = append(all, GFXTasks...)
	log.Println("🌱 [DB] Syncing built-in tasks...")
	var inserted, updated int
	for _, t := range all {
		res, err := db.DB.Exec(`
			INSERT INTO tasks (task_uid, task_name, website_group, automation_type, interval_minutes, is_enabled)
			VALUES (?, ?, ?, ?, ?, 0)
			ON DUPLICATE KEY UPDATE
				task_name = VALUES(task_name),
				website_group = VALUES(website_group),
				automation_type = VALUES(automation_type),
				interval_minutes = VALUES(interval_minutes)`,
			t.UID, t.Name, t.Group, t.AutomationType, t.IntervalMin,
		)
		if err != nil {
			log.Printf("⚠️ [DB] seed %s: %v", t.UID, err)
			continue
		}
		n, _ := res.RowsAffected()
		if n == 1 {
			inserted++
			log.Printf("🌱 [DB] Seeded task: %s", t.UID)
		} else if n == 2 {
			updated++
		}
	}
	log.Printf("🌱 [DB] Sync done — %d new, %d updated", inserted, updated)
}
