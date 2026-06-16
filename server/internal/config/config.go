package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DBHost           string
	DBPort           string
	DBUser           string
	DBPass           string
	DBName           string
	Port             string
	Env              string
	JWTSecret        string
	MasterUsername   string
	MasterPassword   string
	APIKey           string
	LogRetentionDays int
	EnableScheduler  bool
	Role             string // "panel" = enqueue only, "worker" = execute jobs
	WorkerID         string // heartbeat + job claim id (default mac-worker)
}

var Global *Config

func Load() *Config {
	env := map[string]string{
		"DB_HOST": "127.0.0.1", "DB_PORT": "3306",
		"DB_USER": "NewHttpGoAuto", "DB_PASS": "", "DB_NAME": "newhttpgoauto",
		"PORT": "4010", "ENV": "development",
		"JWT_SECRET": "gohttpauto_dev_secret",
		"MASTER_USERNAME": "admin", "MASTER_PASSWORD": "GoHttp@2026!",
		"API_KEY": "gohttp_secret_token_change_me",
		"LOG_RETENTION_DAYS": "2",
		"ENABLE_SCHEDULER":   "true",
		"ROLE":               "worker",
		"WORKER_ID":          "mac-worker",
	}
	if f, err := os.Open(".env"); err == nil {
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				env[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}
	lookup := func(k, fb string) string {
		if v, ok := os.LookupEnv(k); ok {
			return v
		}
		if v, ok := env[k]; ok {
			return v
		}
		return fb
	}
	retention, _ := strconv.Atoi(lookup("LOG_RETENTION_DAYS", "2"))
	if retention < 1 {
		retention = 2
	}
	enableSched := strings.EqualFold(lookup("ENABLE_SCHEDULER", "true"), "true")
	role := strings.ToLower(lookup("ROLE", "worker"))
	if role != "panel" && role != "worker" {
		role = "worker"
	}
	Global = &Config{
		DBHost: lookup("DB_HOST", "127.0.0.1"),
		DBPort: lookup("DB_PORT", "3306"),
		DBUser: lookup("DB_USER", "NewHttpGoAuto"),
		DBPass: lookup("DB_PASS", ""),
		DBName: lookup("DB_NAME", "newhttpgoauto"),
		Port:   lookup("PORT", "4010"),
		Env:    lookup("ENV", "development"),
		JWTSecret:      lookup("JWT_SECRET", "gohttpauto_dev_secret"),
		MasterUsername: lookup("MASTER_USERNAME", "admin"),
		MasterPassword: lookup("MASTER_PASSWORD", "GoHttp@2026!"),
		APIKey:         lookup("API_KEY", ""),
		LogRetentionDays: retention,
		EnableScheduler:  enableSched,
		Role:             role,
		WorkerID:         lookup("WORKER_ID", "mac-worker"),
	}
	return Global
}
