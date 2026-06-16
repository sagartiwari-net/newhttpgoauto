package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gohttpauto/internal/config"
	"gohttpauto/internal/db"
	"gohttpauto/internal/dbseed"
	"gohttpauto/internal/handlers"
	"gohttpauto/internal/middleware"
	"gohttpauto/internal/queue"
	"gohttpauto/internal/scheduler"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	var dbErr error
	if cfg.Role == "worker" {
		dbErr = db.InitWithRetry(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName, 90*time.Second)
	} else {
		dbErr = db.Init(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName)
	}
	if dbErr != nil {
		log.Fatalf("❌ Database connection failed: %v", dbErr)
	}
	defer db.Close()

	handlers.EnsureMasterUser(cfg.MasterUsername, cfg.MasterPassword)
	dbseed.EnsureTasks()
	handlers.StartLogCleanupLoop()
	if cfg.Role == "worker" {
		queue.StartJobPoller()
		log.Printf("🔧 [ROLE] worker — executing jobs from queue + scheduler")
	} else {
		queue.StartQueueMaintenance()
		log.Printf("🔧 [ROLE] panel — manual runs queued for worker Mac (no local execution)")
	}
	if cfg.EnableScheduler {
		scheduler.Start()
	} else {
		log.Println("⏰ [SCHEDULER] Disabled — API/dashboard only (set ENABLE_SCHEDULER=true on worker)")
	}

	// Worker Mac only needs queue poller + scheduler — no HTTP API (avoids :4011 port conflicts).
	if cfg.Role == "worker" {
		log.Println("✅ [WORKER] Ready — polling job_queue (no HTTP server on worker)")
		select {}
	}

	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://127.0.0.1:5173", "*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type", "X-API-Key"},
		AllowCredentials: true,
	}))

	api := r.Group("/api")
	{
		api.POST("/auth/login", handlers.Login)

		// External API trigger (other servers)
		api.POST("/tasks/run", middleware.APIKeyAuth(), handlers.RunTask)

		protected := api.Group("")
		protected.Use(middleware.JWTAuth())
		{
			protected.GET("/auth/me", handlers.Me)
			protected.GET("/stats", handlers.GetStats)
			protected.GET("/tasks", handlers.ListTasks)
			protected.POST("/tasks/toggle", handlers.ToggleTask)
			protected.POST("/tasks/interval", handlers.UpdateInterval)
			protected.POST("/tasks/run-manual", handlers.RunTask)
			protected.GET("/queue", handlers.ListQueue)
			protected.DELETE("/queue/:id", handlers.CancelQueueJob)
			protected.GET("/logs", handlers.ListLogs)
			protected.GET("/credentials", handlers.ListCredentials)
			protected.POST("/credentials", middleware.MasterOnly(), handlers.SaveCredential)
			protected.GET("/credentials/:website_id/password", handlers.GetCredentialPassword)
			protected.GET("/sessions", handlers.ListSessions)
			protected.GET("/scraped-credentials", handlers.ListScrapedCredentials)
			protected.GET("/scraped-credentials/:id/password", handlers.GetScrapedCredentialPassword)
			protected.GET("/users", middleware.MasterOnly(), handlers.ListUsers)
			protected.POST("/users", middleware.MasterOnly(), handlers.CreateUser)
		}
	}

	// Serve dashboard in production
	dist := filepath.Join("..", "dashboard", "dist")
	if _, err := os.Stat(dist); err == nil {
		r.Static("/assets", filepath.Join(dist, "assets"))
		r.StaticFile("/favicon.svg", filepath.Join(dist, "favicon.svg"))
		r.NoRoute(func(c *gin.Context) {
			if c.Request.Method == http.MethodGet && !hasPrefix(c.Request.URL.Path, "/api") {
				c.File(filepath.Join(dist, "index.html"))
				return
			}
			c.Status(404)
		})
	}

	log.Printf("🚀 GoHttpAuto server on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
