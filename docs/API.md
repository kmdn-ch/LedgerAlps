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
> 5 échecs par IP en 15 minutes déclenchent un verrouillage temporaire
> (`429` + en-tête `Retry-After`).

---

## Authentification

| Méthode | Route | Accès | Description |
|---|---|---|---|
| POST | `/auth/bootstrap` | public | Créer le premier administrateur (une seule fois) |
| POST | `/auth/register` | public | Inscription |
| POST | `/auth/login` | public | Connexion → jetons |
| POST | `/auth/refresh` | public | Renouveler le jeton d'accès |
| POST | `/auth/logout` | public | Révoquer le jeton de rafraîchissement |

## Système

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/health` | public | État du serveur et version (hors `/api/v1`) |
| GET | `/api/v1/uid-lookup` | public | Recherche d'entreprise au registre IDE/ZEFIX |

## Comptabilité

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/accounts` | auth | Plan comptable |
| POST | `/accounts` | auth | Créer un compte |
| GET | `/accounts/trial-balance` | auth | Balance de vérification |
| GET | `/accounts/:code/balance` | auth | Solde d'un compte |
| GET | `/journal` | auth | Écritures du journal |
| POST | `/journal` | auth | Créer une écriture (brouillon) |
| POST | `/journal/:id/post` | auth | Valider une écriture — scellée par hachage (CO art. 957a) |

## Facturation

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/invoices` | auth | Liste paginée |
| GET | `/invoices/:id` | auth | Détail |
| POST | `/invoices` | auth | Créer |
| PATCH | `/invoices/:id` | auth | Modifier |
| POST | `/invoices/:id/transition` | auth | Changer de statut |
| POST | `/invoices/:id/convert` | auth | Convertir une offre en facture |
| POST | `/invoices/:id/outcome` | auth | Enregistrer l'issue d'une offre (`refused`, `expired`) |
| POST | `/invoices/:id/credit-note` | auth | Émettre une note de crédit contre une facture |
| GET | `/invoices/:id/pdf` | auth | PDF — bulletin QR-facture sur les factures uniquement |

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
| GET | `/supplier-invoices` | auth | Liste paginée |
| GET | `/supplier-invoices/:id` | auth | Détail avec lignes |
| POST | `/supplier-invoices` | auth | Créer (refus si doublon fournisseur + référence) |
| POST | `/supplier-invoices/:id/transition` | auth | Changer de statut |
| DELETE | `/supplier-invoices/:id` | auth | Supprimer — brouillons uniquement (CO art. 958f) |

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

## Contacts

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/contacts` | auth | Liste |
| GET | `/contacts/:id` | auth | Détail |
| POST | `/contacts` | auth | Créer |
| PATCH | `/contacts/:id` | auth | Modifier |

## TVA et exercices

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/vat/rates` | auth | Taux suisses en vigueur |
| POST | `/vat/declaration` | auth | Déclaration TVA (effective ou TDFN) |
| GET | `/fiscal-years` | auth | Exercices |
| POST | `/fiscal-years/:id/close` | auth | Clôturer un exercice (CO art. 958) |

## Rapports

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/reports/balance-sheet` | auth | Bilan |
| GET | `/reports/income-statement` | auth | Compte de résultat |
| GET | `/reports/general-ledger` | auth | Grand livre |
| GET | `/reports/ar-aging` | auth | Balance âgée des créances |
| GET | `/stats` | auth | Indicateurs du tableau de bord |

## Paiements et ISO 20022

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/payments` | auth | Liste |
| GET | `/payments/:id` | auth | Détail |
| POST | `/payments` | auth | Enregistrer un paiement |
| POST | `/payments/export` | auth | Export virements `pain.001.001.09` |
| POST | `/bank-statements/import` | auth | Import relevés `camt.053.001.08` |

## Audit, conformité, archivage

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/audit-logs` | auth | Journal d'audit |
| GET | `/audit-logs/:id/verify` | auth | Vérifier l'intégrité de la chaîne |
| GET | `/compliance/advisories` | auth | Avis de conformité — voir [compliance](../compliance/README.md) |
| GET | `/security-events` | **admin** | Verrouillages de connexion (contient des adresses IP — nLPD) |
| GET | `/exports/legal-archive` | auth | Archive ZIP 10 ans avec manifeste (CO art. 958f) |

## Paramètres

| Méthode | Route | Accès | Description |
|---|---|---|---|
| GET | `/settings/company` | auth | Profil de l'entreprise |
| PUT | `/settings/company` | **admin** | Modifier le profil |
| POST | `/settings/logo` | auth | Téléverser le logo |
| DELETE | `/settings/logo` | auth | Supprimer le logo |
