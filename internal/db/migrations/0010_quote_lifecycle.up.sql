-- Quote lifecycle: conversion link + commercial outcome.
--
-- A price offer and the invoice that follows it are two distinct documents.
-- Converting in place would destroy the offer, and the client already holds a
-- copy of it: the reference they quote back would no longer exist here. CO
-- art. 958f al. 3 requires that the link between a retained document and the
-- transaction stay guaranteed, and CO art. 957a al. 2 ch. 5 requires
-- traceability. Keeping both documents and linking them satisfies each.
--
-- Why two new columns rather than new `status` values: `invoices.status`
-- carries a CHECK constraint, and widening a CHECK is not portable — SQLite
-- has no ALTER TABLE ... DROP CONSTRAINT, so it would require rebuilding a
-- table that invoice_lines and payments reference. The document lifecycle
-- (draft/sent/cancelled/archived) is shared by every document type and stays
-- in `status`; what is specific to an offer is its commercial outcome, which
-- earns its own column. The rule that an offer can never be "paid" is enforced
-- in Go, where the state machine can see document_type — the CHECK was only
-- ever a backstop, never the state machine.

ALTER TABLE invoices ADD COLUMN converted_from_id TEXT REFERENCES invoices(id);

ALTER TABLE invoices ADD COLUMN quote_outcome TEXT
    CHECK (quote_outcome IS NULL OR quote_outcome IN ('accepted', 'refused', 'expired'));

-- Answers "was this offer already turned into an invoice?" on every conversion
-- attempt, and "which offer produced this invoice?" when auditing a sale.
CREATE INDEX IF NOT EXISTS idx_invoices_converted_from ON invoices(converted_from_id);
