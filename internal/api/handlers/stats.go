package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// ─── Response DTOs ────────────────────────────────────────────────────────────

type statsInvoices struct {
	Draft          int    `json:"draft"`
	Sent           int    `json:"sent"`
	Overdue        int    `json:"overdue"`
	Paid           int    `json:"paid"`
	Cancelled      int    `json:"cancelled"`
	Archived       int    `json:"archived"`
	TotalReceivable string `json:"total_receivable"`
}

type statsJournal struct {
	TotalEntries  int    `json:"total_entries"`
	PostedEntries int    `json:"posted_entries"`
	DraftEntries  int    `json:"draft_entries"`
	LastEntryDate string `json:"last_entry_date"`
}

type statsAccounts struct {
	TotalAccounts  int `json:"total_accounts"`
	ActiveAccounts int `json:"active_accounts"`
}

type statsContacts struct {
	Total     int `json:"total"`
	Customers int `json:"customers"`
	Suppliers int `json:"suppliers"`
}

type statsFiscalYear struct {
	CurrentLabel string `json:"current_label"`
	Status       string `json:"status"`
}

type statsResponse struct {
	Invoices   statsInvoices    `json:"invoices"`
	Journal    statsJournal     `json:"journal"`
	Accounts   statsAccounts    `json:"accounts"`
	Contacts   statsContacts    `json:"contacts"`
	FiscalYear *statsFiscalYear `json:"fiscal_year"`
}

// ─── Handler ──────────────────────────────────────────────────────────────────

type StatsHandler struct {
	db     *sql.DB
	usesPG bool
}

func NewStatsHandler(database *sql.DB, usesPG bool) *StatsHandler {
	return &StatsHandler{db: database, usesPG: usesPG}
}

// GetStats GET /api/v1/stats
func (h *StatsHandler) GetStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	resp := statsResponse{}

	// ── Invoices: counts by status ────────────────────────────────────────────
	{
		q := db.Rebind(`SELECT status, COUNT(*) FROM invoices GROUP BY status`, h.usesPG)
		rows, err := h.db.QueryContext(ctx, q)
		if err != nil {
			log.Printf("stats: invoices group-by query failed: %v", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var status string
				var count int
				if err := rows.Scan(&status, &count); err != nil {
					log.Printf("stats: invoices scan failed: %v", err)
					continue
				}
				switch status {
				case "draft":
					resp.Invoices.Draft = count
				case "sent":
					resp.Invoices.Sent = count
				case "overdue":
					resp.Invoices.Overdue = count
				case "paid":
					resp.Invoices.Paid = count
				case "cancelled":
					resp.Invoices.Cancelled = count
				case "archived":
					resp.Invoices.Archived = count
				}
			}
		}
	}

	// ── Invoices: total receivable (sent + overdue) ───────────────────────────
	{
		q := db.Rebind(`
			SELECT COALESCE(SUM(total_amount), 0)
			FROM invoices
			WHERE status IN ('sent')`, h.usesPG)
		var receivable float64
		if err := h.db.QueryRowContext(ctx, q).Scan(&receivable); err != nil {
			log.Printf("stats: invoices receivable query failed: %v", err)
		}
		resp.Invoices.TotalReceivable = fmt.Sprintf("%.2f", receivable)
	}

	// ── Journal: total entries ────────────────────────────────────────────────
	{
		q := db.Rebind(`SELECT COUNT(*) FROM journal_entries`, h.usesPG)
		if err := h.db.QueryRowContext(ctx, q).Scan(&resp.Journal.TotalEntries); err != nil {
			log.Printf("stats: journal total count failed: %v", err)
		}
	}

	// ── Journal: posted count + draft count + last entry date ─────────────────
	{
		q := db.Rebind(`SELECT COUNT(*) FROM journal_entries WHERE status = 'posted'`, h.usesPG)
		if err := h.db.QueryRowContext(ctx, q).Scan(&resp.Journal.PostedEntries); err != nil {
			log.Printf("stats: journal posted count failed: %v", err)
		}
		resp.Journal.DraftEntries = resp.Journal.TotalEntries - resp.Journal.PostedEntries

		var lastDate sql.NullString
		q2 := db.Rebind(`SELECT MAX(date) FROM journal_entries WHERE status = 'posted'`, h.usesPG)
		if err := h.db.QueryRowContext(ctx, q2).Scan(&lastDate); err != nil {
			log.Printf("stats: journal last entry date failed: %v", err)
		}
		if lastDate.Valid {
			// Trim to YYYY-MM-DD in case the DB returns a full timestamp
			d := lastDate.String
			if len(d) > 10 {
				d = d[:10]
			}
			resp.Journal.LastEntryDate = d
		}
	}

	// ── Accounts: total ───────────────────────────────────────────────────────
	{
		q := db.Rebind(`SELECT COUNT(*) FROM accounts`, h.usesPG)
		if err := h.db.QueryRowContext(ctx, q).Scan(&resp.Accounts.TotalAccounts); err != nil {
			log.Printf("stats: accounts total count failed: %v", err)
		}
	}

	// ── Accounts: active (referenced in at least one journal line) ────────────
	{
		q := db.Rebind(`
			SELECT COUNT(*) FROM accounts
			WHERE id IN (SELECT DISTINCT account_id FROM journal_lines)`, h.usesPG)
		if err := h.db.QueryRowContext(ctx, q).Scan(&resp.Accounts.ActiveAccounts); err != nil {
			log.Printf("stats: accounts active count failed: %v", err)
		}
	}

	// ── Contacts: group by contact_type ──────────────────────────────────────
	{
		q := db.Rebind(`SELECT contact_type, COUNT(*) FROM contacts GROUP BY contact_type`, h.usesPG)
		rows, err := h.db.QueryContext(ctx, q)
		if err != nil {
			log.Printf("stats: contacts group-by query failed: %v", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var ct string
				var count int
				if err := rows.Scan(&ct, &count); err != nil {
					log.Printf("stats: contacts scan failed: %v", err)
					continue
				}
				resp.Contacts.Total += count
				switch ct {
				case "customer":
					resp.Contacts.Customers = count
				case "supplier":
					resp.Contacts.Suppliers = count
				}
			}
		}
	}

	// ── Fiscal year: current open year ───────────────────────────────────────
	{
		q := db.Rebind(`
			SELECT label, status FROM fiscal_years
			WHERE status = 'open'
			ORDER BY start_date DESC
			LIMIT 1`, h.usesPG)
		var label, status string
		err := h.db.QueryRowContext(ctx, q).Scan(&label, &status)
		if err == nil {
			resp.FiscalYear = &statsFiscalYear{
				CurrentLabel: label,
				Status:       status,
			}
		} else if err != sql.ErrNoRows {
			log.Printf("stats: fiscal year query failed: %v", err)
		}
		// nil FiscalYear stays nil when no open year exists
	}

	c.JSON(http.StatusOK, resp)
}
