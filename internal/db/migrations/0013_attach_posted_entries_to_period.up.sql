-- Autorise une seule réparation sur une écriture comptabilisée : renseigner son
-- exercice comptable lorsqu'il manque.
--
-- Contexte. `fiscal_year_id` n'était renseigné nulle part avant la v1.4.6.
-- Toute base existante contient donc des écritures comptabilisées orphelines de
-- tout exercice — invisibles à la clôture, qui filtre dessus, et absentes de
-- l'archive légale filtrée par exercice. Le déclencheur d'immuabilité
-- (CO art. 957a) interdisant tout UPDATE sur une écriture comptabilisée, ces
-- écritures resteraient orphelines pour toujours.
--
-- Pourquoi cette exception ne dégrade pas la garantie d'immuabilité :
--
--   1. Elle est à sens unique — uniquement de NULL vers une valeur. Une
--      écriture déjà rattachée ne peut pas être déplacée d'un exercice à un
--      autre, ce qui serait la manipulation à craindre.
--   2. Toutes les colonnes comptables doivent rester identiques : référence,
--      date, description, statut, empreinte d'intégrité, indicateurs de
--      contrepassation, auteur. Un seul écart et l'UPDATE est refusé.
--   3. `fiscal_year_id` **n'entre dans aucune empreinte**. La chaîne d'audit
--      (CO art. 957a) porte sur user_id, action, table_name, record_id,
--      before_state, after_state, ip_address et created_at ; `integrity_hash`
--      est le maillon chaîné correspondant. Renseigner l'exercice ne change
--      donc aucune empreinte : la vérification de chaîne donne le même
--      résultat avant et après. La réparation est prouvablement neutre.
--
-- Autrement dit : cela rétablit un lien qui aurait dû être écrit à la création,
-- sans toucher à un seul fait comptable.

DROP TRIGGER IF EXISTS trg_journal_entries_no_update;

CREATE TRIGGER trg_journal_entries_no_update
BEFORE UPDATE ON journal_entries
FOR EACH ROW
WHEN OLD.status = 'posted' AND NOT (
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
BEGIN
    SELECT RAISE(ABORT, 'Cannot modify a posted journal entry (CO art. 957a)');
END;
