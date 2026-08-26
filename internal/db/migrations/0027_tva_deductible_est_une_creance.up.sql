-- Le compte 2262 « TVA déductible » est une CRÉANCE, pas une dette.
--
-- Il portait le type `liability` depuis le plan comptable semé en 0002. Or ce
-- compte enregistre la TVA payée sur les achats, que l'AFC doit REMBOURSER :
-- c'est de l'argent qu'on attend, pas qu'on doit. Son voisin 2261 « TVA due »
-- est bien une dette, et la ressemblance des deux codes explique probablement
-- la confusion d'origine.
--
-- # Ce que cela changeait
--
-- Le bilan comme l'état du patrimoine du carnet du lait calculent le solde
-- d'un passif par `crédit - débit`. Ce compte étant débité à chaque achat, son
-- solde sortait NÉGATIF et s'affichait parmi les engagements :
--
--     Engagements
--       2262  TVA déductible (créance fiscale)      -81.00
--
-- Une créance présentée comme une dette négative. Sur une pièce remise à
-- l'administration fiscale, la ligne est fausse dans sa nature même — et le
-- carnet du lait, introduit en v1.5.4, est le premier document du produit à
-- l'exposer.
--
-- # Ce que cela ne changeait PAS
--
-- Les totaux. Un actif de +81 et un passif de -81 contribuent identiquement à
-- la fortune nette comme à l'équation du bilan (`Actif = Passif + Capitaux`) :
--
--     avant :  A        = (P - 81) + C
--     après :  A + 81   =  P       + C
--
-- Les deux se valent. Aucun solde, aucune écriture et aucune empreinte d'audit
-- n'est touché ici : seule la COLONNE dans laquelle le compte s'affiche change.
-- C'est pour cela que le défaut a pu vivre aussi longtemps sans que rien ne le
-- signale.
--
-- # Pourquoi une migration plutôt qu'une correction du fichier 0002
--
-- 0002 ne s'exécute que sur une base neuve. Les installations existantes ont
-- déjà leur ligne 2262 en `liability` ; corriger le seed ne les rattraperait
-- pas, et les deux populations divergeraient en silence.
--
-- La condition sur `account_type` rend la migration idempotente et laisse
-- intact un plan comptable que l'utilisateur aurait retypé lui-même.
UPDATE accounts
   SET account_type = 'asset',
       updated_at   = CURRENT_TIMESTAMP
 WHERE code = '2262'
   AND account_type = 'liability';
