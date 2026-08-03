-- Fige l'identité du destinataire sur la facture, à son émission.
--
-- Une facture ne conservait que `contact_id`. Le PDF, régénéré à la demande,
-- relisait le contact **vivant** : renommer un client, corriger son adresse ou
-- le voir déménager réécrivait rétroactivement toutes ses factures passées.
-- Réimprimer une facture de 2024 la montrait avec les coordonnées de 2026.
--
-- Ce n'est pas un détail d'affichage. Une facture est une pièce comptable : le
-- CO art. 958f impose de la conserver dix ans **telle qu'elle est**, et la
-- LTVA art. 26 exige qu'elle nomme son destinataire. Une pièce dont le contenu
-- change avec la fiche client n'est plus celle qui a été envoyée, et ne prouve
-- plus rien.
--
-- C'est aussi ce qui rendait l'anonymisation d'un contact impossible : effacer
-- l'identité du client aurait effacé celle de toutes ses factures.
--
-- Les colonnes sont nullables : les factures antérieures sont remplies au
-- démarrage depuis leur contact — la meilleure vérité encore disponible. Ce
-- rattrapage est signalé pour ce qu'il est, une reconstitution, et non une
-- restitution de ce qui avait été imprimé.

ALTER TABLE invoices ADD COLUMN recipient_name        TEXT;
ALTER TABLE invoices ADD COLUMN recipient_address     TEXT;
ALTER TABLE invoices ADD COLUMN recipient_postal_code TEXT;
ALTER TABLE invoices ADD COLUMN recipient_city        TEXT;
ALTER TABLE invoices ADD COLUMN recipient_country     TEXT;
ALTER TABLE invoices ADD COLUMN recipient_vat_number  TEXT;

-- Marque une identité reconstituée au rattrapage plutôt que figée à l'émission.
-- La distinction compte lors d'une révision : l'une prouve, l'autre approxime.
ALTER TABLE invoices ADD COLUMN recipient_backfilled INTEGER NOT NULL DEFAULT 0;
