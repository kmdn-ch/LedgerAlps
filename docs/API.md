# Référence API

Toutes les routes applicatives sont préfixées par `/api/v1`.

**Authentification** — JWT `Bearer`. `POST /auth/login` renvoie un
`access_token` (60 min) et un `refresh_token` (30 jours). Le refresh porte un
`jti` permettant de le révoquer individuellement.

| Accès | Signification |
|---|---|
| public | aucun jeton requis |
| auth | jeton d'accès valide |
| admin | jeton d'accès d'un utilisateur administrateur |

> Les routes d'authentification sont protégées par une limitation de débit :
> **10 échecs** par IP en 15 minutes déclenchent un verrouillage
> (`429` + en-tête `Retry-After`), dont la durée **s'allonge à chaque série** :
> 30 s, 1 min, 5 min, 15 min, 1 h — le dernier palier se répétant ensuite.
> Une heure de silence après la fin d'un verrou ramène l'échelle à son premier
> palier ; une connexion réussie l'efface entièrement.
>
> L'adresse retenue est celle de la connexion elle-même. Les en-têtes
> `X-Forwarded-For` / `X-Real-IP` ne sont pris en compte que si le mandataire
> qui les pose est déclaré dans `TRUSTED_PROXIES` — sans quoi n'importe qui
> choisirait l'identité que le compteur observe.

---

## Authentification

| Méthode | Route | Accès | Description |
|---|---|---|---|
| POST | `/auth/bootstrap` | public | Créer le premier administrateur (une seule fois) |
| POST | `/auth/login` | public | Connexion → jetons |
| POST | `/auth/refresh` | public | Renouveler le jeton d'accès |
| POST | `/auth/logout` | public | Révoquer le jeton de rafraîchissement |
| POST | `/auth/change-password` | auth | Choisir son mot de passe. Seule route ouverte à un compte au mot de passe temporaire |
| POST | `/auth/mfa/verify` | jeton d'attente | Deuxième étape de la connexion : code TOTP ou code de secours |
| GET | `/auth/mfa` | auth | État du second facteur du compte connecté |
| POST | `/auth/mfa/setup` | auth | Préparer une inscription — rend le secret, l'URI `otpauth://` et le QR |
| POST | `/auth/mfa/confirm` | auth | Confirmer par un premier code — rend les codes de secours, une seule fois |
| DELETE | `/auth/mfa` | auth | Retirer le second facteur. Mot de passe redemandé |
| GET | `/auth/devices` | auth | Ordinateurs de confiance de ce compte, et la durée accordée |
| DELETE | `/auth/devices` | auth | Les oublier tous — un code sera redemandé partout |

### Ordinateurs de confiance

`POST /auth/mfa/verify` accepte `remember_device: true`. Le serveur pose alors un
cookie HttpOnly `SameSite=Strict` valable **30 jours**, dont seul le haché est
conservé en base. La date d'expiration est **absolue** : se connecter ne la
prolonge pas.

La confiance est liée au **compte**, pas au navigateur : un poste de confiance
pour l'un ne l'est pas pour l'autre. Elle tombe quand le mot de passe change, et
quand le second facteur est retiré ou réinscrit.

### Connexion en deux temps

`POST /auth/login` rend l'une de deux réponses.

Sans second facteur, la réponse habituelle : `access_token`, `role`,
`must_change_password`, `mfa_enrolment_required`, plus le cookie de
rafraîchissement.

Avec un second facteur **confirmé**, aucune session n'est créée :

```json
{ "mfa_required": true, "mfa_token": "…", "expires_in": 300 }
```

`mfa_token` vit cinq minutes et ne vaut **que** pour `POST /auth/mfa/verify`.
Le filtre d'authentification le refuse sur toute autre route — y compris celles
qui n'existent pas encore : le refus est écrit au point de passage obligé,
plutôt que route par route où il finirait par être oublié une fois.

`POST /auth/mfa/verify` accepte le code à six chiffres **ou** un code de secours,
et rend alors la réponse de session habituelle. Un code TOTP ne sert qu'une
fois : la fenêtre acceptée est enregistrée et refusée ensuite. La route est
derrière la limitation de tentatives — cinq échecs, quinze minutes de fermeture.

### Ce que le second facteur protège

Le cas où le **mot de passe** fuit. Il ne protège pas de quelqu'un qui lit déjà
le fichier de base : le secret y est stocké en clair, parce que le serveur doit
le lire à chaque vérification sans intervention humaine — toute clé qui le
protégerait vivrait sur la même machine. C'est le chiffrement de la base et du
disque qui répond à cette menace.

### Qui doit un second facteur

L'**administrateur** et le **comptable** : les deux peuvent modifier quelque
chose, et un mot de passe volé sur l'un ou l'autre permet de fabriquer une
comptabilité. La **lecture seule** en est dispensée — elle ne peut rien modifier.

### Refus propres au second facteur

