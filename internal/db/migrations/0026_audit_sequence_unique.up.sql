-- Le numéro de séquence de la chaîne d'audit devient UNIQUE.
--
-- # Ce qu'il tenait, et ce qui le cassait
--
-- `sequence_number` est ce qui rend une SUPPRESSION visible dans la chaîne du
-- CO art. 957a : les empreintes chaînées montrent qu'un maillon a été modifié,
-- la continuité des numéros montre qu'un maillon a été retiré. Sans elle, on
-- ne détecte que la falsification, pas l'effacement.
--
-- L'index était simple, pas unique. Deux écritures concurrentes lisaient donc
-- le même prédécesseur et inséraient deux maillons portant le même numéro —
-- une chaîne fourchue. La vérification lisait alors « lien rompu » puis
-- « -1 entrée supprimée », et posait Verified=false de façon DÉFINITIVE : le
-- produit accusait l'utilisateur d'avoir altéré ses livres alors que personne
-- n'avait rien touché. Une fausse alerte n'est crue qu'une fois.
--
-- La cause est corrigée dans le code (l'écriture se fait maintenant dans une
-- transaction). Cet index est la seconde barrière : il rend la fourche
-- impossible même si un chemin d'écriture oubliait la transaction un jour.
--
-- # Pourquoi renuméroter d'abord
--
-- Une base déjà fourchue refuserait l'index unique, et la migration échouerait
-- au démarrage — laissant l'utilisateur sans ses livres pour un défaut qui ne
-- l'empêche pas de travailler. On rend donc la suite strictement croissante
-- avant de poser la contrainte.
--
-- **Aucune empreinte n'est touchée.** `ComputeEntryHash` porte sur l'auteur,
-- l'action, la table, l'enregistrement, les états avant/après, l'adresse et
-- l'horodatage — `sequence_number` n'y figure pas, et le chaînage relie
-- `prev_hash` à `entry_hash`, jamais aux numéros. Renuméroter ne peut donc ni
-- valider une chaîne réellement rompue, ni rompre une chaîne saine : cela
-- retire seulement le « -1 entrée supprimée » qui était un artefact de la
-- fourche, et laisse le lien rompu visible s'il l'est vraiment.
--
-- L'ordre retenu — numéro, puis date, puis identifiant — est déterministe et
-- préserve l'ordre existant. Sur une base saine, chaque ligne se voit
-- réattribuer le numéro qu'elle portait déjà.
UPDATE audit_logs
   SET sequence_number = (
       SELECT rang FROM (
           SELECT id,
                  ROW_NUMBER() OVER (ORDER BY sequence_number, created_at, id) AS rang
             FROM audit_logs
       ) AS classement
        WHERE classement.id = audit_logs.id
   );

DROP INDEX IF EXISTS idx_audit_logs_sequence;
CREATE UNIQUE INDEX idx_audit_logs_sequence ON audit_logs(sequence_number);
