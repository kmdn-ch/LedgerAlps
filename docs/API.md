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
| GET | `/invoices/:id/pdf` | auth | PDF avec bulletin QR-facture |

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