| Statut | Cause |
|---|---|
| 401 + `mfa_required` | Un jeton d'attente a été présenté à une route ordinaire |
| 401 | Code faux, ou jeton d'attente expiré (cinq minutes) |
| 403 + `mfa_enrolment_required` | Compte administrateur sans second facteur inscrit |
| 409 | Inscription déjà active — retirez-la d'abord pour changer de téléphone |
| 429 | Trop de codes faux depuis cette adresse |

## Comptes et rôles

Administrateur uniquement. Le rôle est relu dans la base à chaque requête : un
changement s'applique immédiatement, sans attendre l'expiration d'une session.

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/users` | admin | Liste des comptes |
| POST | `/users` | admin | Créer un compte. Le mot de passe est **temporaire** : `must_change_password` est posé |
| PUT | `/users/:id/role` | admin | Changer le rôle (`admin`, `accountant`, `viewer`) |
| PUT | `/users/:id/active` | admin | Activer ou désactiver. Un compte ne se supprime pas (CO art. 957a al. 2 ch. 5) |
| POST | `/users/:id/reset-password` | admin | Remplacer le mot de passe par un temporaire, rendu **une seule fois** |
| DELETE | `/users/:id/mfa` | admin | Retirer le second facteur d'un compte (application 2FA/OTP perdue) |

Les deux dernières routes sont **délibérément séparées**. Réunies en un geste,
elles permettraient à un administrateur de se substituer entièrement à n'importe
quel compte, et le second facteur ne protégerait plus de rien face à lui. Elles
sont tracées séparément dans les événements de sécurité.

Refus : on ne retire pas le dernier administrateur (rétrogradation ou
désactivation), on ne change pas son propre rôle, on ne se réinitialise pas
soi-même, et on ne réinitialise pas un compte désactivé — cela donnerait un mot
de passe utilisable à quelqu'un qui n'a plus le droit d'entrer.

## Système

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/health` | public | État du serveur et version (hors `/api/v1`) |
| GET | `/api/v1/uid-lookup` | public | Recherche d'entreprise au registre IDE/ZEFIX |

## Comptabilité

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/accounts` | lecture | Plan comptable. Champs : `id`, `code`, `name`, `account_type`, `description`, `is_active` |
| POST | `/accounts` | écriture comptable | Créer un compte |
| GET | `/accounts/trial-balance` | lecture | Balance de vérification. Champs : `code`, `name`, `total_debit`, `total_credit`, `balance`. Optionnel : `as_of=AAAA-MM-JJ`. **Écritures comptabilisées uniquement** |
| GET | `/accounts/:code/balance` | lecture | Solde d'un compte |
| GET | `/journal` | lecture | Liste paginée. Filtres : `date_from`, `date_to`, `status`, `reference`, `page`, `page_size`. Chaque ligne porte son `total` (somme des débits) et l'`author` de l'écriture |
| GET | `/journal/:id` | lecture | Détail : les lignes avec `account_code` et `account_name` en clair, plus `integrity_hash` — vide tant que l'écriture est un brouillon |
| POST | `/journal` | écriture comptable | Créer une écriture (brouillon) |
| POST | `/journal/:id/post` | écriture comptable | Comptabiliser — scellée par hachage (CO art. 957a) |

### Créer une écriture

Le compte se désigne par son **numéro** (`account_code`) ou par son identifiant
(`account_id`). Le numéro est ce qu'un comptable a sous les yeux ; exiger
l'identifiant obligeait chaque client à charger le plan comptable et à faire la
traduction — ce qui a produit un formulaire qui répondait 422 à toute saisie.

```json
{
  "date": "2026-08-05",
  "description": "Vente comptant",
  "lines": [
    { "account_code": "1000", "debit_amount": 1076.50 },
    { "account_code": "3200", "credit_amount": 1000.00 },
    { "account_code": "2200", "credit_amount": 76.50, "description": "TVA 8.1%" }
  ]
}
```

Refus, tous en 422, et tous nommant la ligne en cause :

| Cause | Message |
|---|---|
| Moins de deux lignes | « une écriture comporte au moins deux lignes : ce qui est débité et ce qui est crédité » |
| Numéro inconnu | « ligne 1 : le compte 10 n'existe pas dans le plan comptable » |
| Compte désactivé | « ligne 2 : le compte 1109 est désactivé » |
| Ni débit ni crédit | « ligne 3 : ni débit ni crédit » |
| Les deux à la fois | « ligne 1 : un compte est débité ou crédité, pas les deux » |
| Montant négatif | « ligne 2 : un montant ne peut pas être négatif — inscrivez-le de l'autre côté » |
| Déséquilibre | « l'écriture n'est pas équilibrée : débit 100.00, crédit 10.00, **écart 90.00** » |

L'écart est donné parce qu'il désigne presque toujours la faute de frappe :
90.00 sur 100.00 est un zéro oublié, 9.00 une décimale décalée.

### Brouillon et écriture comptabilisée

Un brouillon **ne compte nulle part** : ni à la balance, ni au bilan, ni au
compte de résultat, et il ne porte aucune empreinte. `POST /journal/:id/post` le
scelle dans la chaîne du CO art. 957a et le fait entrer dans les états. C'est
irréversible : une correction se fait par contrepassation.

Le journal n'est **pas** filtré sur l'auteur. Il doit être complet et se
rapprocher de la balance (CO art. 957a al. 2 ch. 2 et 3) ; ce qui borne l'accès
est le rôle, pas un filtre par créateur.

## Facturation

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/invoices` | auth | Liste paginée. Filtres : `status` (dont `overdue`, déduit), `contact_id`, `document_type`, `from`/`to` sur la date d'émission, `page`, `page_size` |
| GET | `/invoices/:id` | auth | Détail |
| POST | `/invoices` | auth | Créer |
| PATCH | `/invoices/:id` | auth | Modifier |
| POST | `/invoices/:id/transition` | auth | Changer de statut |
| POST | `/invoices/:id/convert` | auth | Convertir une offre en facture |
| POST | `/invoices/:id/outcome` | auth | Enregistrer l'issue d'une offre (`refused`, `expired`) |
| POST | `/invoices/:id/credit-note` | auth | Émettre une note de crédit contre une facture |
| GET | `/invoices/:id/pdf` | auth | PDF — bulletin QR-facture sur les factures uniquement |
| POST | `/invoices/bulk-pdf` | auth | Téléchargement groupé. Corps `{"ids": [...]}`. **Un** identifiant renvoie un PDF, **plusieurs** un ZIP — emballer un unique PDF obligerait à le dézipper pour le lire. Doublons ignorés, 200 documents au maximum. Un document disparu entre l'affichage et le clic est omis et compté dans l'en-tête `X-LedgerAlps-Missing` plutôt que passé sous silence |

