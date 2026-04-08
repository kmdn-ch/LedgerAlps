# LedgerAlps

**Comptabilité et facturation locale pour les PME et indépendants suisses.**

LedgerAlps est une application **local-first** — vos données restent sur votre machine ou votre serveur, sans cloud, sans abonnement. Conçu pour respecter le Code des obligations (CO), la nLPD, la QR-facture Six-Group SPC 0200 et les standards ISO 20022.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://golang.org)
[![React](https://img.shields.io/badge/React-18-61DAFB.svg)](https://react.dev)

---

## Stack

| Couche | Technologies |
|--------|-------------|
| Backend | Go 1.26 · Gin · SQLite WAL (local) · PostgreSQL (production) |
| Auth | JWT (access + refresh) · bcrypt + SHA-256 prehash · jti revocation |
| Migrations | `embed.FS` — compilées dans le binaire, appliquées au démarrage |
| Frontend | React 18 · TypeScript · Tailwind CSS · Vite |
| PDF | go-pdf/fpdf · skip2/go-qrcode |

> **Binaire unique, zéro configuration** — un seul fichier `ledgeralps` à copier sur votre serveur. SQLite embarqué par défaut, PostgreSQL optionnel via `POSTGRES_DSN`.

---

## Fonctionnalités

| Domaine | Détail |
|---------|--------|
| **Facturation** | Création, transition de statut (draft→sent→paid/cancelled), PDF A4 avec slip de paiement QR suisse |
| **Comptabilité** | Journal en partie double, Grand Livre, Balance de vérification (trial balance) |
| **QR-facture** | Payload SPC 0200 complet, référence QRR (MOD-10 récursif), Swico S1, QR code PNG embarqué dans le PDF |
| **TVA suisse** | Méthode effective + TDFN (AFC 318/100), taux 2024 : 8.1 % / 2.6 % / 3.8 %, arrondi 0.05 CHF (floor-half-up) |
| **Clôture exercice** | Transaction atomique : solde P&L → compte 5900, scellement de l'exercice, création du suivant (CO art. 958) |
| **Audit immuable** | Chaque écriture postée est protégée par hash SHA-256 chaîné — toute altération est détectable (CO art. 957a) |
| **Sécurité** | CORS allowlist explicite, headers OWASP (CSP, HSTS, X-Frame-Options), timing-attack prevention sur login |
| **Conformité nLPD** | Données personnelles masquées dans les logs d'audit, isolation des données par utilisateur |
| **Plan comptable** | 88 comptes PME suisse (FIDUCIAIRE\|SUISSE) chargés automatiquement à l'installation |

---

## API — 21 endpoints

```
POST   /api/v1/auth/bootstrap          Créer le premier admin (one-shot)
POST   /api/v1/auth/register           Inscription utilisateur
POST   /api/v1/auth/login              Connexion → access_token + refresh_token
POST   /api/v1/auth/refresh            Renouveler l'access token
POST   /api/v1/auth/logout             Révoquer le refresh token

GET    /health                         État du serveur

GET    /api/v1/accounts                Liste des comptes
POST   /api/v1/accounts                Créer un compte
GET    /api/v1/accounts/trial-balance  Balance de vérification (1 requête GROUP BY)
GET    /api/v1/accounts/:code/balance  Solde d'un compte

GET    /api/v1/contacts                Liste des contacts
GET    /api/v1/contacts/:id            Détail d'un contact
POST   /api/v1/contacts                Créer un contact
PATCH  /api/v1/contacts/:id            Mettre à jour un contact (PATCH partiel)

GET    /api/v1/invoices                Liste des factures (paginée)
GET    /api/v1/invoices/:id            Détail d'une facture
GET    /api/v1/invoices/:id/pdf        Télécharger la facture en PDF avec QR-bill
POST   /api/v1/invoices                Créer une facture
POST   /api/v1/invoices/:id/transition Transition de statut (draft/sent/paid/cancelled)

GET    /api/v1/journal                 Liste des écritures comptables
POST   /api/v1/journal                 Créer une écriture (draft)
POST   /api/v1/journal/:id/post        Valider une écriture (hash chaîné CO art. 957a)

GET    /api/v1/fiscal-years            Liste des exercices fiscaux
POST   /api/v1/fiscal-years/:id/close  Clôturer un exercice (admin)
POST   /api/v1/vat/declaration         Générer une déclaration TVA (admin)
```

---

## Démarrage rapide

### Prérequis

- **Go ≥ 1.22** — [télécharger](https://go.dev/dl/)
- **Node.js ≥ 20** — pour le frontend
- **Git**

### Installation

```bash
# 1. Cloner le dépôt
git clone https://github.com/kmdn-ch/LedgerAlps.git
cd LedgerAlps

# 2. Configurer l'environnement
cp .env.go.example .env
# Remplacer JWT_SECRET par une valeur forte (≥ 32 chars) :
# Linux/macOS : export JWT_SECRET=$(openssl rand -hex 32)
# Windows PS  : $env:JWT_SECRET = -join ((48..57+65..90+97..122) | Get-Random -Count 32 | % {[char]$_})

# 3. Lancer le serveur Go (migrations appliquées automatiquement)
go run ./cmd/server

# 4. Frontend (dans un second terminal)
cd frontend && npm install && npm run dev
```

Accès :

| Service | URL |
|---------|-----|
| Frontend | http://localhost:5173 |
| API | http://localhost:8000 |
| Health check | http://localhost:8000/health |

### Premier démarrage — créer l'admin

Lors du tout premier démarrage, aucun utilisateur n'existe. Créez le compte admin :

```bash
curl -X POST http://localhost:8000/api/v1/auth/bootstrap \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@votre-entreprise.ch","name":"Administrateur","password":"VotreMotDePasseFort!"}'
```

> `POST /auth/bootstrap` ne fonctionne qu'une seule fois. Si des utilisateurs existent déjà, il retourne 409.

---

## Configuration

Variables d'environnement (fichier `.env`) :

```bash
# ── Serveur ───────────────────────────────────────────────────────────────────
PORT=8000
DEBUG=false
LOG_LEVEL=INFO

# ── Base de données ───────────────────────────────────────────────────────────
# Par défaut : SQLite (créé automatiquement, aucun service à démarrer)
SQLITE_PATH=ledgeralps.db

# Production : PostgreSQL (décommenter et renseigner)
# POSTGRES_DSN=postgres://ledgeralps:motdepasse@localhost:5432/ledgeralps?sslmode=disable

# ── Auth ──────────────────────────────────────────────────────────────────────
# OBLIGATOIRE — le serveur refuse de démarrer si absent ou < 32 chars
JWT_SECRET=REMPLACER_PAR_openssl_rand_hex_32

# ── CORS ──────────────────────────────────────────────────────────────────────
ALLOWED_ORIGINS=http://localhost:5173

# ── Informations de l'entreprise (pour les PDF de factures) ──────────────────
COMPANY_NAME=LedgerAlps AG
COMPANY_ADDRESS=Bahnhofstrasse 1
COMPANY_CITY=8001 Zürich
COMPANY_COUNTRY=CH
COMPANY_IBAN=CH93 0076 2011 6238 5295 7          # IBAN standard
COMPANY_QR_IBAN=CH44 3199 9123 0008 8901 2        # QR-IBAN (slip de paiement QR)
COMPANY_VAT_NUMBER=CHE-123.456.789 MWST
```

---

## Build — binaire unique

```bash
# Compiler
go build -o ledgeralps ./cmd/server

# Lancer (les migrations sont appliquées automatiquement)
JWT_SECRET=$(openssl rand -hex 32) ./ledgeralps

# Cross-compilation Linux (depuis macOS/Windows)
GOOS=linux GOARCH=amd64 go build -o ledgeralps-linux ./cmd/server
```

---

## Tests

```bash
# Tests unitaires (compliance, sécurité, DB)
go test ./internal/core/... ./internal/db/...

# Tests d'intégration end-to-end (sqlite temp-file + httptest)
go test ./internal/integration/... -v

# Suite complète
go test ./...

# Avec couverture
go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out
```

**Couverture actuelle :**

| Package | Tests |
|---------|-------|
| `core/compliance` | RoundTo5Rappen, ValidateIBAN, ValidateQRIBAN, QR-bill SPC 0200, QRR MOD-10 |
| `core/security` | HashPassword, CheckPassword, JWT generate/parse, ChainHash |
| `db` | Rebind (SQLite↔PG placeholders) |
| `integration` | Bootstrap, Register, Login, Refresh, Logout, Journal, TrialBalance, Contact, Invoice, Auth enforcement |

---

## Structure du projet

```
LedgerAlps/
├── cmd/server/main.go              Point d'entrée — config → DB → migrations → Gin → routes
├── .env.go.example                 Modèle de configuration
├── internal/
│   ├── api/
│   │   ├── handlers/               Handlers Gin (auth, accounts, contacts, invoices, journal,
│   │   │                           fiscal_year, invoice_pdf)
│   │   └── middleware/             RequireAuth, RequireAdmin, CORS, SecurityHeaders, ErrorHandler
│   ├── config/config.go            Validation secrets au démarrage (os.Exit si JWT_SECRET faible)
│   ├── core/
│   │   ├── compliance/             RoundTo5Rappen, ValidateIBAN, ValidateQRIBAN, QR-bill SPC 0200
│   │   └── security/               HashPassword (bcrypt+SHA256), JWT access+refresh, ChainHash
│   ├── db/
│   │   ├── db.go                   Open (SQLite WAL / PG), Migrate (embed.FS atomique)
│   │   ├── id.go                   NewID() — UUID crypto/rand (compatible SQLite + PG)
│   │   ├── rebind.go               Rebind() — convertit ? en $1,$2,… pour PostgreSQL
│   │   └── migrations/
│   │       ├── 0001_initial.up.sql         Schéma complet + triggers immuabilité CO art. 957a
│   │       ├── 0002_seed_plan_comptable.up.sql  88 comptes PME suisse (idempotent)
│   │       └── 0003_auth_refresh.up.sql    Table refresh_tokens (jti, revoked_at)
│   ├── integration/                Tests end-to-end (httptest + sqlite temp-file)
│   ├── models/models.go            Entités métier (User, Account, Invoice, JournalEntry…)
│   └── services/
│       ├── accounting/             CreateEntry (partie double), PostEntry (hash chaîné), CloseYear
│       ├── invoicing/              CreateInvoice (arrondi 5 Rappen), Transition + contrepassation
│       ├── pdf/                    Génération PDF A4 + slip QR suisse (fpdf + go-qrcode)
│       └── vat/                    VATDeclarationService (méthode effective + TDFN, AFC 318/100)
├── frontend/
│   └── src/
│       ├── pages/                  Dashboard, Factures, Journal, Contacts, Comptes, Rapports
│       ├── components/             Composants UI réutilisables
│       ├── api/client.ts           Client API TypeScript centralisé
│       └── store/                  Auth store (access + refresh token)
└── docs/
    └── SESSION_RECAP.md            Récap de session pour reprendre le contexte
```

---

## Conformité légale suisse

| Norme | Implémentation |
|-------|---------------|
| CO art. 957 — Comptabilité obligatoire | Partie double, `sum(débit) == sum(crédit)` vérifié à la création |
| CO art. 957a — Immuabilité des écritures | Triggers SQL `BEFORE UPDATE/DELETE` + hash SHA-256 chaîné sur chaque écriture postée |
| CO art. 958 — Clôture d'exercice | `FiscalYearService.CloseYear()` : transaction atomique, P&L → 5900, scellement |
| nLPD art. 6 — Minimisation des données | Email/IBAN/téléphone masqués dans les logs d'audit, isolation par `created_by_id` |
| TVA CH 2024 — AFC 318/100 | Taux 8.1 % / 2.6 % / 3.8 %, méthode effective et TDFN, arrondi `floor(x×20+0.5)/20` |
| QR-facture Six-Group SPC 0200 v2.3 | Payload complet, référence QRR MOD-10 récursif, Swico S1 (//S1/10/…/11/…) |
| IBAN / QR-IBAN | Validation MOD-97, IID QR-IBAN 30000–31999 (Six-Group) |
| nLPD — Local-first | Aucun cloud, données sur votre infrastructure uniquement |

> **Avertissement** : LedgerAlps facilite la conformité légale suisse mais ne remplace pas la validation par un expert-comptable agréé (fiduciaire).

---

## Sécurité

- **JWT** : access token (60 min) + refresh token (30 jours) avec `jti` en base pour révocation individuelle
- **bcrypt + SHA-256 prehash** : évite la troncation silencieuse bcrypt à 72 bytes — `prehash = sha256(password)` avant bcrypt
- **Timing attack** : `dummyHash` bcrypt pré-calculé au démarrage — le login brûle toujours ~100 ms même si l'email est inconnu
- **CORS** : allowlist explicite (jamais `*`), `Access-Control-Allow-Credentials: true`
- **Headers OWASP** : `Content-Security-Policy`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy`, `HSTS`
- **Secrets** : `JWT_SECRET` vérifié au démarrage — `os.Exit(1)` si < 32 chars ou valeur connue faible

---

## Contribution

Les contributions sont bienvenues. Merci de respecter ces principes :

- Les écritures comptables respectent la partie double (`sum(débit) == sum(crédit)`)
- Les nouvelles migrations sont idempotentes (`CREATE TABLE IF NOT EXISTS`, `INSERT OR IGNORE`)
- Les nouveaux endpoints sont couverts par des tests dans `internal/integration/`
- Le code Go suit les conventions idiomatiques (`go vet`, pas de `panic` dans les handlers)

```bash
# Avant de soumettre une PR
go build ./...
go vet ./...
go test ./...
```

---

## Licence

MIT — voir [LICENSE](LICENSE).

Copyright (c) 2024–2026 LedgerAlps Contributors
