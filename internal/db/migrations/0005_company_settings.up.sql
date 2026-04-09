-- LedgerAlps — Migration 0005: company/tenant settings (singleton row)
-- Compatible with SQLite and PostgreSQL (no DB-specific syntax).

CREATE TABLE IF NOT EXISTS company_settings (
    id                      TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    company_name            TEXT NOT NULL DEFAULT '',
    legal_form              TEXT NOT NULL DEFAULT '',
    address_street          TEXT NOT NULL DEFAULT '',
    address_postal_code     TEXT NOT NULL DEFAULT '',
    address_city            TEXT NOT NULL DEFAULT '',
    address_country         TEXT NOT NULL DEFAULT 'CH',
    che_number              TEXT NOT NULL DEFAULT '',
    vat_number              TEXT NOT NULL DEFAULT '',
    iban                    TEXT NOT NULL DEFAULT '',
    fiscal_year_start_month INTEGER NOT NULL DEFAULT 1
                               CHECK(fiscal_year_start_month BETWEEN 1 AND 12),
    currency                TEXT NOT NULL DEFAULT 'CHF',
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
