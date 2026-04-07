package models

import (
	"time"
)

// ─── Enums ────────────────────────────────────────────────────────────────────

type AccountType string

const (
	AccountTypeAsset     AccountType = "asset"
	AccountTypeLiability AccountType = "liability"
	AccountTypeEquity    AccountType = "equity"
	AccountTypeRevenue   AccountType = "revenue"
	AccountTypeExpense   AccountType = "expense"
)

type JournalEntryStatus string

const (
	JournalEntryStatusDraft    JournalEntryStatus = "draft"
	JournalEntryStatusPosted   JournalEntryStatus = "posted"
	JournalEntryStatusReversed JournalEntryStatus = "reversed"
)

type InvoiceStatus string

const (
	InvoiceStatusDraft     InvoiceStatus = "draft"
	InvoiceStatusSent      InvoiceStatus = "sent"
	InvoiceStatusPaid      InvoiceStatus = "paid"
	InvoiceStatusCancelled InvoiceStatus = "cancelled"
	InvoiceStatusArchived  InvoiceStatus = "archived"
)

type ContactType string

const (
	ContactTypeCustomer ContactType = "customer"
	ContactTypeSupplier ContactType = "supplier"
	ContactTypeBoth     ContactType = "both"
)

// ─── Core entities ────────────────────────────────────────────────────────────

type User struct {
	ID           string    `db:"id"`
	Email        string    `db:"email"`
	Name         string    `db:"name"`
	PasswordHash string    `db:"password_hash"`
	IsAdmin      bool      `db:"is_admin"`
	IsActive     bool      `db:"is_active"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

type Account struct {
	ID          string      `db:"id"`
	Code        string      `db:"code"`
	Name        string      `db:"name"`
	AccountType AccountType `db:"account_type"`
	Description string      `db:"description"`
	IsActive    bool        `db:"is_active"`
	ParentID    *string     `db:"parent_id"`
	CreatedAt   time.Time   `db:"created_at"`
	UpdatedAt   time.Time   `db:"updated_at"`
}

type FiscalYear struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	StartDate time.Time `db:"start_date"`
	EndDate   time.Time `db:"end_date"`
	IsClosed  bool      `db:"is_closed"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type JournalEntry struct {
	ID              string             `db:"id"`
	Reference       string             `db:"reference"`
	Date            time.Time          `db:"date"`
	Description     string             `db:"description"`
	Status          JournalEntryStatus `db:"status"`
	FiscalYearID    *string            `db:"fiscal_year_id"`
	IntegrityHash   *string            `db:"integrity_hash"`
	IsReversal      bool               `db:"is_reversal"`
	ReversalOfID    *string            `db:"reversal_of_id"`
	CreatedByID     string             `db:"created_by_id"`
	CreatedAt       time.Time          `db:"created_at"`
	UpdatedAt       time.Time          `db:"updated_at"`

	Lines []JournalLine `db:"-"` // loaded separately
}

type JournalLine struct {
	ID             string   `db:"id"`
	EntryID        string   `db:"entry_id"`
	AccountID      string   `db:"account_id"`
	DebitAmount    *float64 `db:"debit_amount"`
	CreditAmount   *float64 `db:"credit_amount"`
	Description    string   `db:"description"`
	Sequence       int      `db:"sequence"`
}

type Contact struct {
	ID               string      `db:"id"`
	ContactType      ContactType `db:"contact_type"`
	Name             string      `db:"name"`
	Email            *string     `db:"email"`
	Phone            *string     `db:"phone"`
	Address          *string     `db:"address"`
	City             *string     `db:"city"`
	PostalCode       *string     `db:"postal_code"`
	Country          string      `db:"country"`
	IBAN             *string     `db:"iban"`
	QRIBAN           *string     `db:"qr_iban"`
	VATNumber        *string     `db:"vat_number"`
	PaymentTermDays  int         `db:"payment_term_days"`
	IsActive         bool        `db:"is_active"`
	CreatedAt        time.Time   `db:"created_at"`
	UpdatedAt        time.Time   `db:"updated_at"`
}

type Invoice struct {
	ID              string        `db:"id"`
	InvoiceNumber   string        `db:"invoice_number"`
	ContactID       string        `db:"contact_id"`
	Status          InvoiceStatus `db:"status"`
	IssueDate       time.Time     `db:"issue_date"`
	DueDate         time.Time     `db:"due_date"`
	Currency        string        `db:"currency"`
	SubtotalAmount  float64       `db:"subtotal_amount"`
	VATAmount       float64       `db:"vat_amount"`
	TotalAmount     float64       `db:"total_amount"` // rounded to 0.05 CHF
	VATRate         float64       `db:"vat_rate"`
	Notes           *string       `db:"notes"`
	Terms           *string       `db:"terms"`
	QRReference     *string       `db:"qr_reference"`
	JournalEntryID  *string       `db:"journal_entry_id"`
	FiscalYearID    *string       `db:"fiscal_year_id"`
	CreatedByID     string        `db:"created_by_id"`
	CreatedAt       time.Time     `db:"created_at"`
	UpdatedAt       time.Time     `db:"updated_at"`

	Lines   []InvoiceLine `db:"-"`
	Contact *Contact      `db:"-"`
}

type InvoiceLine struct {
	ID          string  `db:"id"`
	InvoiceID   string  `db:"invoice_id"`
	Description string  `db:"description"`
	Quantity    float64 `db:"quantity"`
	UnitPrice   float64 `db:"unit_price"`
	VATRate     float64 `db:"vat_rate"`
	LineTotal   float64 `db:"line_total"`
	Sequence    int     `db:"sequence"`
}

type AuditLog struct {
	ID          string    `db:"id"`
	UserID      string    `db:"user_id"`
	Action      string    `db:"action"`
	TableName   string    `db:"table_name"`
	RecordID    string    `db:"record_id"`
	BeforeState *string   `db:"before_state"` // JSON, personal data masked
	AfterState  *string   `db:"after_state"`  // JSON, personal data masked
	IPAddress   *string   `db:"ip_address"`
	EntryHash   string    `db:"entry_hash"`   // SHA-256 of this record's fields
	PrevHash    *string   `db:"prev_hash"`    // SHA-256 of previous AuditLog.EntryHash (CO art. 957a)
	CreatedAt   time.Time `db:"created_at"`
}
