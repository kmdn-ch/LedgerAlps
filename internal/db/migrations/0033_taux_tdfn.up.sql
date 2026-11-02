-- Le taux de la dette fiscale nette (TDFN) ACCORDÉ par l'AFC à l'entreprise (audit croisé Gemini, CRIT-07).
--
-- La méthode des taux de la dette fiscale nette est un TAUX FORFAITAIRE que l'AFC accorde à chaque
-- entreprise, selon sa branche, et qu'elle confirme par écrit. LedgerAlps le calculait : « taux normal × 0,8 »,
-- soit 6,48 % pour tout le monde, sur le chiffre d'affaires hors taxe — un taux qu'aucune branche ne porte,
-- sur une base qui n'est pas celle de la méthode. Le montant partait dans une déclaration à l'administration.
--
-- Le taux est désormais une donnée SAISIE, jamais devinée. NULL = pas de taux : la déclaration TDFN est alors
-- refusée avec une explication. En pour cent (6.1 pour 6,1 %), deux décimales au plus.
ALTER TABLE company_settings ADD COLUMN tdfn_rate REAL;
