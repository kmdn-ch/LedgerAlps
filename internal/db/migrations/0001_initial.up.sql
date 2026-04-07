-- LedgerAlps — Initial schema (SQLite compatible, PostgreSQL compatible)
-- Convention: use TEXT for UUIDs (both DBs), REAL for amounts, INTEGER for booleans (SQLite)

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    is_admin      INTEGER NOT NULL DEFAULT 0,
    is_active     INTEGER NOT NULL DEFAULT 1,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS accounts (
    id           TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    code         TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    account_type TEXT NOT NULL CHECK(account_type IN ('asset','liability','equity','revenue','expense')),
    description  TEXT NOT NULL DEFAULT '',
    is_active    INTEGER NOT NULL DEFAULT 1,
    parent_id    TEXT REFERENCES accounts(id),
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS fiscal_years (
    id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    name       TEXT NOT NULL UNIQUE,
    start_date DATE NOT NULL,
    end_date   DATE NOT NULL,
    is_closed  INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK(end_date > start_date)
);

CREATE TABLE IF NOT EXISTS journal_entries (
    id              TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    reference       TEXT NOT NULL UNIQUE,
    date            DATE NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'draft'
                        CHECK(status IN ('draft','posted','reversed')),
    fiscal_year_id  TEXT REFERENCES fiscal_years(id),
    integrity_hash  TEXT,           -- SHA-256 hash when posted (CO art. 957a)
    is_reversal     INTEGER NOT NULL DEFAULT 0,
    reversal_of_id  TEXT REFERENCES journal_entries(id),
    created_by_id   TEXT NOT NULL REFERENCES users(id),
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Prevent UPDATE on posted entries (CO art. 957a — immuabilité des écritures validées)
CREATE TRIGGER IF NOT EXISTS trg_journal_entries_no_update
BEFORE UPDATE ON journal_entries
FOR EACH ROW
WHEN OLD.status = 'posted'
BEGIN
    SELECT RAISE(ABORT, 'Cannot modify a posted journal entry (CO art. 957a)');
END;

-- Prevent DELETE on posted entries (CO art. 957a — conservation 10 ans)
CREATE TRIGGER IF NOT EXISTS trg_journal_entries_no_delete
BEFORE DELETE ON journal_entries
FOR EACH ROW
WHEN OLD.status = 'posted'
BEGIN
    SELECT RAISE(ABORT, 'Cannot delete a posted journal entry (CO art. 957a)');
END;

CREATE TABLE IF NOT EXISTS journal_lines (
    id             TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    entry_id       TEXT NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
    account_id     TEXT NOT NULL REFERENCES accounts(id),
    debit_amount   REAL,
    credit_amount  REAL,
    description    TEXT NOT NULL DEFAULT '',
    sequence       INTEGER NOT NULL DEFAULT 0,
    CHECK(
        (debit_amount IS NOT NULL AND credit_amount IS NULL) OR
        (debit_amount IS NULL AND credit_amount IS NOT NULL)
    )
);

CREATE TABLE IF NOT EXISTS contacts (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    contact_type      TEXT NOT NULL CHECK(contact_type IN ('customer','supplier','both')),
    name              TEXT NOT NULL,
    email             TEXT,
    phone             TEXT,
    address           TEXT,
    city              TEXT,
    postal_code       TEXT,
    country           TEXT NOT NULL DEFAULT 'CH',
    iban              TEXT,
    qr_iban           TEXT,
    vat_number        TEXT,
    payment_term_days INTEGER NOT NULL DEFAULT 30,
    is_active         INTEGER NOT NULL DEFAULT 1,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS invoices (
    id               TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    invoice_number   TEXT NOT NULL UNIQUE,
    contact_id       TEXT NOT NULL REFERENCES contacts(id),
    status           TEXT NOT NULL DEFAULT 'draft'
                         CHECK(status IN ('draft','sent','paid','cancelled','archived')),
    issue_date       DATE NOT NULL,
    due_date         DATE NOT NULL,
    currency         TEXT NOT NULL DEFAULT 'CHF',
    subtotal_amount  REAL NOT NULL DEFAULT 0,
    vat_amount       REAL NOT NULL DEFAULT 0,
    total_amount     REAL NOT NULL DEFAULT 0,  -- rounded to 0.05 CHF
    vat_rate         REAL NOT NULL DEFAULT 0.081,
    notes            TEXT,
    terms            TEXT,
    qr_reference     TEXT,
    journal_entry_id TEXT REFERENCES journal_entries(id),
    fiscal_year_id   TEXT REFERENCES fiscal_years(id),
    created_by_id    TEXT NOT NULL REFERENCES users(id),
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS invoice_lines (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    invoice_id  TEXT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    description TEXT NOT NULL,
    quantity    REAL NOT NULL DEFAULT 1,
    unit_price  REAL NOT NULL DEFAULT 0,
    vat_rate    REAL NOT NULL DEFAULT 0.081,
    line_total  REAL NOT NULL DEFAULT 0,
    sequence    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id           TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    user_id      TEXT NOT NULL REFERENCES users(id),
    action       TEXT NOT NULL,
    table_name   TEXT NOT NULL,
    record_id    TEXT NOT NULL,
    before_state TEXT,   -- JSON, personal data masked (nLPD)
    after_state  TEXT,   -- JSON, personal data masked (nLPD)
    ip_address   TEXT,
    entry_hash       TEXT NOT NULL,   -- SHA-256 of this record
    prev_hash        TEXT,            -- SHA-256 chained from previous entry (CO art. 957a)
    sequence_number  INTEGER NOT NULL, -- Monotonic counter for chain continuity verification
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ─── Indexes ──────────────────────────────────────────────────────────────────

CREATE INDEX IF NOT EXISTS idx_journal_entries_date        ON journal_entries(date);
CREATE INDEX IF NOT EXISTS idx_journal_entries_status      ON journal_entries(status);
CREATE INDEX IF NOT EXISTS idx_journal_entries_ref         ON journal_entries(reference);
CREATE INDEX IF NOT EXISTS idx_journal_entries_date_status ON journal_entries(date, status);
CREATE INDEX IF NOT EXISTS idx_journal_entries_fiscal_year ON journal_entries(fiscal_year_id);
CREATE INDEX IF NOT EXISTS idx_journal_lines_entry         ON journal_lines(entry_id);
CREATE INDEX IF NOT EXISTS idx_journal_lines_account       ON journal_lines(account_id);
CREATE INDEX IF NOT EXISTS idx_journal_lines_entry_account ON journal_lines(entry_id, account_id);
CREATE INDEX IF NOT EXISTS idx_invoices_contact            ON invoices(contact_id);
CREATE INDEX IF NOT EXISTS idx_invoices_status             ON invoices(status);
CREATE INDEX IF NOT EXISTS idx_invoices_issue_date         ON invoices(issue_date);
CREATE INDEX IF NOT EXISTS idx_audit_logs_table_record     ON audit_logs(table_name, record_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user             ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_sequence         ON audit_logs(sequence_number);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at       ON audit_logs(created_at DESC);
