-- Les LIGNES d'une écriture comptabilisée et le JOURNAL D'AUDIT deviennent immuables.
--
-- Seule la table `journal_entries` avait ses déclencheurs (0001). Les lignes — là où vivent
-- les comptes et les montants — et le journal d'audit lui-même se réécrivaient par un simple
-- UPDATE : sur une écriture comptabilisée, un compte 6000 changé en 6200 et un montant porté
-- à 999 999 étaient acceptés, et la vérification de la chaîne répondait ensuite « intègre »
-- (audit croisé Gemini, CRIT-10 : l'immuabilité du produit était une illusion).
--
-- Ces déclencheurs ne sont pas la protection ultime : qui écrit dans le fichier de la base peut
-- les retirer, comme ceux de `journal_entries`. Cette protection-là est la chaîne d'empreintes,
-- qui scelle maintenant le condensé des lignes (`lines_digest`) et le fait voir à la vérification.
-- Les déclencheurs ferment la porte à tout ce qui passe par la base, sans fichier à ouvrir :
-- une injection SQL, un script de maintenance, un outil de base de données.
--
-- Les lignes d'un BROUILLON restent modifiables : on les édite avant de comptabiliser.

CREATE TRIGGER IF NOT EXISTS trg_journal_lines_no_insert_posted
BEFORE INSERT ON journal_lines
FOR EACH ROW
WHEN EXISTS (SELECT 1 FROM journal_entries e WHERE e.id = NEW.entry_id AND e.status = 'posted')
BEGIN
    SELECT RAISE(ABORT, 'écriture comptabilisée : on n''y ajoute pas de ligne (CO art. 957a, 958f)');
END;

CREATE TRIGGER IF NOT EXISTS trg_journal_lines_no_update_posted
BEFORE UPDATE ON journal_lines
FOR EACH ROW
WHEN EXISTS (SELECT 1 FROM journal_entries e
             WHERE e.status = 'posted' AND (e.id = OLD.entry_id OR e.id = NEW.entry_id))
BEGIN
    SELECT RAISE(ABORT, 'écriture comptabilisée : ses lignes ne se modifient pas (CO art. 957a, 958f)');
END;

CREATE TRIGGER IF NOT EXISTS trg_journal_lines_no_delete_posted
BEFORE DELETE ON journal_lines
FOR EACH ROW
WHEN EXISTS (SELECT 1 FROM journal_entries e WHERE e.id = OLD.entry_id AND e.status = 'posted')
BEGIN
    SELECT RAISE(ABORT, 'écriture comptabilisée : ses lignes ne se suppriment pas (CO art. 957a, 958f)');
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_logs_no_update
BEFORE UPDATE ON audit_logs
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'journal d''audit : un maillon ne se modifie pas (CO art. 957a)');
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_logs_no_delete
BEFORE DELETE ON audit_logs
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'journal d''audit : un maillon ne se supprime pas (CO art. 957a)');
END;
