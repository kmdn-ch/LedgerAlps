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
  token_type: string
  // Pas de refresh_token : il est transmis dans un cookie HttpOnly, hors de
  // portée du JavaScript.
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
// Exactement les valeurs que l'API accepte et stocke. « En retard » n'en fait
// pas partie : c'est un état *déduit* de la date d'échéance (voir isOverdue),
// pas une décision qu'on enregistre. Le proposer ici a produit un bouton
// « Marquer en retard » que le serveur refusait systématiquement.
export type DocumentStatus = 'draft' | 'sent' | 'paid' | 'cancelled' | 'archived'

// Ce que l'utilisateur voit. « En retard » s'y ajoute parce qu'une facture
// envoyée dont l'échéance est passée doit se lire comme telle — mais cette
// valeur ne remonte jamais dans un changement de statut. Elle sert aussi de
// filtre de recherche, que le serveur traduit en « envoyée + échue ».
export type DisplayStatus = DocumentStatus | 'overdue'
// Une offre n'est jamais « payée » : personne ne doit rien dessus. Elle est
// acceptée en produisant la facture, refusée, ou expirée.
export type QuoteOutcome = 'accepted' | 'refused' | 'expired'
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
  // Offre dont cette facture est issue. L'offre n'est pas transformée : les deux
  // documents coexistent et se citent (CO art. 957a al. 2 ch. 5).
  converted_from_id: string | null
  // Issue commerciale d'une offre. Toujours null sur une facture.
  quote_outcome: QuoteOutcome | null
  // Facture que cette note de crédit annule (LTVA art. 27 al. 4). Null ailleurs.
  corrects_invoice_id: string | null
  // Nom du contact, résolu par jointure — la liste affichait un identifiant.
  contact_name?: string
  // Somme des notes de crédit rattachées, hors annulées. Une facture
  // entièrement créditée ne doit plus proposer de note de crédit.
  credited_amount: number
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

// ─── Paramètres société ───────────────────────────────────────────────────────
export interface CompanySettings {
  id: string
  company_name: string
  legal_form: string
  address_street: string
  address_postal_code: string
  address_city: string
  address_country: string
  che_number: string
  vat_number: string
  phone: string
  email: string
  iban: string
  fiscal_year_start_month: number
  currency: string
  logo_data?: string | null
}

// ─── UI helpers ───────────────────────────────────────────────────────────────
export interface SelectOption {
  value: string
  label: string
}

// ─── Sauvegardes ──────────────────────────────────────────────────────────────
export interface BackupItem {
  name: string
  size_bytes: number
  created_at: string
  encrypted: boolean
}

export interface PendingRestore {
  source_name: string
  requested_at: string
}

// ─── Maintenance & Système ────────────────────────────────────────────────────
export interface FindingDocument {
  id: string
  number: string
  detail?: string
}

export interface IntegrityFinding {
  severity: 'error' | 'warning' | 'info'
  check: string
  title: string
  detail: string
  action?: string
  count: number
  // Les pièces visées. Un constat sans elles oblige à chercher à la main dans
  // toute la liste, et c'est ce qui fait qu'on ne corrige pas.
  documents?: FindingDocument[]
}

export interface IntegrityReport {
  checked_at: string
  findings: IntegrityFinding[]
  clean: boolean
}

export interface SystemHealth {
  version: string
  database: { engine: string }
  counts: Record<string, number>
  backups?: {
    count: number; encrypted: number; directory: string
    newest?: string; newest_name?: string
  }
  network: { host: string; tls: boolean; loopback: boolean; insecure_opt_in: boolean }
  disk_encryption?: {
    status: 'encrypted' | 'not_encrypted' | 'unknown'
    mechanism?: string
    advisory: boolean
  }
  capabilities: Record<string, boolean>
  // Ce que la rétention nLPD art. 6 al. 4 fait réellement, en chiffres :
  // annoncer une durée sans montrer qu'elle s'applique était le défaut à éviter.
  personal_data?: {
    ip_retention_days: number
    event_retention_days: number
    security_events?: number
    ip_addresses_held?: number
    contacts_anonymised?: number
    invoices_recipient_reconstructed?: number
  }
  login_lockouts?: number
}

export interface ServerSettings {
  host: string
  tls_cert: string
  tls_key: string
  allow_insecure_http: boolean
}

// ─── Piste d'audit (CO art. 957a) ─────────────────────────────────────────────
export interface AuditLog {
  id: string
  user_id: string
  action: string
  table_name: string
  record_id: string
  before_state?: string
  after_state?: string
  ip_address?: string
  entry_hash: string
  prev_hash?: string
  sequence_number: number
  created_at: string
  // Résolu par jointure à la lecture : absent si l'auteur n'existe plus.
  user_name?: string
}

export interface AuditLogPage {
  items: AuditLog[]
  total: number
  limit: number
  offset: number
  pages: number
}

export interface ChainBreak {
  sequence_number: number
  id: string
  created_at: string
  kind: 'entry_altered' | 'link_broken' | 'sequence_gap' | 'anchor_invalid'
  detail: string
}

export interface ChainReport {
  verified: boolean
  entries: number
  // Entrées écrites avant la v1.4.6 : leur empreinte propre n'est pas
  // recalculable, mais leur chaînage l'est.
  legacy_entries: number
  first_sequence: number
  last_sequence: number
  breaks: ChainBreak[]
  truncated: boolean
  checked_at: string
}

// ─── Conformité & clôture ─────────────────────────────────────────────────────
export interface FiscalYear {
  id: string
  name: string
  start_date: string
  end_date: string
  is_closed: boolean
}

export interface RotateSecretResult {
  rotated: boolean
  secret_length: number
  sessions_revoked: number
  restart_required: boolean
  message: string
}

export interface AnonymiseResult {
  id: string
  label: string
  anonymised_at: string
  invoices_kept: number
  legal_basis: string[]
  what_was_erased: string[]
  what_was_kept: string[]
  // Ce que l'anonymisation NE fait pas : les sauvegardes déjà prises.
  backups_notice: string
}