### Types de documents

`document_type` vaut `invoice`, `quote` (offre de prix) ou `credit_note`. Chacun
tire son numéro d'une séquence annuelle distincte : `FA-2026-0001`,
`OF-2026-0001`, `NC-2026-0001`.

Le type n'est pas cosmétique — il détermine trois comportements :

- **Seule une facture porte un bulletin QR** et s'intitule « FACTURE ». Une
  offre munie d'un bulletin de paiement et d'un montant de TVA est, pour son
  destinataire comme pour l'AFC, une facture : il peut la payer et en déduire
  l'impôt préalable, ce qui rend l'émetteur redevable de cet impôt
  (LTVA art. 27 al. 2).
- **Seules les factures et notes de crédit entrent dans la déclaration TVA.**
  La dette d'impôt naît « au moment de la facturation » (LTVA art. 40 al. 1
  let. a) ; une offre n'en fait naître aucune. Les notes de crédit y entrent
  avec le signe opposé (LTVA art. 41).
- **Une offre ne peut pas passer au statut `paid`.** Personne ne doit rien
  dessus. Sa machine à états est distincte : `draft → sent → cancelled |
  archived`.

### Convertir une offre en facture

`POST /invoices/:id/convert` — corps optionnel :

```json
{ "issue_date": "2026-08-01", "due_date": "2026-08-31" }
```

Sans corps, la facture est émise du jour, échéance à 30 jours.

**L'offre n'est pas transformée : elle est conservée.** Le client en détient une
copie ; remplacer l'enregistrement le laisserait citer une référence qui
n'existe plus chez vous — précisément le lien que le CO art. 958f al. 3 demande
de garantir. La facture créée porte son propre numéro `FA-`, reprend les lignes
de l'offre à l'identique, et pointe vers elle par `converted_from_id`. L'offre
reçoit `quote_outcome = "accepted"`.

| Code | Cas |
|---|---|
| `201` | Facture créée (statut `draft`) |
| `404` | Document introuvable |
| `409` | Cette offre a déjà donné lieu à une facture |
| `422` | Ce n'est pas une offre, ou elle n'est pas au statut `sent` |

Le `409` est la garde contre une double facturation de la même prestation.

`quote_outcome` vaut `accepted`, `refused` ou `expired`, et n'est jamais
renseigné sur une facture. `accepted` n'est pas acceptée par
`/outcome` : une offre est acceptée en produisant la facture, jamais en
basculant un champ — sans quoi une offre pourrait se lire « acceptée » sans
aucune facture derrière.

### Émettre une note de crédit

`POST /invoices/:id/credit-note` — corps optionnel :

```json
{ "issue_date": "2026-08-02", "reason": "Retour marchandise", "lines": [] }
```

