package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/api/handlers"
	"github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	embeddedFrontend "github.com/kmdn-ch/ledgeralps/internal/frontend"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
	"github.com/kmdn-ch/ledgeralps/internal/services/updatecheck"
	"github.com/kmdn-ch/ledgeralps/version"
)

func main() {
	// ── 1. Load and validate configuration ────────────────────────────────────
	cfg := config.Load()

	// ── 1b. Apply a restore staged from the interface ─────────────────────────
	// Restoring swaps the database file out from under every open connection,
	// so the running server cannot do it to itself. The UI stages the restore;
	// it is applied here, before the database is opened, and only here.
	//
	// A failure must not stop the start: the user would be left with neither
	// the restored books nor the ones they had. The staged copy is cleared
	// either way, so a broken restore is not retried at every launch.
	if applied, previous, err := db.ApplyPendingRestore(context.Background(), cfg, db.BackupDir()); err != nil {
		log.Printf("WARNING: la restauration demandée a échoué, la base actuelle est conservée: %v", err)
	} else if applied != "" {
		fmt.Printf("LedgerAlps: sauvegarde restaurée depuis %s\n", applied)
		if previous != "" {
			fmt.Printf("LedgerAlps: base précédente conservée dans %s\n", previous)
		}
	}

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

	// ── 3b. Automatic backup ──────────────────────────────────────────────────
	// LedgerAlps is local-first: this SQLite file is the only copy of records the
	// CO (art. 958f) requires be kept for ten years. Take a snapshot at startup
	// when the newest one is older than a day. A failure here must never stop the
	// server — the user still needs access to their books.
	//
	// BACKUP_PASSPHRASE encrypts these automatic snapshots. It is opt-in rather
	// than generated: a key the software holds protects nothing against someone
	// holding the machine, and a passphrase the user has not written down turns
	// a lost laptop into a lost ledger.
	if path, err := db.MaybeAutoBackup(
		context.Background(), database, cfg, db.BackupDir(), db.DefaultInterval, db.DefaultKeep,
		os.Getenv("BACKUP_PASSPHRASE"),
	); err != nil {
		log.Printf("WARNING: automatic backup failed: %v", err)
	} else if path != "" {
		fmt.Printf("LedgerAlps: backup written to %s\n", path)
	}

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
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": version.Version()})
	})

	// ── 6. Services ───────────────────────────────────────────────────────────
	accountingSvc := accounting.New(database, cfg.UsePostgres())

	// ── 7. API v1 ─────────────────────────────────────────────────────────────
	v1 := r.Group("/api/v1")

	// Auth — public endpoints.
	// Credential-checking routes sit behind a per-IP rate limiter so an attacker
	// cannot brute-force a password against a locally exposed instance.
	authHandler := handlers.NewAuthHandler(database, cfg)
	loginLimiter := middleware.NewLoginRateLimiter(
		middleware.DefaultLoginMaxAttempts,
		middleware.DefaultLoginWindow,
		middleware.DefaultLoginLockout,
	)
	// Persist lockouts so brute-force attempts are visible to an administrator.
	loginLimiter.OnLockout(func(ip string, until time.Time) {
		handlers.RecordLoginLockout(database, cfg.UsePostgres(), ip, until)
	})
	v1.POST("/auth/login", loginLimiter.Middleware(), authHandler.Login)
	v1.POST("/auth/refresh", loginLimiter.Middleware(), authHandler.Refresh)
	v1.POST("/auth/logout", authHandler.Logout)
	v1.POST("/auth/register", loginLimiter.Middleware(), authHandler.Register)
	v1.POST("/auth/bootstrap", loginLimiter.Middleware(), authHandler.Bootstrap) // one-shot: creates first admin user

	// Swiss registry proxy — public (called from setup wizard, no auth yet)
	v1.GET("/uid-lookup", handlers.UIDLookup)

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
	api.GET("/accounts/trial-balance", ah.TrialBalance) // BEFORE /:code to avoid shadowing
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
	// Supplier invoices (factures d'achat) — source of deductible input VAT
	sih := handlers.NewSupplierInvoicesHandler(database, cfg.UsePostgres())
	api.GET("/supplier-invoices", sih.ListSupplierInvoices)
	api.GET("/supplier-invoices/:id", sih.GetSupplierInvoice)
	api.POST("/supplier-invoices", sih.CreateSupplierInvoice)
	api.POST("/supplier-invoices/:id/transition", sih.TransitionSupplierInvoice)
	api.DELETE("/supplier-invoices/:id", sih.DeleteSupplierInvoice)

	api.GET("/invoices", ih.ListInvoices)
	api.GET("/invoices/:id", ih.GetInvoice)
	api.GET("/invoices/:id/pdf", ih.GetInvoicePDF)
	api.POST("/invoices", ih.CreateInvoice)
	api.PATCH("/invoices/:id", ih.UpdateInvoice)
	api.POST("/invoices/:id/transition", ih.TransitionInvoice)
	// Quote lifecycle: an offer becomes an invoice by producing one, not by
	// mutating into one — both documents are kept and linked.
	api.POST("/invoices/:id/convert", ih.ConvertQuote)
	api.POST("/invoices/:id/outcome", ih.SetQuoteOutcome)
	// Une note de crédit cite la facture qu'elle annule (LTVA art. 27 al. 4)
	// et son montant est borné par ce qui a déjà été crédité.
	api.POST("/invoices/:id/credit-note", ih.CreateCreditNote)

	// Sauvegardes. Créer un instantané est sûr serveur en marche ; restaurer
	// ne l'est pas, la restauration est donc préparée puis appliquée au
	// démarrage suivant. Réservé aux administrateurs : une restauration
	// remplace toute la comptabilité.
	bh := handlers.NewBackupsHandler(database, cfg)
	api.GET("/backups", middleware.RequireAdmin(cfg.JWTSecret), bh.ListBackups)
	api.POST("/backups", middleware.RequireAdmin(cfg.JWTSecret), bh.CreateBackup)
	api.POST("/backups/restore", middleware.RequireAdmin(cfg.JWTSecret), bh.StageRestore)
	api.DELETE("/backups/restore", middleware.RequireAdmin(cfg.JWTSecret), bh.CancelRestore)

	// Fiscal years + VAT declaration (admin)
	fyh := handlers.NewFiscalYearHandler(database, cfg.UsePostgres())
	api.GET("/fiscal-years", fyh.ListFiscalYears)
	api.POST("/fiscal-years/:id/close", fyh.CloseFiscalYear)
	api.POST("/vat/declaration", fyh.GenerateVATDeclaration)

	// VAT rates (static reference data — no DB)
	api.GET("/vat/rates", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"rates": []gin.H{
				{"code": "standard", "rate": 8.1, "label": "Taux normal (TVA 2024)"},
				{"code": "reduced", "rate": 2.6, "label": "Taux réduit (alimentation, livres)"},
				{"code": "special", "rate": 3.8, "label": "Taux spécial (hébergement)"},
			},
		})
	})

	// ISO 20022 — pain.001 export + camt.053 import
	isoH := handlers.NewISO20022Handler()
	api.POST("/payments/export", isoH.ExportPain001)
	api.POST("/bank-statements/import", isoH.ImportCamt053)

	// Legal archive export — CO art. 958f (10-year retention)
	expH := handlers.NewExportHandler(database, cfg.UsePostgres())
	api.GET("/exports/legal-archive", expH.LegalArchive)

	// Stats dashboard
	statsH := handlers.NewStatsHandler(database, cfg.UsePostgres())
	api.GET("/stats", statsH.GetStats)

	// Reports
	rh := handlers.NewReportsHandler(database, cfg.UsePostgres())
	api.GET("/reports/balance-sheet", rh.BalanceSheet)
	api.GET("/reports/income-statement", rh.IncomeStatement)
	api.GET("/reports/general-ledger", rh.GeneralLedger)
	api.GET("/reports/ar-aging", rh.ARaging)

	// Payments (CRUD — must be registered after /payments/export to avoid shadowing)
	ph := handlers.NewPaymentsHandler(database, cfg.UsePostgres(), accountingSvc)
	api.POST("/payments", ph.CreatePayment)
	api.GET("/payments", ph.ListPayments)
	api.GET("/payments/:id", ph.GetPayment)

	// Audit logs
	alh := handlers.NewAuditHandler(database, cfg.UsePostgres())
	api.GET("/audit-logs", alh.ListAuditLogs)
	api.GET("/audit-logs/:id/verify", alh.VerifyAuditLog)

	// Security telemetry — admin only: lockout records expose client IPs (nLPD).
	seh := handlers.NewSecurityEventHandler(database, cfg.UsePostgres())
	api.GET("/security-events", middleware.RequireAdmin(cfg.JWTSecret), seh.ListSecurityEvents)

	// Compliance advisories — served from the feed embedded in the binary,
	// so this works with no network access.
	updates := updatecheck.New(cfg.UpdateCheck, updatecheck.DefaultEndpoint, updatecheck.DefaultInterval)
	cplh := handlers.NewComplianceHandler(updates)
	api.GET("/compliance/advisories", cplh.ListAdvisories)
	api.GET("/compliance/update-check", cplh.CheckForUpdate)

	// Company settings
	sh := handlers.NewSettingsHandler(database, cfg.UsePostgres())
	api.GET("/settings/company", sh.GetCompany)
	api.PUT("/settings/company", middleware.RequireAdmin(cfg.JWTSecret), sh.PutCompany)
	api.POST("/settings/logo", sh.UploadLogo)
	api.DELETE("/settings/logo", sh.DeleteLogo)

	// ── 8. Frontend (embedded) ───────────────────────────────────────────────
	// The React build is compiled directly into the binary via //go:embed.
	// This eliminates all external path resolution and installer packaging issues.
	distFS, err := fs.Sub(embeddedFrontend.FS, "dist")
	if err != nil {
		log.Fatalf("FATAL: embedded frontend FS is broken: %v", err)
	}

	if assetsFS, err := fs.Sub(distFS, "assets"); err == nil {
		r.StaticFS("/assets", http.FS(assetsFS))
	}

	// serveEmbedded reads a file from the embedded FS and writes it directly.
	// We intentionally avoid c.FileFromFS / http.FileServer here because
	// http.FileServer issues redirects (e.g. "index.html" → "/index.html")
	// that cause ERR_TOO_MANY_REDIRECTS in the browser.
	serveEmbedded := func(c *gin.Context, path, contentType string) {
		data, err := fs.ReadFile(distFS, path)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, contentType, data)
	}

	r.GET("/favicon.ico", func(c *gin.Context) { serveEmbedded(c, "favicon.ico", "image/x-icon") })
	r.GET("/logo.svg", func(c *gin.Context) { serveEmbedded(c, "logo.svg", "image/svg+xml") })
	fmt.Println("LedgerAlps: serving embedded frontend")

	// SPA fallback: all non-API routes serve index.html for client-side routing.
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/health") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		serveEmbedded(c, "index.html", "text/html; charset=utf-8")
	})

	// ── 9. Start ──────────────────────────────────────────────────────────────
	addr := ":" + cfg.Port
	fmt.Printf("LedgerAlps: listening on http://localhost%s\n", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("FATAL: server error: %v", err)
	}
}
