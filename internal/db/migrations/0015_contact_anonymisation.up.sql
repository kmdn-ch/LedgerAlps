-- Anonymisation d'un contact — nLPD (RS 235.1) art. 6 al. 4 et art. 32.
--
-- L'art. 6 al. 4 impose de détruire ou d'anonymiser les données personnelles
-- dès qu'elles ne sont plus nécessaires au regard des finalités du traitement.
-- L'art. 32 permet à la personne concernée d'en demander l'effacement.
--
-- Ces deux règles se heurtent au CO art. 958f, qui impose de conserver dix ans
-- les livres et les pièces comptables, et à la LTVA art. 26, qui exige que la
-- facture nomme son destinataire. Effacer un client effacerait une mention
-- obligatoire de ses factures.
--
-- La conciliation tient en une phrase : ce qui doit être conservé, ce n'est pas
-- la *fiche client*, c'est la *pièce*. Depuis la migration 0014, chaque facture
-- porte l'identité de son destinataire telle qu'elle était à l'émission. La
-- fiche peut donc être anonymisée sans toucher à une seule pièce comptable.
--
-- `anonymised_at` est la trace de l'opération : la nLPD demande de pouvoir
-- démontrer sa conformité, et un effacement sans trace ne se démontre pas. La
-- colonne ne contient aucune donnée personnelle — seulement une date.

ALTER TABLE contacts ADD COLUMN anonymised_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_contacts_anonymised ON contacts(anonymised_at);
