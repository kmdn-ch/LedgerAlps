-- Un relevé camt.053 marque un STORNO par `RvslInd` : l'annulation d'une opération antérieure
-- (versement rejeté, prélèvement contesté). L'import l'ignorait, si bien que le storno d'un
-- encaissement se lisait comme un encaissement de plus et proposait de solder une facture
-- (audit croisé Gemini, CRIT-09).
--
-- La colonne le garde ; les écritures déjà importées restent à 0 — on ne peut pas savoir
-- après coup lesquelles étaient des stornos.
ALTER TABLE bank_entries ADD COLUMN is_reversal INTEGER NOT NULL DEFAULT 0;
