package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"gohttpauto/internal/auth"
	"gohttpauto/internal/config"
	"gohttpauto/internal/db"
	"gohttpauto/internal/middleware"
	"gohttpauto/internal/queue"

	"github.com/gin-gonic/gin"
)

// ─── Auth ────────────────────────────────────────────────────────────────────

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
		return
	}
	var id int
	var hash, role string
	err := db.DB.QueryRow(
		`SELECT id, password_hash, role FROM users WHERE username=? AND is_active=1`,
		req.Username,
	).Scan(&id, &hash, &role)
	if err != nil || !auth.CheckPassword(hash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	_, _ = db.DB.Exec(`UPDATE users SET last_login=NOW() WHERE id=?`, id)
	token, _ := auth.NewToken(id, req.Username, role, config.Global.JWTSecret)
	logActivity(req.Username, role, "LOGIN", "Successful login", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"token": token, "username": req.Username, "role": role})
}

func Me(c *gin.Context) {
	claims := c.MustGet(middleware.CtxUserKey).(*auth.Claims)
	c.JSON(http.StatusOK, gin.H{"username": claims.Username, "role": claims.Role})
}

// ─── Dashboard stats ─────────────────────────────────────────────────────────

func GetStats(c *gin.Context) {
	var s db.DashboardStats
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&s.TotalTasks)
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM tasks WHERE is_enabled=1`).Scan(&s.CronActive)
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM task_logs WHERE status='running'`).Scan(&s.CurrentlyRunning)
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM task_logs WHERE status='failed' AND created_at >= CURDATE()`).Scan(&s.FailedToday)
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM task_logs WHERE status='success' AND created_at >= CURDATE()`).Scan(&s.SuccessToday)
	c.JSON(http.StatusOK, s)
}

// ─── Tasks ───────────────────────────────────────────────────────────────────

