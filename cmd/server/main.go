package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/api/handlers"
	"github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

func main() {
	// ── 1. Load and validate configuration ────────────────────────────────────
	cfg := config.Load()

	// ── 2. Open database ──────────────────────────────────────────────────────
	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("FATAL: cannot open database: %v", err)
	}
	defer database.Close()

	// ── 3. Migrations auto-embarquées ─────────────────────────────────────────
	fmt.Println("LedgerAlps: applying migrations…")
	if err := db.Migrate(database, cfg.UsePostgres()); err != nil {
		log.Fatalf("FATAL: migration failed: %v", err)
	}
	fmt.Println("LedgerAlps: migrations up-to-date.")

	// ── 4. Gin ────────────────────────────────────────────────────────────────
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		log.Printf("PANIC recovered: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS(strings.Split(cfg.AllowedOrigins, ",")...))
	r.Use(middleware.ErrorHandler())
	if cfg.Debug {
		r.Use(gin.Logger())
	}

	// ── 5. Health ─────────────────────────────────────────────────────────────
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": "0.2.0-go"})
	})

	// ── 6. Services ───────────────────────────────────────────────────────────
	accountingSvc := accounting.New(database, cfg.UsePostgres())

	// ── 7. API v1 ─────────────────────────────────────────────────────────────
	v1 := r.Group("/api/v1")

	// Auth — public endpoints
	authHandler := handlers.NewAuthHandler(database, cfg)
	v1.POST("/auth/login", authHandler.Login)
	v1.POST("/auth/refresh", authHandler.Refresh)
	v1.POST("/auth/logout", authHandler.Logout)
	v1.POST("/auth/register", authHandler.Register)
	v1.POST("/auth/bootstrap", authHandler.Bootstrap) // one-shot: creates first admin user

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
	api.GET("/accounts/trial-balance", ah.TrialBalance)     // BEFORE /:code to avoid shadowing
	api.GET("/accounts/:code/balance", ah.AccountBalance)
	api.POST("/accounts", ah.CreateAccount)

	// Contacts
	ch := handlers.NewContactsHandler(database, cfg.UsePostgres())
	api.GET("/contacts", ch.ListContacts)
	api.GET("/contacts/:id", ch.GetContact)
	api.POST("/contacts", ch.CreateContact)
	api.PATCH("/contacts/:id", ch.UpdateContact)

	// Invoices
	ih := handlers.NewInvoicesHandler(database, cfg.UsePostgres(), accountingSvc)
	api.GET("/invoices", ih.ListInvoices)
	api.GET("/invoices/:id", ih.GetInvoice)
	api.POST("/invoices", ih.CreateInvoice)
	api.POST("/invoices/:id/transition", ih.TransitionInvoice)

	// Fiscal years + VAT declaration (admin)
	fyh := handlers.NewFiscalYearHandler(database, cfg.UsePostgres())
	api.GET("/fiscal-years", fyh.ListFiscalYears)
	api.POST("/fiscal-years/:id/close", fyh.CloseFiscalYear)
	api.POST("/vat/declaration", fyh.GenerateVATDeclaration)

	// ── 8. Start ──────────────────────────────────────────────────────────────
	addr := ":" + cfg.Port
	fmt.Printf("LedgerAlps: listening on http://localhost%s\n", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("FATAL: server error: %v", err)
	}
}
