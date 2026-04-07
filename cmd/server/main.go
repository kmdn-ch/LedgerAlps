package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/api/handlers"
	"github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

func main() {
	// ── 1. Load and validate configuration ────────────────────────────────────
	// Aborts with os.Exit(1) if JWT_SECRET is weak, empty, or < 32 chars.
	cfg := config.Load()

	// ── 2. Open database (SQLite WAL by default, PostgreSQL if DSN is set) ────
	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("FATAL: cannot open database: %v", err)
	}
	defer database.Close()

	// ── 3. Apply embedded migrations automatically ────────────────────────────
	fmt.Println("LedgerAlps: applying migrations…")
	if err := db.Migrate(database, cfg.UsePostgres()); err != nil {
		log.Fatalf("FATAL: migration failed: %v", err)
	}
	fmt.Println("LedgerAlps: migrations up-to-date.")

	// ── 4. Configure Gin ──────────────────────────────────────────────────────
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		log.Printf("PANIC recovered: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}))
	r.Use(middleware.SecurityHeaders())
	if cfg.Debug {
		r.Use(gin.Logger())
	}

	// ── 5. Health check (unauthenticated) ─────────────────────────────────────
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": "0.2.0-go"})
	})

	// ── 6. Services ───────────────────────────────────────────────────────────
	accountingSvc := accounting.New(database, cfg.UsePostgres())

	// ── 7. API v1 routes ──────────────────────────────────────────────────────
	v1 := r.Group("/api/v1")

	// Auth (public)
	v1.POST("/auth/login", handlers.NewAuthHandler(database, cfg).Login)

	// Protected routes — JWT required
	api := v1.Group("")
	api.Use(middleware.RequireAuth(cfg.JWTSecret))

	// Journal
	jh := handlers.NewJournalHandler(database, cfg.UsePostgres())
	jwh := handlers.NewJournalWriteHandler(accountingSvc)
	api.GET("/journal", jh.ListJournal)
	api.POST("/journal", jwh.CreateEntry)
	api.POST("/journal/:id/post", jwh.PostEntry)

	// Accounts
	ah := handlers.NewAccountsHandler(database, cfg.UsePostgres())
	api.GET("/accounts", ah.ListAccounts)
	api.POST("/accounts", ah.CreateAccount)

	// Contacts
	ch := handlers.NewContactsHandler(database, cfg.UsePostgres())
	api.GET("/contacts", ch.ListContacts)
	api.GET("/contacts/:id", ch.GetContact)
	api.POST("/contacts", ch.CreateContact)
	api.PATCH("/contacts/:id", ch.UpdateContact)

	// Invoices
	ih := handlers.NewInvoicesHandler(database, cfg.UsePostgres())
	api.GET("/invoices", ih.ListInvoices)
	api.GET("/invoices/:id", ih.GetInvoice)
	api.POST("/invoices", ih.CreateInvoice)
	api.POST("/invoices/:id/transition", ih.TransitionInvoice)

	// ── 8. Start server ───────────────────────────────────────────────────────
	addr := ":" + cfg.Port
	fmt.Printf("LedgerAlps: listening on http://localhost%s\n", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("FATAL: server error: %v", err)
	}
}