`lines` vide crédite la facture **en totalité**, en reprenant ses lignes.
Fournir des lignes crédite une partie (retour d'un article, poste contesté).

La note de crédit référence la facture par `corrects_invoice_id`, et son PDF
porte la mention « Annule la facture : FA-… ». LTVA art. 27 al. 4 définit
en effet la correction comme « un document qui mentionne et annule la facture
d'origine » — la mention doit donc figurer sur le document, pas seulement en
base. La facture d'origine n'est pas modifiée : une correction ajoute un
document, elle ne réécrit pas celui qu'elle corrige.

**Le montant est borné.** La somme des notes de crédit rattachées à une facture
ne peut pas dépasser son total ; les notes annulées ne comptent pas, puisqu'elles
ne créditent rien. Une tolérance d'un centime absorbe l'arrondi à 5 centimes
lorsqu'on crédite ligne à ligne.

| Code | Cas |
|---|---|
| `201` | Note de crédit créée (statut `draft`) |
| `404` | Facture introuvable |
| `409` | Le total crédité dépasserait la facture |
| `422` | La cible n'est pas une facture, ou elle est en brouillon / annulée |

Seule une facture **envoyée ou payée** peut être créditée : un brouillon n'a
jamais été émis, une facture annulée est déjà sans effet — dans les deux cas il
n'y a aucune dette d'impôt à corriger.

> **Écriture au journal** — une note de crédit ne passe pas d'écriture
> automatique, parce qu'aucune facture n'en passe : les ventes se saisissent au
> journal manuellement (seuls les paiements sont automatisés). Contrepasser
> automatiquement un produit jamais enregistré créerait un produit négatif sans
> contrepartie, et risquerait le double comptage si l'utilisateur a déjà passé
> la correction lui-même. Voir la [roadmap](../ROADMAP.md).

## Factures fournisseurs

Source de l'impôt préalable déductible. Seules les factures `booked` ou `paid`
comptent dans la déclaration TVA.

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/supplier-invoices` | lecture | Liste paginée |
| GET | `/supplier-invoices/:id` | lecture | Détail avec lignes |
| POST | `/supplier-invoices` | écriture documents | Créer (refus si doublon fournisseur + référence) |
| PUT | `/supplier-invoices/:id` | écriture documents | Modifier — **brouillons uniquement** (409 sinon) |
| POST | `/supplier-invoices/read-qr` | écriture documents | Lire le QR d'un PDF ou d'une image déposée. **Ne crée rien** |
| POST | `/supplier-invoices/:id/transition` | écriture comptable | Changer de statut |

### Lire le QR d'une facture

Multipart, champ `file`, 10 Mo au maximum. La réponse ne crée rien :

```json
{
  "found": true,
  "bill": { "creditor_name": "…", "creditor_iban": "CH44…", "amount": 1621.50,
            "currency": "CHF", "reference_type": "QRR", "reference": "21000…",
            "message": "Facture FA-118", "is_qr_iban": true },
  "supplier": { "id": "…", "name": "…" }
}
```

La réponse porte aussi `hints`, lu dans la **couche texte** du PDF — ce que le QR
ne contient pas :

```json
{ "invoice_number": "538690", "invoice_number_label": "numéro de facture",
  "issue_date": "2025-12-01", "issue_date_label": "date",
  "due_date": "2025-12-31", "due_date_label": "échu",
  "vat_rate": 0, "vat_mentioned": false, "vat_label": "",
  "supplier_uid": "CHE-103.727.240" }
```

Chaque valeur est accompagnée de **l'étiquette qui l'a produite** : c'est ce qui
permet de repérer une lecture de travers sans rouvrir le document.

`vat_mentioned: false` signifie qu'aucun **taux** n'a été trouvé — le taux vaut
alors 0 % et le montant du QR est aussi le montant hors taxe. Le mot « TVA » ou
« MWST » ne suffit pas : il figure dans le numéro d'assujetti du fournisseur.

Un PDF sans couche texte — un scan — rend des `hints` vides. C'est une absence,
pas une erreur : le QR reste exploitable.

Le fournisseur est reconnu **par son IBAN** — un nom se saisit de dix façons, un
compte non. `id` vide signifie qu'il reste à créer.

Un document sans QR répond **200** avec `found: false` et un `reason` : beaucoup
de factures n'en portent pas, et la saisie manuelle reste le chemin normal. Un
bulletin dont la référence contredit l'IBAN (IG v2.4 §4.2.2) répond **422** en
nommant l'incohérence.
| DELETE | `/supplier-invoices/:id` | écriture comptable | Supprimer — brouillons uniquement (CO art. 958f) |

### Comptabiliser écrit au journal

Le passage à `booked` écrit et **scelle** l'écriture :

```
Débit  <expense_account_code>   montant hors taxe   (6500 par défaut)
Débit  2262 TVA déductible      montant de TVA      (omis s'il n'y en a pas)
Crédit 2000 Créanciers          montant TTC
```

La réponse porte le `journal_entry_id`. L'opération est **idempotente par le
lien** : une facture déjà rattachée à une écriture n'en produit pas une seconde,
si bien qu'un aller-retour de statut ne double ni la charge ni la TVA déductible.

Un échec **bloque** la transition. Contrairement à l'émission d'une facture
client — où le document est déjà parti et où nier l'envoi serait pire — rien
n'est engagé vis-à-vis d'un tiers, et laisser passer le statut sans l'écriture
recréerait le défaut que cette route corrige.

`payment_reference` est la référence du **bulletin de versement**, à ne pas
confondre avec `supplier_reference` qui est le numéro de la facture chez le
fournisseur. C'est elle qui voyage dans l'ordre de virement.

## Sauvegardes

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/backups` | **admin** | Liste des instantanés + restauration en attente |
| POST | `/backups` | **admin** | Créer un instantané (`passphrase` facultative) |
| POST | `/backups/restore` | **admin** | **Préparer** une restauration (`confirm` requis) |
| DELETE | `/backups/restore` | **admin** | Annuler une restauration préparée |
| POST | `/system/restart` | **admin** | Redémarrer pour appliquer la restauration préparée |

Réservé aux administrateurs : une restauration remplace toute la comptabilité.

**Créer** est immédiat et sûr serveur en marche — SQLite écrit une copie
cohérente d'une base en service. Une `passphrase` non vide chiffre l'instantané ;
la copie en clair n'est effacée qu'après relecture, déchiffrement et contrôle
d'intégrité.

**Restaurer ne l'est pas**, et l'API ne fait donc pas semblant. `POST
/backups/restore` répond **`202 Accepted`** : l'instantané est déchiffré et
vérifié sur-le-champ — pendant que l'utilisateur peut corriger une phrase de
passe erronée — puis mis de côté. Le remplacement a lieu **au démarrage
suivant**, avant l'ouverture de la base, parce qu'un serveur ne peut pas
échanger sous lui le fichier qu'il a ouvert.

La comptabilité remplacée est sauvegardée juste avant : une restauration lancée
par erreur reste réversible. Si elle échoue au redémarrage, le serveur démarre
quand même sur la base existante et le journalise — laisser l'utilisateur sans
application serait pire que de ne pas restaurer.

### Redémarrer

`POST /system/restart` — **refusé (`422`) s'il n'y a aucune restauration en
attente**. En dehors de ce cas, un bouton de redémarrage n'est qu'un moyen
supplémentaire d'interrompre quelqu'un en pleine saisie.

Le serveur répond `202` **avant** de s'arrêter, puis : arrêt de l'écoute HTTP,
fermeture de la base, lancement d'une copie neuve de son propre binaire, sortie.
L'ordre compte — le nouveau processus applique la restauration avant d'ouvrir
la base, ce qu'il ne peut pas faire tant que l'ancien détient le fichier.

Le lanceur Windows démarre le serveur sans le superviser : personne d'autre ne
le relancerait, d'où ce redémarrage auto-porté. L'interface interroge `/health`
jusqu'à réponse avant de recharger la page.

## Maintenance & Système

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/onboarding` | auth | Mise en route : les cinq étapes sans lesquelles une facture suisse ne tient pas, et ce qui bloque chacune. Ne rend que des **états** et des noms de champs — les phrases sont au catalogue du frontend |
| POST | `/settings/logo` | **admin** | Logo de la société, en adresse de données PNG ou JPEG (2 Mo au plus). Toute image dont un côté dépasse **300 px** est réduite, sans déformation, et ré-encodée en PNG. La réponse rend `logo_data`, `width`, `height` et `resized` — ce qui a été RETENU, pas ce qui a été envoyé |
| PUT | `/settings/company` | **admin** | `vat_status` vaut `""` (non déclaré), `"liable"` ou `"exempt"`. **Absent = ne touche pas** ; toute autre valeur répond `422`. `"exempt"` efface `vat_number` — il s'imprime sur la facture, et le garder contredirait la déclaration (LTVA art. 27 al. 1) |
| GET | `/maintenance/integrity` | **admin** | Contrôle de cohérence des données |
| GET | `/maintenance/health` | **admin** | État du système, sauvegardes, exposition réseau |
| GET | `/settings/server` | **admin** | Réglages réseau en vigueur |
| PUT | `/settings/server` | **admin** | Écrire les réglages réseau (effet au redémarrage) |

**Lecture seule.** Ces vues montrent, elles ne réparent pas : une comptabilité
incohérente se corrige par une écriture, pas par un bouton — réparer en silence
effacerait la trace de ce qui s'est passé, ce que le CO art. 957a al. 2 ch. 5
interdit précisément.

`integrity` renvoie une liste de constats, chacun avec une gravité (`error`,
`warning`, `info`), un compte, et surtout **une action** : un constat sur lequel
on ne peut rien faire est du bruit qui apprend à ignorer la page. Rien n'est
signalé comme `error` sans qu'un chiffre soit réellement faux — l'arrondi au
5 rappen, par exemple, n'en est pas un.

`PUT /settings/server` écrit `host`, `tls_cert`, `tls_key` et
`allow_insecure_http` dans `config.json`, puis répond `restart_required: true`.
Il ne peut pas appliquer à chaud : l'adresse d'écoute et la configuration TLS
sont choisies une seule fois, au démarrage.

Ce chemin existe parce que `config.json` est écrit **une seule fois**, par
l'assistant de premier démarrage, et jamais retouché : une installation mise à
jour ne contient donc que les clés de sa version d'origine. Toute option ajoutée
depuis était absente, lue comme sa valeur par défaut, et inatteignable.

L'écriture préserve **toutes** les clés inconnues — sérialiser une structure
supprimerait `jwt_secret` au passage, ce qui déconnecterait chaque utilisateur de
sa comptabilité. Le fichier est écrit à côté puis renommé : une coupure en cours
d'écriture laisserait sinon un `config.json` tronqué, et l'application ne
redémarrerait pas.

Certificat et clé doivent être fournis **ensemble**, et leurs chemins sont
vérifiés avant enregistrement : la moitié d'une configuration TLS empêcherait le
serveur de démarrer, et l'utilisateur se serait exclu de ses propres comptes.

`health` expose aussi les **capacités** du produit, la table même qui empêche les
avis de conformité de devenir faux (voir [`compliance/README.md`](../compliance/README.md)).

## Contacts

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/contacts` | auth | Liste |
| GET | `/contacts/:id` | auth | Détail |
| POST | `/contacts` | auth | Créer |
| PATCH | `/contacts/:id` | auth | Modifier |
> **Validation IBAN** (ISO 13616, guide SIX pour les éditeurs de logiciels).
> `POST` et `PATCH` vérifient trois choses, dans cet ordre : la **structure**
> (deux lettres de pays, deux chiffres de clé, alphanumérique ensuite), la
> **longueur imposée par le pays** d'après le registre officiel, puis la **clé
> MOD-97**. Le contrôle de longueur est celui qui manquait : un IBAN suisse à 20
> caractères a environ une chance sur 97 de passer le seul MOD-97, et serait
> rejeté par la banque à la remise du fichier de virements.
>
> Un code pays absent du registre embarqué n'est pas rejeté — il évolue après la
> compilation du binaire, et bloquer une facturation coûterait plus qu'accepter
> un IBAN partiellement vérifié. Une valeur vide vaut « pas d'IBAN » ; les
> espaces sont retirés avant enregistrement.

| POST | `/contacts/:id/anonymise` | **admin** | Anonymise un contact (nLPD art. 6 al. 4 et 32). Efface nom, adresse, courriel, téléphone, IBAN, n° TVA et notes ; conserve la ligne et sa date d'anonymisation. Refuse si une facture du contact ne porte pas son destinataire figé, et refuse un second passage |

> **Anonymiser sans amputer une pièce comptable.** La nLPD art. 6 al. 4 impose
> de détruire ou d'anonymiser une donnée personnelle dès qu'elle n'est plus
> nécessaire, et l'art. 32 ouvre un droit à l'effacement. Le CO art. 958f impose
> en parallèle dix ans de conservation, et la LTVA art. 26 exige que la facture
> nomme son destinataire.
>
> Ce n'est pas contradictoire : ce que la loi commerciale protège est la
> **pièce**, pas la fiche client. Depuis la migration `0014`, chaque facture
> porte l'identité de son destinataire **telle qu'elle était à l'émission**. La
> fiche peut donc être vidée sans qu'aucun document ne perde une mention
> obligatoire — le PDF affiche l'identité figée, pas le contact d'aujourd'hui.
>
> Avant cela, une facture ne stockait que `contact_id` : renommer un client
> réécrivait rétroactivement toutes ses factures passées. Les factures
> antérieures sont complétées au démarrage depuis leur contact et marquées
> `recipient_backfilled = 1` — une reconstitution et une pièce d'origine ne se
> valent pas devant un réviseur.
>
> **Rétention** (nLPD art. 6 al. 4). Les adresses IP des verrouillages de
> connexion sont anonymisées après 90 jours et l'événement supprimé après un an,
> à chaque démarrage. La table annonçait cette limite dans son schéma sans que
> rien ne l'applique. `GET /maintenance/health` expose les chiffres réels sous
> `personal_data` : annoncer une durée sans montrer qu'elle s'applique était
> précisément le défaut.


## TVA et exercices

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/vat/rates` | auth | Taux suisses en vigueur |
| POST | `/vat/declaration` | auth | Déclaration TVA (effective ou TDFN) |
| GET | `/fiscal-years` | auth | Exercices déclarés, du plus récent au plus ancien |
| POST | `/fiscal-years` | **admin** | Déclarer un exercice (`name`, `start_date`, `end_date`). Refuse tout chevauchement |
| POST | `/fiscal-years/:id/close` | **admin** | Clôture (CO art. 958) : vire produits et charges au résultat, verrouille la période, ouvre la suivante |

> **Rattachement à l'exercice.** Chaque écriture et chaque document est rattaché
> à l'exercice couvrant sa date. Si aucun ne la couvre, LedgerAlps crée l'**année
> civile** correspondante : refuser rendrait le produit inutilisable, puisque
> l'installation n'en sème aucun. Pour un exercice décalé (juillet–juin), le
> déclarer via `POST /fiscal-years` **avant** d'y comptabiliser.
>
> Le champ n'était renseigné nulle part avant la v1.4.6. `CloseYear` filtrant
> dessus, il clôturait sans voir aucune écriture : pas d'écriture de clôture, pas
> de garde sur les brouillons, et une réponse `200 closed` malgré tout. Les bases
> existantes sont rattrapées au démarrage.
>
> **Verrouillage de période** (CO art. 958f, Olico art. 3). Un exercice clos
> refuse la création **et** la comptabilisation d'écritures — le second contrôle
> porte le cas qui compte : un brouillon créé avant la clôture, comptabilisé
> après. Réponse `422` avec le motif légal. La correction se passe dans
> l'exercice ouvert.
>
> **Écriture de clôture.** Elle rejoint la chaîne d'empreintes du CO art. 957a
> comme les autres. Elle était auparavant insérée directement en `posted`, sans
> empreinte ni maillon d'audit — la pièce qui vire le résultat était la seule
> hors chaîne.

## Rapports

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/reports/balance-sheet` | auth | Bilan |
| GET | `/reports/income-statement` | auth | Compte de résultat |
| GET | `/reports/general-ledger` | auth | Grand livre |
| GET | `/reports/ar-aging` | auth | Balance âgée des créances |
| GET | `/reports/revenue` | auth | Chiffre d'affaires groupé par `year`, `month` ou `contact`, avec `from`/`to`. La réponse porte `basis` : la convention de calcul, pour qu'un total ne soit pas comparé à un autre calculé autrement |
| GET | `/reports/simplified-accounting` | lecture | Comptabilité simplifiée (CO art. 957 al. 2). `from`, `to` |
| GET | `/reports/simplified-accounting.pdf` | lecture | Le même, en PDF — le document remis à l'administration |
| GET | `/exports/simplified-accounting.csv` | lecture | Le même, en CSV |
| GET | `/stats` | auth | Indicateurs du tableau de bord |

## Paiements et ISO 20022

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/payments` | auth | Liste |
| GET | `/payments/:id` | auth | Détail |
| POST | `/payments` | auth | Enregistrer un paiement |
| GET | `/exports/journal.csv` | lecture | Journal général en CSV. `from`, `to`. Point-virgule + BOM UTF-8, ligne de total |
| GET | `/exports/ledger.csv` | lecture | Grand livre : mouvements par compte avec solde cumulé |
| GET | `/exports/trial-balance.csv` | lecture | Balance de vérification, comptes sans mouvement omis |
| GET | `/payments/payable` | lecture | Factures fournisseurs comptabilisées et non réglées, avec le compte à débiter et, le cas échéant, ce qui empêche le paiement |
| POST | `/payments/export` | écriture comptable | Ordre de virement `pain.001.001.09` |

### Produire un ordre de paiement

La forme utile ne transmet que des identifiants de factures :

```json
{ "execution_date": "2026-08-10", "supplier_invoice_ids": ["…", "…"] }
```

Le créancier, l'IBAN, le montant et la référence sont **relus par le serveur**
dans les livres. Accepter des montants depuis le client reviendrait à laisser
une page web dicter ce qui part à la banque. La forme historique
(`transactions[]` décrivant chaque virement) reste acceptée pour les
intégrations existantes.

Le compte à débiter vient de la fiche entreprise ; il n'y a rien à saisir.

**Générer n'est pas payer** : aucun statut ne change. La facture reste
`booked` jusqu'à ce que le débit apparaisse au relevé, ce qu'établit l'import
camt.053.

Refus, en 422, nommant toujours la facture concernée :

| Cause | Effet |
|---|---|
| Facture plus payable (réglée, annulée) | refus global, avec invitation à recharger |
| Fournisseur sans IBAN | « Aucun IBAN sur la fiche du fournisseur (Contacts → …) » |
| Référence QR sans QR-IBAN | exigence des SIX IG v2.4 §4.2.2 — la référence QR impose un QR-IBAN |
| Référence d'un format inconnu | ni 27 chiffres (QRR) ni ISO 11649 (`RF…`) |
| IBAN de l'entreprise absent ou invalide | renvoie vers Paramètres → Entreprise |

### Ce que le fichier contient

`SvcLvl` n'est posé que pour un virement en **euros** : « SEPA » sur un paiement
en francs vers un IBAN suisse décrit un service que l'opération n'utilise pas et
expose au rejet. La référence QR s'écrit dans `<Prtry>` — « QRR » ne figure pas
dans la liste de codes externes ISO 20022 — tandis que `SCOR`, qui y figure,
s'écrit dans `<Cd>`.
| POST | `/bank-statements/import` | auth | Import relevés `camt.053.001.08` |

La réponse porte `entries` (ce qui a été lu), `count`, puis `imported` et
`duplicate` — ce que l'appel a réellement ajouté et ce qu'il a reconnu comme
déjà présent. Les deux derniers ont manqué longtemps, et l'écran de
rapprochement, qui les lit, annonçait « 0 écriture(s) ajoutée(s) » après chaque
import réussi.

## Audit, conformité, archivage

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/audit-logs` | **admin** | Piste d'audit. `order=asc` (défaut, ordre d'écriture) ou `desc` ; `table_name`, `record_id`, `from`, `to`, `limit`, `offset` |

