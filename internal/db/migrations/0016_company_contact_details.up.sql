-- Téléphone et courriel de l'émetteur, pour la facture.
--
-- `company_settings` portait l'adresse postale mais aucun moyen de joindre
-- l'entreprise. Une facture doit permettre au destinataire de poser une
-- question — sur une ligne, sur un délai, sur un montant — sans avoir à
-- chercher ailleurs. Une facture qu'on ne peut pas contester facilement se
-- paie tard, ou pas.
--
-- Ce n'est pas une mention imposée par la loi : la LTVA art. 26 al. 2 exige le
-- nom et le lieu du fournisseur, pas ses coordonnées de contact. C'est un usage
-- commercial, et il coûte deux colonnes.

ALTER TABLE company_settings ADD COLUMN phone TEXT NOT NULL DEFAULT '';
ALTER TABLE company_settings ADD COLUMN email TEXT NOT NULL DEFAULT '';
