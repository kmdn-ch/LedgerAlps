-- Autorise une seconde réparation, à sens unique, sur une écriture
-- comptabilisée : marquer comme extourne une écriture qui EN EST une, quand
-- elle ne l'était pas encore.
--
-- Contexte. Le troisième audit a établi que le chemin d'annulation des
-- factures FOURNISSEUR ne renseignait jamais `is_reversal` ni
-- `reversal_of_id` — seul celui des factures clientes le faisait
-- (internal/services/invoicing/service.go). Une extourne fournisseur
-- s'écrivait donc dans les livres en se DÉCRIVANT elle-même comme une
-- extourne dans son libellé, tout en portant `is_reversal = 0` et aucun
-- lien vers l'écriture qu'elle annule — jusque dans l'archive légale
-- (CO art. 958f) remise à la fiduciaire.
--
-- Le code applicatif est corrigé (voir supplier_cancel.go). Cette migration
-- ouvre la porte, étroite, pour que les écritures DÉJÀ comptabilisées avec
-- le défaut puissent être corrigées à leur tour — le déclencheur
-- d'immuabilité interdisant sinon tout UPDATE sur une ligne « posted ».
--
-- Pourquoi cette exception ne dégrade pas la garantie d'immuabilité, sur le
-- même modèle que celle posée par la migration 0013 pour `fiscal_year_id` :
--
--   1. Elle est à SENS UNIQUE — uniquement de « non marquée » vers
--      « marquée ». Une extourne déjà marquée ne peut pas être démarquée,
--      ni son lien déplacé vers une autre écriture : ce serait la
--      manipulation à craindre, et elle reste refusée.
--   2. Toutes les colonnes qui PORTENT un fait comptable doivent rester
--      identiques : référence, date, description, statut, empreinte
--      d'intégrité, exercice, auteur. Un seul écart et l'UPDATE est
--      toujours refusé.
--   3. `is_reversal` et `reversal_of_id` n'entrent dans AUCUNE empreinte.
--      La chaîne d'audit (CO art. 957a) porte sur user_id, action,
--      table_name, record_id, before_state, after_state, ip_address et
--      created_at ; `integrity_hash` est le maillon chaîné correspondant.
--      Les marquer ne change donc aucune empreinte : la vérification de
--      chaîne donne le même résultat avant et après. La réparation est
--      prouvablement neutre — comme la précédente.
--
-- Autrement dit : cela rétablit une information qui aurait dû être écrite à
-- la création de l'extourne, sans toucher à un seul fait comptable.

DROP TRIGGER IF EXISTS trg_journal_entries_no_update;

CREATE TRIGGER trg_journal_entries_no_update
BEFORE UPDATE ON journal_entries
FOR EACH ROW
WHEN OLD.status = 'posted' AND NOT (
    (
        -- Exception posée par la migration 0013 : rattacher l'exercice
        -- manquant, une seule fois, de NULL vers une valeur.
            OLD.fiscal_year_id IS NULL
        AND NEW.fiscal_year_id IS NOT NULL
        AND NEW.reference      IS OLD.reference
        AND NEW.date           IS OLD.date
        AND NEW.description    IS OLD.description
        AND NEW.status         IS OLD.status
        AND NEW.integrity_hash IS OLD.integrity_hash
        AND NEW.is_reversal    IS OLD.is_reversal
        AND NEW.reversal_of_id IS OLD.reversal_of_id
        AND NEW.created_by_id  IS OLD.created_by_id
    )
    OR
    (
        -- Cette exception : marquer une extourne, une seule fois, de
        -- « non marquée » vers « marquée et rattachée ».
            OLD.is_reversal     = 0
        AND NEW.is_reversal     = 1
        AND OLD.reversal_of_id IS NULL
        AND NEW.reversal_of_id IS NOT NULL
        AND NEW.reference       IS OLD.reference
        AND NEW.date            IS OLD.date
        AND NEW.description     IS OLD.description
        AND NEW.status          IS OLD.status
        AND NEW.integrity_hash  IS OLD.integrity_hash
        AND NEW.fiscal_year_id  IS OLD.fiscal_year_id
        AND NEW.created_by_id   IS OLD.created_by_id
    )
)
BEGIN
    SELECT RAISE(ABORT, 'Cannot modify a posted journal entry (CO art. 957a)');
END;