Chaque maillon porte `before_state` et `after_state`. `before_state` vaut `null`
sur une création — rien ne précédait, ce qui n'est pas la même chose qu'un état
vide. Les valeurs personnelles y sont masquées (nLPD art. 6), des deux côtés et
avant le calcul de l'empreinte.

`after_state` porte en outre `champs_modifies` : les noms des champs dont la
valeur a réellement changé, calculés **avant** le masquage. C'est ce qui rend
lisible un changement d'IBAN, dont les deux valeurs sont masquées et donc
identiques dans la piste. Cette liste entre dans l'empreinte : l'altérer casse
la chaîne.
| GET | `/audit-logs/verify-chain` | **admin** | Vérifier **toute** la chaîne : empreintes, chaînage, continuité des numéros. `200` si intacte, `409` avec le rapport sinon |
| GET | `/audit-logs/attestation` | **admin** | Attestation d'intégrité (Olico art. 9), en pièce jointe JSON : état de la chaîne, empreinte de tête, périmètre, **et ses limites** |
| GET | `/audit-logs/:id/verify` | **admin** | Vérifier une entrée isolée. Détecte une modification de contenu, **pas une suppression** — voir la note ci-dessous |
| POST | `/audit-logs/attestation/verify` | **admin** | Vérifier une attestation qu'on nous présente — la sienne, ou celle qu'un client a remise. Le fichier va dans un formulaire (`file`) ou dans le corps. Rend trois contrôles : le sceau du document, la correspondance de l'empreinte de tête avec les livres au même numéro de séquence, et l'état actuel de la chaîne |
| GET | `/compliance/advisories` | auth | Avis de conformité — voir [compliance](../compliance/README.md) |
| GET | `/security-events` | **admin** | Verrouillages de connexion (contient des adresses IP — nLPD) |
| GET | `/exports/legal-archive` | auth | Archive ZIP 10 ans avec manifeste (CO art. 958f). Contient le JSON **et** un dossier `csv/` — export de réversibilité ouvrable dans un tableur, avec les lignes imbriquées extraites dans leurs propres fichiers |

