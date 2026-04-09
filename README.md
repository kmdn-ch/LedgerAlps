# LedgerAlps

**Comptabilité et facturation locale pour les PME et indépendants suisses.**

LedgerAlps est une application **local-first** — vos données restent sur votre machine ou votre serveur, sans cloud, sans abonnement. Conçu pour respecter le Code des obligations (CO), la nLPD, la QR-facture Six-Group SPC 0200 et les standards ISO 20022.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://golang.org)
[![React](https://img.shields.io/badge/React-18-61DAFB.svg)](https://react.dev)
[![CI](https://github.com/kmdn-ch/LedgerAlps/actions/workflows/test.yml/badge.svg)](https://github.com/kmdn-ch/LedgerAlps/actions/workflows/test.yml)

---

## Installation Windows (recommandé)

Téléchargez le dernier installeur depuis la page [Releases](https://github.com/kmdn-ch/LedgerAlps/releases/latest) :

```
LedgerAlps_Setup_<version>_windows_amd64.exe
```

**Ce que fait l'installeur :**
1. Copie `ledgeralps.exe` (lanceur), `ledgeralps-server.exe` et les fichiers frontend dans `Program Files\LedgerAlps\`
2. Crée des raccourcis Bureau et Menu Démarrer

**Premier lancement :**
Double-cliquez sur le raccourci **LedgerAlps**. Un assistant de configuration s'ouvre automatiquement dans votre navigateur — il vous demande de créer votre compte administrateur. Une fois validé, l'application démarre et s'ouvre directement.

> Les données (base de données SQLite et configuration) sont stockées dans `%APPDATA%\LedgerAlps\` et sont préservées lors des mises à jour et désinstallations.

---

## Stack

| Couche | Technologies |
|--------|-------------|
| Backend | Go 1.26 · Gin · SQLite WAL (local) · PostgreSQL (production) |
| Auth | JWT (access + refresh) · bcrypt + SHA-256 prehash · jti revocation |
| Migrations | `embed.FS` — compilées dans le binaire, appliquées au démarrage |
| Frontend | React 18 · TypeScript · Tailwind CSS · Vite |
| PDF | go-pdf/fpdf · skip2/go-qrcode |
| Lanceur Windows | Go · `-H=windowsgui` · assistant de configuration intégré |

> **Binaire unique, zéro configuration** — un seul fichier `ledgeralps-server` à copier sur votre serveur. SQLite embarqué par défaut, PostgreSQL optionnel via `POSTGRES_DSN`. Sur Windows, `ledgeralps.exe` gère le démarrage, la configuration et l'ouverture du navigateur automatiquement.

---

## Fonctionnalités

| Domaine | Détail |
|---------|--------|
| **Lanceur Windows** | Assistant premier démarrage (navigateur), génération automatique du JWT_SECRET, bootstrap admin |
| **Facturation** | Création, transitions (draft→sent→paid/cancelled), PDF A4 + slip QR suisse |
| **Comptabilité** | Journal en partie double, Grand Livre, Balance de vérification |
| **Rapports** | Bilan, Compte de résultat, Grand Livre détaillé, Balance âgée créances |
| **Paiements** | CRUD paiements avec écriture comptable automatique |
| **QR-facture** | Payload SPC 0200 complet, QRR MOD-10 récursif, Swico S1 |
| **TVA suisse** | Méthode effective + TDFN (AFC 318/100), taux 2024 : 8.1 % / 2.6 % / 3.8 %, arrondi 0.05 CHF |
| **ISO 20022** | Export virements `pain.001.001.09`, import relevés `camt.053.001.08` |
| **Clôture exercice** | Transaction atomique : P&L → 5900, scellement, création exercice suivant (CO art. 958) |
| **Audit immuable** | Hash SHA-256 chaîné sur chaque écriture postée (CO art. 957a) |
| **Export légal** | Archive ZIP 10 ans avec manifest SHA-256, IBAN masqué nLPD (CO art. 958f) |
| **Sécurité** | CORS allowlist, headers OWASP, timing-attack prevention |
| **Plan comptable** | 88 comptes PME suisse (FIDUCIAIRE\|SUISSE) chargés automatiquement |

---

## API — 35 endpoints

```
POST   /api/v1/auth/bootstrap             Créer le premier admin (one-shot)
POST   /api/v1/auth/register              Inscription
POST   /api/v1/auth/login                 Connexion → access_token + refresh_token
POST   /api/v1/auth/refresh               Renouveler l'access token
POST   /api/v1/auth/logout                Révoquer le refresh token

GET    /health                            État du serveur

GET    /api/v1/accounts                   Liste des comptes
POST   /api/v1/accounts                   Créer un compte
GET    /api/v1/accounts/trial-balance     Balance de vérification
GET    /api/v1/accounts/:code/balance     Solde d'un compte

GET    /api/v1/contacts                   Liste des contacts
GET    /api/v1/contacts/:id               Détail d'un contact
POST   /api/v1/contacts                   Créer un contact
PATCH  /api/v1/contacts/:id               Mettre à jour un contact

GET    /api/v1/invoices                   Liste des factures (paginée)
GET    /api/v1/invoices/:id               Détail d'une facture
GET    /api/v1/invoices/:id/pdf           PDF avec QR-bill
POST   /api/v1/invoices                   Créer une facture
POST   /api/v1/invoices/:id/transition    Transition de statut

GET    /api/v1/journal                    Liste des écritures
POST   /api/v1/journal                    Créer une écriture (draft)
POST   /api/v1/journal/:id/post           Valider une écriture (hash CO art. 957a)

GET    /api/v1/fiscal-years               Liste des exercices fiscaux
POST   /api/v1/fiscal-years/:id/close     Clôturer un exercice (admin)
POST   /api/v1/vat/declaration            Déclaration TVA
GET    /api/v1/vat/rates                  Taux TVA suisses 2024

GET    /api/v1/reports/balance-sheet      Bilan
GET    /api/v1/reports/income-statement   Compte de résultat
GET    /api/v1/reports/general-ledger     Grand Livre
GET    /api/v1/reports/ar-aging           Balance âgée créances

POST   /api/v1/payments                   Enregistrer un paiement
GET    /api/v1/payments                   Liste des paiements
GET    /api/v1/payments/:id               Détail d'un paiement

GET    /api/v1/audit-logs                 Journal d'audit
GET    /api/v1/audit-logs/:id/verify      Vérifier l'intégrité

POST   /api/v1/payments/export            Export ISO 20022 pain.001
POST   /api/v1/bank-statements/import     Import ISO 20022 camt.053
GET    /api/v1/exports/legal-archive      Archive ZIP légale (CO art. 958f)
GET    /api/v1/stats                      Statistiques tableau de bord
```

---

## Démarrage rapide (développement)

### Prérequis

- **Go ≥ 1.22** — [télécharger](https://go.dev/dl/)
- **Node.js ≥ 20** — pour le frontend
- **Git**

### Installation

```bash
git clone https://github.com/kmdn-ch/LedgerAlps.git
cd LedgerAlps

# Générer un secret JWT fort
export JWT_SECRET=$(openssl rand -hex 32)

# Démarrer le serveur (migrations appliquées automatiquement)
go run ./cmd/server

# Frontend (second terminal)
cd frontend && npm install && npm run dev
```

| Service | URL |
|---------|-----|
| Frontend | http://localhost:5173 |
| API | http://localhost:8000 |
| Health | http://localhost:8000/health |

### Créer le premier admin

```bash
curl -X POST http://localhost:8000/api/v1/auth/bootstrap \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@votre-entreprise.ch","name":"Administrateur","password":"VotreMotDePasseFort!"}'
```

> Sur Windows avec l'installeur, cette étape est faite automatiquement par l'assistant de configuration.

---

## Configuration

La configuration est lue dans l'ordre suivant :

1. **Fichier JSON** — `%APPDATA%\LedgerAlps\config.json` (Windows) ou `~/.ledgeralps/config.json` (Linux/macOS)
   Généré automatiquement par le lanceur Windows au premier démarrage.
2. **Variables d'environnement** — pour les déploiements Docker / CI / serveur.

```bash
PORT=8000
DEBUG=false
SQLITE_PATH=ledgeralps.db
# POSTGRES_DSN=postgres://ledgeralps:motdepasse@localhost:5432/ledgeralps?sslmode=disable
JWT_SECRET=REMPLACER_PAR_openssl_rand_hex_32   # obligatoire, min 32 chars
ALLOWED_ORIGINS=http://localhost:5173
```

---

## Build

```bash
# Natif
go build -o ledgeralps-server ./cmd/server

# Lanceur Windows (sans fenêtre console)
GOOS=windows GOARCH=amd64 go build -ldflags="-H=windowsgui" -o ledgeralps.exe ./cmd/launcher

# Installeur Windows complet (nécessite NSIS ou Inno Setup dans PATH)
make release
```

---

## Tests

```bash
go test ./...
go test -race ./...
go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out
```

| Package | Tests |
|---------|-------|
| `core/compliance` | RoundTo5Rappen, ValidateIBAN, ValidateQRIBAN, QR-bill SPC 0200, QRR MOD-10 |
| `core/security` | HashPassword, CheckPassword, JWT generate/parse, ChainHash |
| `db` | Rebind (SQLite↔PG placeholders) |
| `integration` | Bootstrap, Login, Refresh, Logout, Journal, TrialBalance, Contact, Invoice, Auth |

---

## Structure du projet

```
LedgerAlps/
├── cmd/
│   ├── server/main.go          Serveur API + fichiers frontend statiques
│   ├── launcher/main.go        Lanceur Windows (assistant config + démarrage serveur)
│   └── cli/                    CLI admin (migrate, bootstrap, health)
├── internal/
│   ├── api/handlers/           Auth, Accounts, Contacts, Invoices, Journal,
│   │                           Reports, Payments, Audit, FiscalYear, Stats, Export
│   ├── api/middleware/         RequireAuth, CORS, SecurityHeaders, ErrorHandler
│   ├── config/config.go        Config JSON + env vars, validation JWT_SECRET
│   ├── core/compliance/        Arrondi TVA, IBAN, QR-bill SPC 0200
│   ├── core/security/          bcrypt+SHA256, JWT access+refresh, ChainHash
│   ├── db/                     SQLite WAL / PostgreSQL, migrations embed.FS
│   │   └── migrations/         0001_initial · 0002_plan_comptable · 0003_auth_refresh
│   ├── integration/            Tests end-to-end httptest + sqlite
│   ├── models/                 Entités métier
│   └── services/               accounting · invoicing · pdf · vat · iso20022
├── frontend/src/               React 18 · TypeScript · Tailwind CSS
├── infrastructure/
│   ├── windows/installer.nsi   Installeur NSIS Windows
│   └── linux/                  Systemd service, scripts deb/rpm
├── installer/ledgeralps.iss    Script Inno Setup (alternatif)
└── .goreleaser.yaml            Build multi-plateforme + releases GitHub
```

---

## Conformité légale suisse

| Norme | Implémentation |
|-------|---------------|
| CO art. 957 | Partie double, `sum(débit) == sum(crédit)` vérifié à la création |
| CO art. 957a | Triggers SQL + hash SHA-256 chaîné sur chaque écriture postée |
| CO art. 958 | Clôture atomique, P&L → 5900 |
| CO art. 958f | Archive ZIP 10 ans avec manifest SHA-256 |
| nLPD art. 6 | IBAN/email masqués dans les logs, isolation par utilisateur |
| TVA CH 2024 | 8.1 % / 2.6 % / 3.8 %, méthode effective et TDFN, arrondi 0.05 CHF |
| QR-facture SPC 0200 v2.3 | Payload complet, QRR MOD-10, Swico S1 |
| IBAN / QR-IBAN | Validation MOD-97, IID QR-IBAN 30000–31999 |

> **Avertissement** : LedgerAlps facilite la conformité légale suisse mais ne remplace pas la validation par un expert-comptable agréé.

---

## Licence

MIT — voir [LICENSE](LICENSE).

Copyright (c) 2024–2026 LedgerAlps Contributors