func ListTasks(c *gin.Context) {
	rows, err := db.DB.Query(`
		SELECT task_uid, task_name, website_group, automation_type,
		       interval_minutes, is_enabled, last_run_at, next_run_at, created_at, updated_at
		FROM tasks ORDER BY website_group, task_name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var list []db.Task
	for rows.Next() {
		var t db.Task
		_ = rows.Scan(&t.TaskUID, &t.TaskName, &t.WebsiteGroup, &t.AutomationType,
			&t.IntervalMinutes, &t.IsEnabled, &t.LastRunAt, &t.NextRunAt, &t.CreatedAt, &t.UpdatedAt)
		list = append(list, t)
	}
	if list == nil {
		list = []db.Task{}
	}
	c.JSON(http.StatusOK, list)
}

type toggleReq struct {
	TaskUID   string `json:"task_uid" binding:"required"`
	IsEnabled int    `json:"is_enabled"`
}

func ToggleTask(c *gin.Context) {
	var req toggleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	_, err := db.DB.Exec(`UPDATE tasks SET is_enabled=? WHERE task_uid=?`, req.IsEnabled, req.TaskUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	claims := c.MustGet(middleware.CtxUserKey).(*auth.Claims)
	state := "disabled"
	if req.IsEnabled == 1 {
		state = "enabled"
	}
	logActivity(claims.Username, claims.Role, "TASK_TOGGLE", "Task "+req.TaskUID+" "+state, c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type intervalReq struct {
	TaskUID         string `json:"task_uid" binding:"required"`
	IntervalMinutes int    `json:"interval_minutes" binding:"required,min=1"`
}

func UpdateInterval(c *gin.Context) {
	var req intervalReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	_, err := db.DB.Exec(`UPDATE tasks SET interval_minutes=? WHERE task_uid=?`, req.IntervalMinutes, req.TaskUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type runReq struct {
	TaskUID string `json:"task_uid" binding:"required"`
}

func RunTask(c *gin.Context) {
	var req runReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_uid required"})
		return
	}
	triggeredBy := "manual"
	if v, ok := c.Get("triggered_by"); ok {
		triggeredBy = v.(string)
	} else if claims, ok := c.Get(middleware.CtxUserKey); ok {
		triggeredBy = claims.(*auth.Claims).Username
	}
	if !queue.Submit(req.TaskUID, triggeredBy) {
		c.JSON(http.StatusConflict, gin.H{"error": "task already running or queued"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Task enqueued", "task_uid": req.TaskUID})
}

// ─── Logs ────────────────────────────────────────────────────────────────────

func ListLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	status := c.DefaultQuery("status", "all")
	group := c.DefaultQuery("group", "all")

	q := `
		SELECT l.id, l.task_uid, t.task_name, t.website_group, t.automation_type,
		       l.status, COALESCE(l.message,''), l.triggered_by, l.duration_ms, l.created_at
		FROM task_logs l
		LEFT JOIN tasks t ON t.task_uid = l.task_uid
		WHERE l.created_at >= DATE_SUB(NOW(), INTERVAL ? DAY)`
	args := []any{config.Global.LogRetentionDays}
	if status != "all" {
		q += ` AND l.status=?`
		args = append(args, status)
	}
	if group != "all" {
		q += ` AND t.website_group=?`
		args = append(args, group)
	}
	q += ` ORDER BY l.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.DB.Query(q, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var list []db.TaskLog
	for rows.Next() {
		var l db.TaskLog
		var triggeredBy sql.NullString
		_ = rows.Scan(&l.ID, &l.TaskUID, &l.TaskName, &l.WebsiteGroup, &l.AutomationType,
			&l.Status, &l.Message, &triggeredBy, &l.DurationMS, &l.CreatedAt)
		if triggeredBy.Valid {
			l.TriggeredBy = triggeredBy.String
		}
		list = append(list, l)
	}
	if list == nil {
		list = []db.TaskLog{}
	}
	c.JSON(http.StatusOK, list)
}

// ─── Credentials ─────────────────────────────────────────────────────────────

func ListCredentials(c *gin.Context) {
	rows, err := db.DB.Query(`
		SELECT website_id, COALESCE(label,''), username, password_enc, is_enabled, updated_at, created_at
		FROM credentials ORDER BY website_id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var list []db.Credential
	for rows.Next() {
		var cr db.Credential
		var rawPass string
		_ = rows.Scan(&cr.WebsiteID, &cr.Label, &cr.Username, &rawPass, &cr.IsEnabled, &cr.UpdatedAt, &cr.CreatedAt)
		cr.Password = auth.MaskPassword(rawPass)
		list = append(list, cr)
	}
	if list == nil {
		list = []db.Credential{}
	}
	c.JSON(http.StatusOK, list)
}

type credReq struct {
	WebsiteID string `json:"website_id" binding:"required"`
	Label     string `json:"label"`
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
}

func SaveCredential(c *gin.Context) {
	var req credReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "website_id, username, password required"})
		return
	}
	_, err := db.DB.Exec(`
		INSERT INTO credentials (website_id, label, username, password_enc, is_enabled)
		VALUES (?, ?, ?, ?, 1)
		ON DUPLICATE KEY UPDATE label=VALUES(label), username=VALUES(username),
			password_enc=VALUES(password_enc), is_enabled=1, updated_at=NOW()`,
		req.WebsiteID, req.Label, req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	claims := c.MustGet(middleware.CtxUserKey).(*auth.Claims)
	logActivity(claims.Username, claims.Role, "CREDENTIAL_SAVE", "Saved: "+req.WebsiteID, c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func GetCredentialPassword(c *gin.Context) {
	websiteID := c.Param("website_id")
	var password string
	err := db.DB.QueryRow(`SELECT password_enc FROM credentials WHERE website_id=?`, websiteID).Scan(&password)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"password": password})
}

// ─── Sessions (captured cookies / storage) ───────────────────────────────────

func ListSessions(c *gin.Context) {
	rows, err := db.DB.Query(`
		SELECT website_id,
		       COALESCE(cookies_json,''), COALESCE(cookies_netscape,''), COALESCE(cookies_header,''),
		       COALESCE(local_storage,''), COALESCE(indexed_db,''),
		       updated_at, created_at
		FROM shared_sessions ORDER BY website_id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var list []db.SharedSession
	for rows.Next() {
		var s db.SharedSession
		_ = rows.Scan(&s.WebsiteID, &s.CookiesJSON, &s.CookiesNetscape, &s.CookiesHeader,
			&s.LocalStorage, &s.IndexedDB, &s.UpdatedAt, &s.CreatedAt)
		list = append(list, s)
	}
	if list == nil {
		list = []db.SharedSession{}
	}
	c.JSON(http.StatusOK, list)
}

// ─── Scraped credentials (GFX cred-fetch automations) ────────────────────────

func ListScrapedCredentials(c *gin.Context) {
	rows, err := db.DB.Query(`
		SELECT id, source_platform, website_name, COALESCE(login_url,''),
		       username, password, updated_at, created_at
		FROM scraped_credentials ORDER BY website_name, username`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var list []db.ScrapedCredential
	for rows.Next() {
		var cr db.ScrapedCredential
		_ = rows.Scan(&cr.ID, &cr.SourcePlatform, &cr.WebsiteName, &cr.LoginURL,
			&cr.Username, &cr.Password, &cr.UpdatedAt, &cr.CreatedAt)
		cr.Password = auth.MaskPassword(cr.Password)
		list = append(list, cr)
	}
	if list == nil {
		list = []db.ScrapedCredential{}
	}
	c.JSON(http.StatusOK, list)
}

func GetScrapedCredentialPassword(c *gin.Context) {
	id := c.Param("id")
	var password string
	err := db.DB.QueryRow(`SELECT password FROM scraped_credentials WHERE id=?`, id).Scan(&password)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"password": password})
}

// ─── Users (master only) ─────────────────────────────────────────────────────

type createUserReq struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required,min=6"`
	Role        string `json:"role" binding:"required"`
	DisplayName string `json:"display_name"`
}

func ListUsers(c *gin.Context) {
	rows, _ := db.DB.Query(`SELECT id, username, role, COALESCE(display_name,''), is_active, last_login, created_at FROM users ORDER BY id`)
	defer rows.Close()
	var list []db.User
	for rows.Next() {
		var u db.User
		var last sql.NullTime
		_ = rows.Scan(&u.ID, &u.Username, &u.Role, &u.DisplayName, &u.IsActive, &last, &u.CreatedAt)
		if last.Valid {
			t := last.Time
			u.LastLogin = &t
		}
		list = append(list, u)
	}
	if list == nil {
		list = []db.User{}
	}
	c.JSON(http.StatusOK, list)
}

func CreateUser(c *gin.Context) {
	var req createUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.Role != "master" && req.Role != "operator" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be master or operator"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hash failed"})
		return
	}
	claims := c.MustGet(middleware.CtxUserKey).(*auth.Claims)
	_, err = db.DB.Exec(`
		INSERT INTO users (username, password_hash, role, display_name, created_by)
		VALUES (?, ?, ?, ?, ?)`,
		req.Username, hash, req.Role, req.DisplayName, claims.Username)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username may already exist"})
		return
	}
	logActivity(claims.Username, claims.Role, "USER_CREATE", "Created user: "+req.Username, c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func logActivity(username, role, action, details, ip string) {
	_, _ = db.DB.Exec(
		`INSERT INTO activity_logs (username, role, action, details, ip_address) VALUES (?,?,?,?,?)`,
		username, role, action, details, ip)
}

func EnsureMasterUser(username, password string) {
	var count int
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if count > 0 {
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return
	}
	_, _ = db.DB.Exec(
		`INSERT INTO users (username, password_hash, role, display_name) VALUES (?,?,'master','Administrator')`,
		username, hash)
}

func PurgeOldLogs() {
	days := config.Global.LogRetentionDays
	res, err := db.DB.Exec(`DELETE FROM task_logs WHERE created_at < DATE_SUB(NOW(), INTERVAL ? DAY)`, days)
	if err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			// silent cleanup
		}
	}
}

func StartLogCleanupLoop() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		PurgeOldLogs()
		for range ticker.C {
			PurgeOldLogs()
		}
	}()
}
