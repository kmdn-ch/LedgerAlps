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
	ID           string    `db:"id"            json:"id"`
	Email        string    `db:"email"         json:"email"`
	Name         string    `db:"name"          json:"name"`
	PasswordHash string    `db:"password_hash" json:"-"` // never expose hash
	IsAdmin      bool      `db:"is_admin"      json:"is_admin"`
	IsActive     bool      `db:"is_active"     json:"is_active"`
	CreatedAt    time.Time `db:"created_at"    json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"    json:"updated_at"`
}

type Account struct {
	ID          string      `db:"id"           json:"id"`
	Code        string      `db:"code"         json:"code"`
	Name        string      `db:"name"         json:"name"`
	AccountType AccountType `db:"account_type" json:"account_type"`
	Description string      `db:"description"  json:"description"`
	IsActive    bool        `db:"is_active"    json:"is_active"`
	ParentID    *string     `db:"parent_id"    json:"parent_id,omitempty"`
	CreatedAt   time.Time   `db:"created_at"   json:"created_at"`
	UpdatedAt   time.Time   `db:"updated_at"   json:"updated_at"`
}

type FiscalYear struct {
	ID        string    `db:"id"         json:"id"`
	Name      string    `db:"name"        json:"name"`
	StartDate time.Time `db:"start_date"  json:"start_date"`
	EndDate   time.Time `db:"end_date"    json:"end_date"`
	IsClosed  bool      `db:"is_closed"   json:"is_closed"`
	CreatedAt time.Time `db:"created_at"  json:"created_at"`
	UpdatedAt time.Time `db:"updated_at"  json:"updated_at"`
}

type JournalEntry struct {
	ID            string             `db:"id"              json:"id"`
	Reference     string             `db:"reference"       json:"reference"`
	Date          time.Time          `db:"date"            json:"date"`
	Description   string             `db:"description"     json:"description"`
	Status        JournalEntryStatus `db:"status"          json:"status"`
	FiscalYearID  *string            `db:"fiscal_year_id"  json:"fiscal_year_id,omitempty"`
	IntegrityHash *string            `db:"integrity_hash"  json:"integrity_hash,omitempty"`
	IsReversal    bool               `db:"is_reversal"     json:"is_reversal"`
	ReversalOfID  *string            `db:"reversal_of_id"  json:"reversal_of_id,omitempty"`
	CreatedByID   string             `db:"created_by_id"   json:"created_by_id"`
	CreatedAt     time.Time          `db:"created_at"      json:"created_at"`
	UpdatedAt     time.Time          `db:"updated_at"      json:"updated_at"`

	Lines []JournalLine `db:"-" json:"lines,omitempty"`
}

type JournalLine struct {
	ID           string   `db:"id"            json:"id"`
	EntryID      string   `db:"entry_id"      json:"entry_id"`
	AccountID    string   `db:"account_id"    json:"account_id"`
	DebitAmount  *float64 `db:"debit_amount"  json:"debit_amount,omitempty"`
	CreditAmount *float64 `db:"credit_amount" json:"credit_amount,omitempty"`
	Description  string   `db:"description"   json:"description"`
	Sequence     int      `db:"sequence"      json:"sequence"`
}

type Contact struct {
	ID              string      `db:"id"               json:"id"`
	ContactType     ContactType `db:"contact_type"     json:"contact_type"`
	IsCompany       bool        `db:"is_company"       json:"is_company"`
	Name            string      `db:"name"             json:"name"`
	LegalName       *string     `db:"legal_name"       json:"legal_name,omitempty"`
	Email           *string     `db:"email"            json:"email,omitempty"`
	Phone           *string     `db:"phone"            json:"phone,omitempty"`
	Address         *string     `db:"address"          json:"address,omitempty"`
	City            *string     `db:"city"             json:"city,omitempty"`
	PostalCode      *string     `db:"postal_code"      json:"postal_code,omitempty"`
	Country         string      `db:"country"          json:"country"`
	IBAN            *string     `db:"iban"             json:"iban,omitempty"`
	QRIBAN          *string     `db:"qr_iban"          json:"qr_iban,omitempty"`
	VATNumber       *string     `db:"vat_number"       json:"vat_number,omitempty"`
	UIDNumber       *string     `db:"uid_number"       json:"uid_number,omitempty"`
	PaymentTermDays int         `db:"payment_term_days" json:"payment_term_days"`
	Notes           *string     `db:"notes"            json:"notes,omitempty"`
	IsActive        bool        `db:"is_active"        json:"is_active"`
	CreatedAt       time.Time   `db:"created_at"       json:"created_at"`
	UpdatedAt       time.Time   `db:"updated_at"       json:"updated_at"`
}

