package db

import (
	"database/sql"
	"time"
)

type Task struct {
	TaskUID         string       `json:"task_uid"`
	TaskName        string       `json:"task_name"`
	WebsiteGroup    string       `json:"website_group"`
	AutomationType  string       `json:"automation_type"`
	IntervalMinutes int          `json:"interval_minutes"`
	IsEnabled       int          `json:"is_enabled"`
	LastRunAt       sql.NullTime `json:"last_run_at"`
	NextRunAt       sql.NullTime `json:"next_run_at"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

type TaskLog struct {
	ID          int       `json:"id"`
	TaskUID     string    `json:"task_uid"`
	TaskName    string    `json:"task_name,omitempty"`
	WebsiteGroup string   `json:"website_group,omitempty"`
	AutomationType string `json:"automation_type,omitempty"`
	Status      string    `json:"status"`
	Message     string    `json:"message"`
	TriggeredBy string    `json:"triggered_by"`
	DurationMS  int       `json:"duration_ms"`
	CreatedAt   time.Time `json:"created_at"`
}

type Credential struct {
	WebsiteID  string    `json:"website_id"`
	Label      string    `json:"label"`
	Username   string    `json:"username"`
	Password   string    `json:"password,omitempty"` // masked in list responses
	IsEnabled  int       `json:"is_enabled"`
	UpdatedAt  time.Time `json:"updated_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type User struct {
	ID          int        `json:"id"`
	Username    string     `json:"username"`
	Role        string     `json:"role"`
	DisplayName string     `json:"display_name"`
	IsActive    int        `json:"is_active"`
	LastLogin   *time.Time `json:"last_login"`
	CreatedAt   time.Time  `json:"created_at"`
}

type DashboardStats struct {
	TotalTasks       int `json:"total_tasks"`
	CronActive       int `json:"cron_active"`
	CurrentlyRunning int `json:"currently_running"`
	FailedToday      int `json:"failed_today"`
	SuccessToday     int `json:"success_today"`
}

type SharedSession struct {
	WebsiteID       string    `json:"website_id"`
	CookiesJSON     string    `json:"cookies_json"`
	CookiesNetscape string    `json:"cookies_netscape"`
	CookiesHeader   string    `json:"cookies_header"`
	LocalStorage    string    `json:"local_storage"`
	IndexedDB       string    `json:"indexed_db"`
	UpdatedAt       time.Time `json:"updated_at"`
	CreatedAt       time.Time `json:"created_at"`
}

type ScrapedCredential struct {
	ID             int       `json:"id"`
	SourcePlatform string    `json:"source_platform"`
	WebsiteName    string    `json:"website_name"`
	LoginURL       string    `json:"login_url"`
	Username       string    `json:"username"`
	Password       string    `json:"password"`
	UpdatedAt      time.Time `json:"updated_at"`
	CreatedAt      time.Time `json:"created_at"`
}
