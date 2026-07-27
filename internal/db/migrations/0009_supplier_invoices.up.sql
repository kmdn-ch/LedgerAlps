-- Supplier invoices (factures fournisseurs / factures d'achat).
--
-- Without these, input VAT (impôt préalable, AFC form 318 line 400) cannot be
-- captured, so the VAT declaration reports zero deductible and the business
-- over-declares what it owes. Expenses could previously only be recorded as
-- manual journal entries, which carry no VAT breakdown.
--
-- Kept separate from `invoices` on purpose: a sales invoice is issued by us
-- (we own the sequential number the CO requires to be gapless) whereas a
-- supplier invoice is received (the supplier owns its reference). The
-- accounting entries are mirror images — expense + input VAT against
-- accounts payable, rather than receivable against revenue + VAT payable.
CREATE TABLE IF NOT EXISTS supplier_invoices (
    id                   TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    supplier_id          TEXT NOT NULL REFERENCES contacts(id),

    -- The supplier's own invoice number, as printed on the document.
    supplier_reference   TEXT NOT NULL,

    -- draft   : captured, not yet in the books
    -- booked  : posted to the journal, counts for VAT
    -- paid    : settled
    -- cancelled
    status               TEXT NOT NULL DEFAULT 'draft',

    issue_date           DATE NOT NULL,
    due_date             DATE,
    currency             TEXT NOT NULL DEFAULT 'CHF',

    subtotal_amount      REAL NOT NULL DEFAULT 0,   -- HT
    vat_amount           REAL NOT NULL DEFAULT 0,   -- input VAT (déductible)
    total_amount         REAL NOT NULL DEFAULT 0,   -- TTC
    vat_rate             REAL NOT NULL DEFAULT 0.081,
    amount_paid          REAL NOT NULL DEFAULT 0,

    -- Charge account to debit (e.g. 4000 marchandises, 6000 loyer).
    expense_account_code TEXT,

    notes                TEXT,
    journal_entry_id     TEXT REFERENCES journal_entries(id),
    fiscal_year_id       TEXT REFERENCES fiscal_years(id),
    created_by_id        TEXT REFERENCES users(id),
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Booking the same supplier document twice is a duplicate-payment risk;
    -- the database refuses it rather than relying on the operator noticing.
    UNIQUE (supplier_id, supplier_reference)
);

CREATE INDEX IF NOT EXISTS idx_supplier_invoices_supplier ON supplier_invoices(supplier_id);
CREATE INDEX IF NOT EXISTS idx_supplier_invoices_status   ON supplier_invoices(status);
CREATE INDEX IF NOT EXISTS idx_supplier_invoices_issued   ON supplier_invoices(issue_date);

-- Lines carry their own VAT rate: one supplier invoice routinely mixes rates
-- (8.1% on goods, 2.6% on food/books), and each rate must land in its own
-- bucket on the declaration.
CREATE TABLE IF NOT EXISTS supplier_invoice_lines (
    id                   TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    supplier_invoice_id  TEXT NOT NULL REFERENCES supplier_invoices(id) ON DELETE CASCADE,
    description          TEXT NOT NULL,
    quantity             REAL NOT NULL DEFAULT 1,
    unit_price           REAL NOT NULL DEFAULT 0,
    vat_rate             REAL NOT NULL DEFAULT 0.081,
    line_total           REAL NOT NULL DEFAULT 0,
    expense_account_code TEXT,
    sequence             INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_supplier_invoice_lines_parent
    ON supplier_invoice_lines(supplier_invoice_id);
