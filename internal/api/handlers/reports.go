package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// ─── ReportsHandler ───────────────────────────────────────────────────────────

type ReportsHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewReportsHandler(database *sql.DB, usePostgres bool) *ReportsHandler {
	return &ReportsHandler{db: database, usePostgres: usePostgres}
}

// ─── Response DTOs ────────────────────────────────────────────────────────────

type reportAccountLine struct {
	AccountID   string  `json:"account_id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Balance     float64 `json:"balance"`
}

type balanceSheetResponse struct {
	AsOf                  string              `json:"as_of"`
	Assets                []reportAccountLine `json:"assets"`
	Liabilities           []reportAccountLine `json:"liabilities"`
	Equity                []reportAccountLine `json:"equity"`
	TotalAssets           float64             `json:"total_assets"`
	TotalLiabilitiesEquity float64            `json:"total_liabilities_equity"`
}

type incomeStatementLine struct {
	AccountID string  `json:"account_id"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Amount    float64 `json:"amount"`
}

type incomeStatementResponse struct {
	From      string                `json:"from"`
	To        string                `json:"to"`
	Revenue   []incomeStatementLine `json:"revenue"`
	Expenses  []incomeStatementLine `json:"expenses"`
	TotalRevenue  float64           `json:"total_revenue"`
	TotalExpenses float64           `json:"total_expenses"`
	NetIncome     float64           `json:"net_income"`
}

type generalLedgerLine struct {
	EntryID     string  `json:"entry_id"`
	Reference   string  `json:"reference"`
	Date        string  `json:"date"`
	Description string  `json:"description"`
	Debit       float64 `json:"debit"`
	Credit      float64 `json:"credit"`
	Balance     float64 `json:"balance"`
}

type generalLedgerResponse struct {
	AccountCode    string              `json:"account_code"`
	AccountName    string              `json:"account_name"`
	From           string              `json:"from"`
	To             string              `json:"to"`
	OpeningBalance float64             `json:"opening_balance"`
	Lines          []generalLedgerLine `json:"lines"`
	ClosingBalance float64             `json:"closing_balance"`
}

type arAgingLine struct {
	InvoiceID     string  `json:"invoice_id"`
	InvoiceNumber string  `json:"invoice_number"`
	ContactName   string  `json:"contact_name"`
	TotalAmount   float64 `json:"total_amount"`
	DueDate       string  `json:"due_date"`
	DaysOverdue   int     `json:"days_overdue"`
	Bucket        string  `json:"bucket"`
}

type arAgingResponse struct {
	AsOf      string        `json:"as_of"`
	Current   []arAgingLine `json:"current"`
	Days1To30 []arAgingLine `json:"days_1_to_30"`
	Days31To60 []arAgingLine `json:"days_31_to_60"`
	Days61To90 []arAgingLine `json:"days_61_to_90"`
	Days91Plus []arAgingLine `json:"days_91_plus"`
	TotalCurrent   float64 `json:"total_current"`
	TotalDays1To30 float64 `json:"total_days_1_to_30"`
	TotalDays31To60 float64 `json:"total_days_31_to_60"`
	TotalDays61To90 float64 `json:"total_days_61_to_90"`
	TotalDays91Plus float64 `json:"total_days_91_plus"`
	GrandTotal      float64 `json:"grand_total"`
}

// ─── GET /api/v1/reports/balance-sheet ───────────────────────────────────────

