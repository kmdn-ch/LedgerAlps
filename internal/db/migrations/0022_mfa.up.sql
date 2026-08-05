-- Second facteur (TOTP) et codes de secours.
--
-- Le secret est stocké en clair dans la base. Ce n'est pas un oubli : le
-- serveur doit le lire à chaque vérification, sans intervention humaine, donc
-- toute clé qui le protégerait vivrait sur la même machine. Ce qui protège
-- réellement ce secret est le chiffrement de la base et celui du disque —
-- exactement comme pour le reste de la comptabilité.
--
-- Ce que le second facteur protège, c'est le cas où le MOT DE PASSE fuit :
-- réutilisé ailleurs, deviné, lu par-dessus l'épaule. Il ne protège pas d'un
-- attaquant qui lit déjà le fichier de base ; celui-là n'a besoin d'aucun code.
CREATE TABLE IF NOT EXISTS user_mfa (
    user_id     TEXT PRIMARY KEY REFERENCES users(id),
    secret      TEXT NOT NULL,
    -- NULL tant que l'inscription n'a pas été confirmée par un premier code.
    -- Un secret créé mais jamais confirmé ne doit pas bloquer la connexion :
    -- quelqu'un qui abandonne l'assistant en cours de route serait enfermé.
    confirmed_at TIMESTAMP,
    -- Dernière fenêtre de trente secondes acceptée. Un code ne sert qu'une
    -- fois : sans ce compteur, un code lu par-dessus l'épaule reste utilisable
    -- pendant toute sa minute de validité.
    last_window INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Codes de secours.
--
-- Sans eux, un téléphone perdu enferme définitivement le dernier
-- administrateur : plus personne ne peut créer de compte, restaurer une
-- sauvegarde, ni rendre le droit de le faire. Le second facteur créerait alors
-- la panne qu'il est censé prévenir.
--
-- Ils sont hachés comme des mots de passe : la base les contient, et quelqu'un
-- qui la lit ne doit pas y trouver de quoi contourner le second facteur.
CREATE TABLE IF NOT EXISTS mfa_recovery_codes (
    id        TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    user_id   TEXT NOT NULL REFERENCES users(id),
    code_hash TEXT NOT NULL,
    -- Un code de secours ne sert qu'une fois. Marqué plutôt que supprimé : il
    -- reste utile de voir combien ont été consommés, et quand.
    used_at    TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_mfa_recovery_user ON mfa_recovery_codes(user_id, used_at);
