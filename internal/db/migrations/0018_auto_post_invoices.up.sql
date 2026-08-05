-- Comptabilisation automatique des factures à l'émission.
--
-- Jusqu'ici, aucune facture ne passait d'écriture : seuls les paiements étaient
-- automatisés. Les utilisateurs qui tenaient une comptabilité complète les
-- saisissaient donc à la main.
--
-- Activer la comptabilisation automatique sur une installation existante
-- doublerait ces écritures — le produit serait compté deux fois, la TVA due
-- aussi. C'est pourquoi le réglage arrive à 1 pour les nouvelles installations
-- (valeur par défaut de la colonne, appliquée à l'insertion faite au premier
-- lancement) et est explicitement remis à 0 pour les fiches existantes.
--
-- Autrement dit : rien ne change pour qui utilise déjà LedgerAlps, tant qu'il
-- n'a pas activé le réglage lui-même en connaissance de cause.
ALTER TABLE company_settings ADD COLUMN auto_post_invoices INTEGER NOT NULL DEFAULT 1;

UPDATE company_settings SET auto_post_invoices = 0;