> **Pourquoi une vérification de chaîne, et pas seulement par entrée.** Vérifier
> une entrée recalcule sa propre empreinte : cela détecte la modification de son
> contenu, mais **pas son effacement**. Supprimer une ligne laisse l'empreinte de
> toutes les autres parfaitement valide. Seuls le chaînage (`prev_hash` de N doit
> valoir `entry_hash` de N−1) et la continuité des numéros de séquence rendent une
> suppression visible. `verify-chain` contrôle les quatre propriétés et nomme
> chaque rupture : `entry_altered`, `link_broken`, `sequence_gap`, `anchor_invalid`
> (ce dernier couvre la troncature du **début** de la chaîne, le seul cas où tous
> les maillons restants restent mutuellement cohérents).
>
> **Limite assumée** : une chaîne tronquée à la **fin** reste cohérente. Rien ne
> distingue « les trois dernières écritures ont été effacées » de « il n'y a pas
> encore eu de quatrième écriture ». C'est la sauvegarde qui répond à cette
> question, et l'écran le dit.
>
> **Entrées antérieures à la v1.4.6** : `hash_version = 1`. Leur empreinte propre
> n'est pas recalculable (voir la migration `0012`), ce que les deux routes
> signalent explicitement — `verifiable: false` et `legacy_entries` — au lieu de
> les compter comme altérées. Leur chaînage, lui, est vérifié normalement.


## Paramètres

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/settings/company` | auth | Profil de l'entreprise |
| PUT | `/settings/company` | **admin** | Modifier le profil |
| POST | `/settings/logo` | auth | Téléverser le logo |
| DELETE | `/settings/logo` | auth | Supprimer le logo |
| POST | `/settings/server/rotate-secret` | **admin** | Régénère le secret de signature des jetons. Déconnecte toutes les sessions et **rien d'autre** : mots de passe intacts (bcrypt), aucune donnée touchée, sauvegardes toujours utilisables — elles ne contiennent pas `config.json`. Prend effet au redémarrage ; les jetons de rafraîchissement en base sont révoqués au passage |