type Invoice struct {
	ID             string        `db:"id"              json:"id"`
	InvoiceNumber  string        `db:"invoice_number"  json:"invoice_number"`
	DocumentType   string        `db:"document_type"   json:"document_type"`
	ContactID      string        `db:"contact_id"      json:"contact_id"`
	Status         InvoiceStatus `db:"status"          json:"status"`
	IssueDate      time.Time     `db:"issue_date"      json:"issue_date"`
	DueDate        time.Time     `db:"due_date"        json:"due_date"`
	Currency       string        `db:"currency"        json:"currency"`
	SubtotalAmount float64       `db:"subtotal_amount" json:"subtotal_amount"`
	VATAmount      float64       `db:"vat_amount"      json:"vat_amount"`
	TotalAmount    float64       `db:"total_amount"    json:"total_amount"`
	VATRate        float64       `db:"vat_rate"        json:"vat_rate"`
	AmountPaid     float64       `db:"amount_paid"     json:"amount_paid"`
	Notes          *string       `db:"notes"           json:"notes,omitempty"`
	Terms          *string       `db:"terms"           json:"terms,omitempty"`
	QRReference    *string       `db:"qr_reference"    json:"qr_reference,omitempty"`
	JournalEntryID *string       `db:"journal_entry_id" json:"journal_entry_id,omitempty"`
	FiscalYearID   *string       `db:"fiscal_year_id"  json:"fiscal_year_id,omitempty"`
	CreatedByID    string        `db:"created_by_id"   json:"created_by_id"`
	CreatedAt      time.Time     `db:"created_at"      json:"created_at"`
	UpdatedAt      time.Time     `db:"updated_at"      json:"updated_at"`

	Lines   []InvoiceLine `db:"-" json:"lines,omitempty"`
	Contact *Contact      `db:"-" json:"contact,omitempty"`
}

type InvoiceLine struct {
	ID          string   `db:"id"           json:"id"`
	InvoiceID   string   `db:"invoice_id"   json:"invoice_id"`
	Description string   `db:"description"  json:"description"`
	Quantity    float64  `db:"quantity"     json:"quantity"`
	Unit        *string  `db:"unit"         json:"unit,omitempty"`
	UnitPrice   float64  `db:"unit_price"   json:"unit_price"`
	DiscountPct float64  `db:"discount_pct" json:"discount_pct"`
	VATRate     float64  `db:"vat_rate"     json:"vat_rate"`
	LineTotal   float64  `db:"line_total"   json:"line_total"`
	Sequence    int      `db:"sequence"     json:"sequence"`
}

type AuditLog struct {
	ID             string    `db:"id"              json:"id"`
	UserID         string    `db:"user_id"         json:"user_id"`
	Action         string    `db:"action"          json:"action"`
	TableName      string    `db:"table_name"      json:"table_name"`
	RecordID       string    `db:"record_id"       json:"record_id"`
	BeforeState    *string   `db:"before_state"    json:"before_state,omitempty"`
	AfterState     *string   `db:"after_state"     json:"after_state,omitempty"`
	IPAddress      *string   `db:"ip_address"      json:"ip_address,omitempty"`
	EntryHash      string    `db:"entry_hash"      json:"entry_hash"`
	PrevHash       *string   `db:"prev_hash"       json:"prev_hash,omitempty"`
	SequenceNumber int64     `db:"sequence_number" json:"sequence_number"`
	CreatedAt      time.Time `db:"created_at"      json:"created_at"`
}

// ─── Payments ─────────────────────────────────────────────────────────────────

type PaymentMethod string

const (
	PaymentMethodBankTransfer PaymentMethod = "bank_transfer"
	PaymentMethodCash         PaymentMethod = "cash"
	PaymentMethodCard         PaymentMethod = "card"
	PaymentMethodCheck        PaymentMethod = "check"
	PaymentMethodOther        PaymentMethod = "other"
)

type Payment struct {
	ID             string        `db:"id"               json:"id"`
	InvoiceID      string        `db:"invoice_id"       json:"invoice_id"`
	Amount         float64       `db:"amount"           json:"amount"`
	PaymentDate    time.Time     `db:"payment_date"     json:"payment_date"`
	Method         PaymentMethod `db:"method"           json:"method"`
	Reference      *string       `db:"reference"        json:"reference,omitempty"`
	JournalEntryID *string       `db:"journal_entry_id" json:"journal_entry_id,omitempty"`
	CreatedByID    string        `db:"created_by_id"    json:"created_by_id"`
	CreatedAt      time.Time     `db:"created_at"       json:"created_at"`
	UpdatedAt      time.Time     `db:"updated_at"       json:"updated_at"`
}

// ─── Company settings ─────────────────────────────────────────────────────────

// CompanySettings holds the one-row singleton for this installation's tenant profile.
type CompanySettings struct {
	ID                   string    `db:"id"                       json:"id"`
	CompanyName          string    `db:"company_name"             json:"company_name"`
	LegalForm            string    `db:"legal_form"               json:"legal_form"`
	AddressStreet        string    `db:"address_street"           json:"address_street"`
	AddressPostalCode    string    `db:"address_postal_code"      json:"address_postal_code"`
	AddressCity          string    `db:"address_city"             json:"address_city"`
	AddressCountry       string    `db:"address_country"          json:"address_country"`
	CheNumber            string    `db:"che_number"               json:"che_number"`
	VatNumber            string    `db:"vat_number"               json:"vat_number"`
	IBAN                 string    `db:"iban"                     json:"iban"`
	FiscalYearStartMonth int       `db:"fiscal_year_start_month"  json:"fiscal_year_start_month"`
	Currency             string    `db:"currency"                 json:"currency"`
	CreatedAt            time.Time `db:"created_at"               json:"created_at"`
	UpdatedAt            time.Time `db:"updated_at"               json:"updated_at"`
}
