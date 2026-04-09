-- LedgerAlps — Migration 0004: payments table
-- Compatible with SQLite and PostgreSQL (no DB-specific syntax).

CREATE TABLE IF NOT EXISTS payments (
    id               TEXT PRIMARY KEY,
    invoice_id       TEXT NOT NULL REFERENCES invoices(id),
    amount           NUMERIC(15,2) NOT NULL CHECK(amount > 0),
    payment_date     DATE NOT NULL,
    method           TEXT NOT NULL CHECK(method IN ('bank_transfer','cash','card','check','other')),
    reference        TEXT,
    journal_entry_id TEXT REFERENCES journal_entries(id),
    created_by_id    TEXT NOT NULL REFERENCES users(id),
    created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_payments_invoice ON payments(invoice_id);
CREATE INDEX IF NOT EXISTS idx_payments_date    ON payments(payment_date);
