-- Sprint 9: document_type, amount_paid, unit, discount_pct
-- SQLite ALTER TABLE supports ADD COLUMN only (no constraints on new cols).

ALTER TABLE invoices      ADD COLUMN document_type TEXT NOT NULL DEFAULT 'invoice';
ALTER TABLE invoices      ADD COLUMN amount_paid   REAL NOT NULL DEFAULT 0;
ALTER TABLE invoice_lines ADD COLUMN unit          TEXT;
ALTER TABLE invoice_lines ADD COLUMN discount_pct  REAL NOT NULL DEFAULT 0;

-- Migrate vat_rate from decimal notation (0.081) to percentage (8.1).
-- The CHECK vat_rate < 1 targets only legacy rows; new inserts always use percentages.
UPDATE invoices      SET vat_rate = ROUND(vat_rate * 100, 4) WHERE vat_rate > 0 AND vat_rate < 1;
UPDATE invoice_lines SET vat_rate = ROUND(vat_rate * 100, 4) WHERE vat_rate > 0 AND vat_rate < 1;
