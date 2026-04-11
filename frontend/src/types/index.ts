// LedgerAlps — Types TypeScript (miroir des schémas Pydantic backend)

// ─── Auth ─────────────────────────────────────────────────────────────────────
export interface User {
  id: string
  email: string
  name: string
  is_active: boolean
  is_admin: boolean
  created_at: string
}

export interface TokenResponse {
  access_token: string
  refresh_token: string
  token_type: string
}

// ─── Plan comptable ───────────────────────────────────────────────────────────
export type AccountType = 'asset' | 'liability' | 'equity' | 'revenue' | 'expense'

export interface Account {
  id: string
  number: string
  name: string
  account_type: AccountType
  is_active: boolean
  parent_id: string | null
}

export interface AccountBalance {
  account_number: string
  account_name: string
  account_type: string
  debit: string
  credit: string
  balance: string
  as_of: string | null
}

// ─── Journal ──────────────────────────────────────────────────────────────────
export type JournalEntryStatus = 'draft' | 'posted' | 'reversed'

export interface JournalLine {
  id: string
  debit_account_id: string | null
  credit_account_id: string | null
  amount: string
  currency: string
  amount_chf: string
  description: string | null
}

export interface JournalEntry {
  id: string
  date: string
  reference: string
  description: string
  status: JournalEntryStatus
  posted_at: string | null
  lines: JournalLine[]
  created_at: string
}

export interface JournalEntryCreate {
  date: string
  description: string
  lines: JournalLineCreate[]
  reference?: string
}

export interface JournalLineCreate {
  debit_account?: string
  credit_account?: string
  amount: number
  currency?: string
  description?: string
  vat_code?: string
}

// ─── Contacts ─────────────────────────────────────────────────────────────────
export type ContactType = 'customer' | 'supplier' | 'both'

export interface Contact {
  id: string
  contact_type: ContactType
  is_company: boolean
  name: string
  legal_name: string | null
  address: string | null
  postal_code: string | null
  city: string | null
  country: string
  email: string | null
  phone: string | null
  iban: string | null
  qr_iban: string | null
  vat_number: string | null
  uid_number: string | null
  payment_term_days: number
  notes: string | null
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface ContactCreate {
  contact_type: ContactType
  is_company: boolean
  name: string
  legal_name?: string
  address_line1?: string
  address_line2?: string
  postal_code?: string
  city?: string
  country: string
  uid_number?: string
  vat_number?: string
  email?: string
  phone?: string
  payment_term_days: number
  iban?: string
  currency: string
  notes?: string
}

// ─── Factures ─────────────────────────────────────────────────────────────────
export type DocumentStatus = 'draft' | 'sent' | 'paid' | 'overdue' | 'cancelled' | 'archived'
export type DocumentType   = 'invoice' | 'quote' | 'credit_note'

export interface InvoiceLine {
  id: string
  invoice_id: string
  description: string
  quantity: number
  unit: string | null
  unit_price: number
  discount_pct: number
  vat_rate: number
  line_total: number
  sequence: number
}

export interface Invoice {
  id: string
  invoice_number: string
  document_type: DocumentType
  contact_id: string
  status: DocumentStatus
  issue_date: string
  due_date: string | null
  currency: string
  subtotal_amount: number
  vat_amount: number
  total_amount: number
  vat_rate: number
  amount_paid: number
  notes: string | null
  terms: string | null
  qr_reference: string | null
  lines: InvoiceLine[]
  created_at: string
  updated_at: string
}

// ─── TVA ──────────────────────────────────────────────────────────────────────
export interface VATCompute {
  base_amount: string
  vat_rate: string
  vat_amount: string
  total_amount: string
  vat_code: string
}

// ─── Pagination ───────────────────────────────────────────────────────────────
export interface Paginated<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

// ─── UI helpers ───────────────────────────────────────────────────────────────
export interface SelectOption {
  value: string
  label: string
}
