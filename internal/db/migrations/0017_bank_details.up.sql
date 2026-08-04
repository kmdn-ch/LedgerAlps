-- Coordonnées de la banque, pour un paiement par virement.
--
-- L'IBAN suffit à la QR-facture : le bulletin porte tout ce qu'une banque
-- suisse doit savoir. Il ne suffit pas à un virement passé depuis l'étranger,
-- où l'on demande le nom de la banque et son BIC/SWIFT — et il ne suffit pas
-- non plus à un client qui saisit le virement à la main et veut vérifier qu'il
-- envoie l'argent au bon endroit.
--
-- Ces champs sont facultatifs. Une entreprise qui n'encaisse qu'en Suisse par
-- QR-facture n'en a pas l'usage, et les rendre obligatoires ajouterait trois
-- cases à remplir pour rien.

ALTER TABLE company_settings ADD COLUMN bank_name    TEXT NOT NULL DEFAULT '';
ALTER TABLE company_settings ADD COLUMN bank_address TEXT NOT NULL DEFAULT '';
ALTER TABLE company_settings ADD COLUMN bank_bic     TEXT NOT NULL DEFAULT '';
