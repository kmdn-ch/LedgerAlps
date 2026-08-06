-- Ordinateurs de confiance : « se souvenir de cet ordinateur ».
--
-- Redemander un code à chaque connexion sur le poste où l'on travaille tous les
-- jours n'ajoute rien : le second facteur protège d'un mot de passe qui fuit, et
-- quelqu'un qui a déjà la main sur ce poste n'a pas besoin d'attendre la
-- prochaine connexion. En revanche, une protection ressentie comme une
-- brimade finit désactivée — c'est la façon la plus sûre de la perdre
-- entièrement.
--
-- # Trente jours
--
-- C'est la durée retenue par la plupart des services qui offrent cette option.
-- Assez long pour que le poste habituel ne redemande rien pendant un mois de
-- travail ; assez court pour qu'un portable oublié quelque part redevienne
-- protégé avant qu'on ait fini de le chercher. La date est absolue, jamais
-- prolongée à l'usage : sans quoi un poste consulté chaque semaine ne
-- redemanderait plus jamais de code.
--
-- # Ce qui est stocké
--
-- Le HACHÉ du jeton, jamais le jeton. Le navigateur en garde l'unique copie
-- dans un cookie HttpOnly. Quelqu'un qui lit cette table ne peut donc pas s'en
-- servir pour contourner le second facteur — c'est la même règle que pour les
-- codes de secours.
CREATE TABLE IF NOT EXISTS trusted_devices (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    user_id     TEXT NOT NULL REFERENCES users(id),
    token_hash  TEXT NOT NULL,

    -- De quoi reconnaître l'appareil dans la liste, sans en dire plus que
    -- nécessaire (nLPD art. 6 : minimisation). Pas d'empreinte de navigateur,
    -- pas de matériel : un libellé et une date suffisent à décider si l'on
    -- révoque.
    label       TEXT NOT NULL DEFAULT '',
    last_ip     TEXT NOT NULL DEFAULT '',

    expires_at  TIMESTAMP NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_trusted_devices_user ON trusted_devices(user_id, expires_at);
