-- `amount_paid` des factures déjà marquées « payées ».
--
-- Jusqu'à la 1.6.4, « Marquer payée » ne changeait que le statut : la colonne
-- `amount_paid` restait à zéro sur une facture réglée en totalité. Les écrans qui
-- lisent cette colonne (encaissé du chiffre d'affaires, tableau de bord) comptaient
-- donc zéro encaissé sur toutes les factures payées avant la mise à jour.
--
-- Depuis la 1.6.5 tout encaissement renseigne `amount_paid` (voir
-- invoicing.RecordPayment). Cette migration aligne les factures PAYÉES d'avant :
-- une facture au statut `paid` est réglée en totalité, son encaissé est son total.
--
-- Ce qu'elle ne touche pas, volontairement :
--
--   * les factures « archivées » — le statut ne dit pas si elles ont été payées ou
--     annulées avant d'être rangées ;
--   * les offres de prix, qui n'ont jamais de paiement ;
--   * toute facture dont `amount_paid` est déjà renseigné.
--
-- Elle ne crée AUCUNE écriture au journal : le journal est immuable, et la date et
-- le compte de ces encaissements anciens ne sont connus de personne d'autre que de
-- celui qui les a saisis. Ces factures ont pu être passées à la main.
--
-- Rejouable sans effet : la clause `amount_paid = 0` empêche tout second passage.
UPDATE invoices
SET amount_paid = total_amount
WHERE status = 'paid'
  AND amount_paid = 0
  AND total_amount > 0
  AND COALESCE(document_type, 'invoice') <> 'quote';
