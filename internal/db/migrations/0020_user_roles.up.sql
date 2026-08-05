-- Rôles : Administrateur, Comptable, Lecture seule.
--
-- Jusqu'ici un compte était administrateur ou ne l'était pas. Donner un accès à
-- sa fiduciaire revenait donc à partager le compte, avec le droit de modifier
-- les livres et d'effacer les sauvegardes.
--
-- La colonne `is_admin` est conservée et tenue à jour. Deux raisons : une base
-- restaurée dans une version antérieure doit rester utilisable, et une
-- migration qui supprime la seule source d'autorité d'un produit est une
-- migration dont l'échec ne se rattrape pas.
--
-- La traduction de l'existant élargit le moins possible :
--   is_admin = 1  →  admin
--   is_admin = 0  →  accountant
--
-- « accountant » et non « viewer » : ces comptes écrivaient des factures, et
-- les rétrograder du jour au lendemain casserait des installations. L'inverse
-- serait pire — personne ne devient administrateur par une migration.
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'accountant';

UPDATE users SET role = CASE WHEN is_admin = 1 THEN 'admin' ELSE 'accountant' END;

-- L'index sert au contrôle fait à CHAQUE requête : le rôle est lu dans la base
-- et jamais dans le jeton, pour qu'une rétrogradation prenne effet tout de
-- suite au lieu d'attendre l'expiration du jeton.
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role, is_active);
