package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/api/handlers"
	"github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
	"github.com/kmdn-ch/ledgeralps/version"
)

// distDir returns the path to the frontend dist folder. Resolution order:
//  1. LEDGERALPS_INSTALL_DIR env var (set by the Windows launcher in production).
//  2. Directory next to the running binary (standard installed layout).
//  3. %PROGRAMFILES%\LedgerAlps\dist (Windows production fallback).
//  4. frontend/dist relative to the current working directory (dev fallback).
func distDir() string {
	check := func(candidate string) bool {
		_, err := os.Stat(filepath.Join(candidate, "index.html"))
		return err == nil
	}

	// 1. Env var set by the Windows launcher — most reliable in production.
	if installDir := os.Getenv("LEDGERALPS_INSTALL_DIR"); installDir != "" {
		candidate := filepath.Join(installDir, "dist")
		if check(candidate) {
			return candidate
		}
		log.Printf("distDir: LEDGERALPS_INSTALL_DIR=%q set but dist/index.html not found there", installDir)
	}

	// 2. Next to the running binary (works for all OS installs).
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "dist")
		if check(candidate) {
			return candidate
		}
		log.Printf("distDir: dist/index.html not found next to binary at %q", filepath.Dir(exe))
	}

	// 3. Windows: standard ProgramFiles install location (fallback when env var
	//    is not propagated, e.g. server launched outside the launcher).
	for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
		if pf := os.Getenv(env); pf != "" {
			candidate := filepath.Join(pf, "LedgerAlps", "dist")
			if check(candidate) {
				log.Printf("distDir: found frontend via %%%s%%: %s", env, candidate)
				return candidate
			}
		}
	}

	// 4. Dev: relative to current working directory.
	return "frontend/dist"
}

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
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": version.Version()})
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
	api.GET("/invoices/:id/pdf", ih.GetInvoicePDF)
	api.POST("/invoices", ih.CreateInvoice)
	api.POST("/invoices/:id/transition", ih.TransitionInvoice)

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
				{"code": "reduced",  "rate": 2.6, "label": "Taux réduit (alimentation, livres)"},
				{"code": "special",  "rate": 3.8, "label": "Taux spécial (hébergement)"},
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

	// Company settings
	sh := handlers.NewSettingsHandler(database, cfg.UsePostgres())
	api.GET("/settings/company", sh.GetCompany)
	api.PUT("/settings/company", middleware.RequireAdmin(cfg.JWTSecret), sh.PutCompany)

	// ── 8. Frontend static files ─────────────────────────────────────────────
	// Serve the React build. All non-API routes fall through to index.html
	// so that client-side routing works.
	dist := distDir()
	distOK := false
	if _, err := os.Stat(dist); err == nil {
		r.Static("/assets", filepath.Join(dist, "assets"))
		r.StaticFile("/favicon.ico", filepath.Join(dist, "favicon.ico"))
		// Serve logo.svg if present
		if _, err2 := os.Stat(filepath.Join(dist, "logo.svg")); err2 == nil {
			r.StaticFile("/logo.svg", filepath.Join(dist, "logo.svg"))
		}
		distOK = true
		fmt.Printf("LedgerAlps: serving frontend from %s\n", dist)
	} else {
		log.Printf("LedgerAlps: no frontend dist found at %q — serving diagnostic page", dist)
	}

	// NoRoute is always registered so the browser never receives a raw gin 404.
	// When the dist folder is missing we return a diagnostic HTML page instead.
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/health") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if distOK {
			c.File(filepath.Join(dist, "index.html"))
			return
		}
		// Frontend dist not found — render a diagnostic page so the user
		// gets actionable information instead of a bare "404 page not found".
		c.Data(http.StatusServiceUnavailable, "text/html; charset=utf-8", []byte(`<!DOCTYPE html>
<html lang="en"><head><meta charset="UTF-8"><title>LedgerAlps — Frontend missing</title>
<style>body{font-family:system-ui,sans-serif;max-width:600px;margin:60px auto;padding:0 20px;color:#1a2e4a}
h1{color:#c0392b}code{background:#f0f4f8;padding:2px 6px;border-radius:4px}
.hint{background:#eff6ff;border:1px solid #bfdbfe;border-radius:8px;padding:16px;margin-top:24px}
</style></head><body>
<h1>Frontend not found</h1>
<p>The LedgerAlps server is running correctly, but the React frontend
could not be located at the expected path:</p>
<p><code>`+dist+`</code></p>
<div class="hint">
<strong>Fix:</strong> Re-install LedgerAlps using the latest installer from
<a href="https://github.com/kmdn-ch/LedgerAlps/releases">github.com/kmdn-ch/LedgerAlps/releases</a>.
If the problem persists, check <code>%APPDATA%\LedgerAlps\server.log</code> for details.
</div>
</body></html>`))
	})

	// ── 9. Start ──────────────────────────────────────────────────────────────
	addr := ":" + cfg.Port
	fmt.Printf("LedgerAlps: listening on http://localhost%s\n", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("FATAL: server error: %v", err)
	}
}
