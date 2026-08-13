-- Assujetti ou non à la TVA : une DÉCISION, pas un champ vide.
--
-- Jusqu'ici, la seule trace de ce statut était la présence d'un numéro de TVA.
-- Un champ vide voulait alors dire deux choses opposées — « je ne suis pas
-- assujetti » et « je le suis mais je ne l'ai pas encore saisi » — que rien ne
-- permettait de distinguer. LedgerAlps appliquait donc 8.1 % par défaut à
-- chaque ligne, puis refusait la facture au moment de l'établir. Le débutant
-- non assujetti se heurtait à un mur qu'il ne pouvait comprendre qu'en lisant
-- la LTVA.
--
-- Trois valeurs :
--
--   ''        non déclaré — l'état de départ, et la question à poser
--   'liable'  assujetti (registre AFC des assujettis)
--   'exempt'  non assujetti
--
-- Les installations EXISTANTES qui portent un numéro de TVA sont marquées
-- assujetties : ce numéro ne s'obtient qu'en s'inscrivant au registre, la
-- déduction est sûre, et poser la question à quelqu'un qui y a déjà répondu par
-- son numéro serait du bruit.
--
-- Celles qui n'en portent pas restent NON DÉCLARÉES. Deviner « non assujetti »
-- ferait passer leurs lignes à 0 % du jour au lendemain — une correction
-- silencieuse de leur facturation, sur une hypothèse. La question leur sera
-- posée ; en attendant, rien ne change pour elles.
ALTER TABLE company_settings ADD COLUMN vat_status TEXT NOT NULL DEFAULT '';

UPDATE company_settings
   SET vat_status = 'liable'
 WHERE TRIM(COALESCE(vat_number, '')) <> '';
