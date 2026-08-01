-- Credit notes: link to the invoice they correct.
--
-- A credit note reduces the VAT debt (LTVA art. 41). Since v1.4.3 the figure is
-- right, but the document referenced nothing: a controller seeing reduced VAT
-- had no way back to the invoice being corrected, which is what CO art. 957a
-- al. 2 ch. 5 asks of traceability. LTVA art. 27 al. 4 is more explicit still —
-- a correction is "un document qui mentionne et annule la facture d'origine",
-- and mentioning implies pointing at something.
--
-- The link also makes the amount checkable. Without it nothing stopped
-- crediting more than the original invoice; with it, the sum of the credit
-- notes attached to an invoice can be compared against its total.
--
-- Deliberately separate from converted_from_id (0010): that one records "this
-- invoice came from that offer", this one records "this credit note cancels
-- that invoice". Same shape, opposite direction, different meaning — merging
-- them would make both ambiguous.

ALTER TABLE invoices ADD COLUMN corrects_invoice_id TEXT REFERENCES invoices(id);

-- Answers "how much of this invoice has already been credited?" on every
-- credit-note creation, and "was this invoice ever corrected?" when auditing.
CREATE INDEX IF NOT EXISTS idx_invoices_corrects ON invoices(corrects_invoice_id);