// BalanceSheet returns assets, liabilities and equity as of a given date.
// Query param: date=YYYY-MM-DD (defaults to today).
func (h *ReportsHandler) BalanceSheet(c *gin.Context) {
	asOf := c.Query("date")
	if asOf == "" {
		asOf = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", asOf); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date must be YYYY-MM-DD"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Aggregate posted journal lines up to (and including) asOf per account.
	// For assets: balance = sum(debit) - sum(credit)
	// For liabilities/equity: balance = sum(credit) - sum(debit)
	q := db.Rebind(`
		SELECT
		    a.id,
		    a.code,
		    a.name,
		    a.account_type,
		    COALESCE(SUM(jl.debit_amount),  0) AS total_debit,
		    COALESCE(SUM(jl.credit_amount), 0) AS total_credit
		FROM accounts a
		LEFT JOIN journal_lines jl ON jl.account_id = a.id
		LEFT JOIN journal_entries je
		       ON je.id = jl.entry_id
		      AND je.status = 'posted'
		      AND je.date <= ?
		WHERE a.account_type IN ('asset','liability','equity')
		GROUP BY a.id, a.code, a.name, a.account_type
		ORDER BY a.code`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q, asOf)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	resp := balanceSheetResponse{
		AsOf:        asOf,
		Assets:      []reportAccountLine{},
		Liabilities: []reportAccountLine{},
		Equity:      []reportAccountLine{},
	}

	for rows.Next() {
		var id, code, name, accountType string
		var totalDebit, totalCredit float64
		if err := rows.Scan(&id, &code, &name, &accountType, &totalDebit, &totalCredit); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}

		line := reportAccountLine{AccountID: id, Code: code, Name: name}
		switch accountType {
		case "asset":
			line.Balance = totalDebit - totalCredit
			if line.Balance != 0 {
				resp.Assets = append(resp.Assets, line)
				resp.TotalAssets += line.Balance
			}
		case "liability":
			line.Balance = totalCredit - totalDebit
			if line.Balance != 0 {
				resp.Liabilities = append(resp.Liabilities, line)
				resp.TotalLiabilitiesEquity += line.Balance
			}
		case "equity":
			line.Balance = totalCredit - totalDebit
			if line.Balance != 0 {
				resp.Equity = append(resp.Equity, line)
				resp.TotalLiabilitiesEquity += line.Balance
			}
		}
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ─── GET /api/v1/reports/income-statement ────────────────────────────────────

// IncomeStatement returns revenue, expenses, and net income for a date range.
// Query params: from=YYYY-MM-DD, to=YYYY-MM-DD (both required).
func (h *ReportsHandler) IncomeStatement(c *gin.Context) {
	fromStr := c.Query("from")
	toStr := c.Query("to")
	if fromStr == "" || toStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to query parameters are required (YYYY-MM-DD)"})
		return
	}
	if _, err := time.Parse("2006-01-02", fromStr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from must be YYYY-MM-DD"})
		return
	}
	if _, err := time.Parse("2006-01-02", toStr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to must be YYYY-MM-DD"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	q := db.Rebind(`
		SELECT
		    a.id,
		    a.code,
		    a.name,
		    a.account_type,
		    COALESCE(SUM(jl.debit_amount),  0) AS total_debit,
		    COALESCE(SUM(jl.credit_amount), 0) AS total_credit
		FROM accounts a
		LEFT JOIN journal_lines jl ON jl.account_id = a.id
		LEFT JOIN journal_entries je
		       ON je.id = jl.entry_id
		      AND je.status = 'posted'
		      AND je.date >= ?
		      AND je.date <= ?
		WHERE a.account_type IN ('revenue','expense')
		GROUP BY a.id, a.code, a.name, a.account_type
		ORDER BY a.code`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q, fromStr, toStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	resp := incomeStatementResponse{
		From:     fromStr,
		To:       toStr,
		Revenue:  []incomeStatementLine{},
		Expenses: []incomeStatementLine{},
	}

	for rows.Next() {
		var id, code, name, accountType string
		var totalDebit, totalCredit float64
		if err := rows.Scan(&id, &code, &name, &accountType, &totalDebit, &totalCredit); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}

		line := incomeStatementLine{AccountID: id, Code: code, Name: name}
		switch accountType {
		case "revenue":
			// Revenue increases with credits; normal balance is credit.
			line.Amount = totalCredit - totalDebit
			if line.Amount != 0 {
				resp.Revenue = append(resp.Revenue, line)
				resp.TotalRevenue += line.Amount
			}
		case "expense":
			// Expenses increase with debits; normal balance is debit.
			line.Amount = totalDebit - totalCredit
			if line.Amount != 0 {
				resp.Expenses = append(resp.Expenses, line)
				resp.TotalExpenses += line.Amount
			}
		}
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	resp.NetIncome = resp.TotalRevenue - resp.TotalExpenses
	c.JSON(http.StatusOK, resp)
}

// ─── GET /api/v1/reports/general-ledger ──────────────────────────────────────

// GeneralLedger returns all posted journal lines for a given account code within
// a date range, with a running balance calculated in Go.
// Query params: account_code=XXXX (required), from=YYYY-MM-DD, to=YYYY-MM-DD.
func (h *ReportsHandler) GeneralLedger(c *gin.Context) {
	accountCode := c.Query("account_code")
	if accountCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_code query parameter is required"})
		return
	}
	fromStr := c.Query("from")
	toStr := c.Query("to")
	if fromStr != "" {
		if _, err := time.Parse("2006-01-02", fromStr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from must be YYYY-MM-DD"})
			return
		}
	}
	if toStr != "" {
		if _, err := time.Parse("2006-01-02", toStr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "to must be YYYY-MM-DD"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Resolve account by code.
	acctQ := db.Rebind(`SELECT id, name FROM accounts WHERE code = ? AND is_active = 1`, h.usePostgres)
	var accountID, accountName string
	if err := h.db.QueryRowContext(ctx, acctQ, accountCode).Scan(&accountID, &accountName); err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Compute opening balance: all posted lines before fromStr (if supplied).
	var openingBalance float64
	if fromStr != "" {
		obQ := db.Rebind(`
			SELECT
			    COALESCE(SUM(jl.debit_amount),  0),
			    COALESCE(SUM(jl.credit_amount), 0)
			FROM journal_lines jl
			JOIN journal_entries je ON je.id = jl.entry_id AND je.status = 'posted'
			WHERE jl.account_id = ? AND je.date < ?`, h.usePostgres)
		var obDebit, obCredit float64
		if err := h.db.QueryRowContext(ctx, obQ, accountID, fromStr).Scan(&obDebit, &obCredit); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		// Use debit - credit as the running balance basis (adjusted per account type
		// at display time by the consumer; we expose raw debit-minus-credit here).
		openingBalance = obDebit - obCredit
	}

	// Build the main ledger query with optional date filters.
	baseQ := `
		SELECT
		    je.id,
		    je.reference,
		    je.date,
		    COALESCE(jl.description, je.description) AS description,
		    COALESCE(jl.debit_amount,  0) AS debit,
		    COALESCE(jl.credit_amount, 0) AS credit
		FROM journal_lines jl
		JOIN journal_entries je ON je.id = jl.entry_id AND je.status = 'posted'
		WHERE jl.account_id = ?`

	args := []any{accountID}

	if fromStr != "" {
		baseQ += " AND je.date >= ?"
		args = append(args, fromStr)
	}
	if toStr != "" {
		baseQ += " AND je.date <= ?"
		args = append(args, toStr)
	}
	baseQ += " ORDER BY je.date ASC, je.reference ASC"

	rows, err := h.db.QueryContext(ctx, db.Rebind(baseQ, h.usePostgres), args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	resp := generalLedgerResponse{
		AccountCode:    accountCode,
		AccountName:    accountName,
		From:           fromStr,
		To:             toStr,
		OpeningBalance: openingBalance,
		Lines:          []generalLedgerLine{},
	}

	runningBalance := openingBalance
	for rows.Next() {
		var entryID, reference, dateStr, description string
		var debit, credit float64
		if err := rows.Scan(&entryID, &reference, &dateStr, &description, &debit, &credit); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		// Trim timestamp to date-only string if the DB returns a full timestamp.
		if len(dateStr) > 10 {
			dateStr = dateStr[:10]
		}
		runningBalance += debit - credit
		resp.Lines = append(resp.Lines, generalLedgerLine{
			EntryID:     entryID,
			Reference:   reference,
			Date:        dateStr,
			Description: description,
			Debit:       debit,
			Credit:      credit,
			Balance:     runningBalance,
		})
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	resp.ClosingBalance = runningBalance
	c.JSON(http.StatusOK, resp)
}

// ─── GET /api/v1/reports/ar-aging ────────────────────────────────────────────

// ARaging returns accounts receivable aging for all unpaid (status='sent') invoices.
// Buckets: current (≤0 days overdue), 1-30, 31-60, 61-90, 91+ days.
func (h *ReportsHandler) ARaging(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	today := time.Now().Format("2006-01-02")

	q := db.Rebind(`
		SELECT
		    i.id,
		    i.invoice_number,
		    co.name AS contact_name,
		    i.total_amount,
		    i.due_date
		FROM invoices i
		JOIN contacts co ON co.id = i.contact_id
		WHERE i.status = 'sent'
		ORDER BY i.due_date ASC`, h.usePostgres)

	rows, err := h.db.QueryContext(ctx, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	todayTime, _ := time.Parse("2006-01-02", today)

	resp := arAgingResponse{
		AsOf:      today,
		Current:   []arAgingLine{},
		Days1To30: []arAgingLine{},
		Days31To60: []arAgingLine{},
		Days61To90: []arAgingLine{},
		Days91Plus: []arAgingLine{},
	}

	for rows.Next() {
		var invoiceID, invoiceNumber, contactName, dueDateStr string
		var totalAmount float64
		if err := rows.Scan(&invoiceID, &invoiceNumber, &contactName, &totalAmount, &dueDateStr); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		// Trim to date-only.
		if len(dueDateStr) > 10 {
			dueDateStr = dueDateStr[:10]
		}
		dueDate, err := time.Parse("2006-01-02", dueDateStr)
		if err != nil {
			// Skip malformed due dates rather than aborting the entire report.
			continue
		}

		daysOverdue := int(todayTime.Sub(dueDate).Hours() / 24)
		var bucket string
		switch {
		case daysOverdue <= 0:
			bucket = "current"
		case daysOverdue <= 30:
			bucket = "1-30"
		case daysOverdue <= 60:
			bucket = "31-60"
		case daysOverdue <= 90:
			bucket = "61-90"
		default:
			bucket = "91+"
		}

		line := arAgingLine{
			InvoiceID:     invoiceID,
			InvoiceNumber: invoiceNumber,
			ContactName:   contactName,
			TotalAmount:   totalAmount,
			DueDate:       dueDateStr,
			DaysOverdue:   daysOverdue,
			Bucket:        bucket,
		}

		switch bucket {
		case "current":
			resp.Current = append(resp.Current, line)
			resp.TotalCurrent += totalAmount
		case "1-30":
			resp.Days1To30 = append(resp.Days1To30, line)
			resp.TotalDays1To30 += totalAmount
		case "31-60":
			resp.Days31To60 = append(resp.Days31To60, line)
			resp.TotalDays31To60 += totalAmount
		case "61-90":
			resp.Days61To90 = append(resp.Days61To90, line)
			resp.TotalDays61To90 += totalAmount
		default:
			resp.Days91Plus = append(resp.Days91Plus, line)
			resp.TotalDays91Plus += totalAmount
		}
		resp.GrandTotal += totalAmount
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows error"})
		return
	}

	c.JSON(http.StatusOK, resp)
}
