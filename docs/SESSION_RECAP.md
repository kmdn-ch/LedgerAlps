# LedgerAlps — Récap session (2026-04-08)

> **Charger ce fichier en début de chaque session pour reprendre le contexte complet.**
> Mode de travail : **Team Agency Agents** (5 agents en parallèle)

---

## État actuel : Sprints 1 + 2 + 3 TERMINÉS ✅

Branche active : `go-rewrite` | PR ouverte : **kmdn-ch/LedgerAlps#1** (go-rewrite → main)

Dernier commit : `9e10bb9` — Sprint 3 complet

---

## Architecture Go — Stack retenue

| Composant | Choix |
|-----------|-------|
| HTTP | `gin-gonic/gin` |
| DB locale | SQLite WAL — `modernc.org/sqlite` |
| DB prod | PostgreSQL — `jackc/pgx/v5` |
| Auth | `golang-jwt/jwt/v5` + bcrypt + SHA-256 prehash |
| Migrations | `embed.FS` maison (auto au démarrage) |
| Frontend | React + TypeScript + Tailwind — inchangé |

---

## API complète (18 endpoints)

```
POST /auth/login · /auth/refresh · /auth/logout
GET  /health
GET  /accounts                    POST /accounts
GET  /accounts/trial-balance
GET  /accounts/:code/balance
GET  /contacts                    GET  /contacts/:id
POST /contacts                    PATCH /contacts/:id
GET  /invoices                    GET  /invoices/:id
POST /invoices                    POST /invoices/:id/transition
GET  /journal                     POST /journal
POST /journal/:id/post
GET  /fiscal-years                POST /fiscal-years/:id/close
POST /vat/declaration
```

---

## Fichiers Go créés

```
cmd/server/main.go
.env.go.example
internal/
  api/handlers/  accounts, auth, contacts, context, fiscal_year,
                 invoices, journal, journal_write
  api/middleware/ auth, cors, errors, security
  config/config.go
  core/compliance/swiss.go + swiss_test.go
  core/security/security.go + security_test.go
  db/ db.go, id.go, rebind.go, rebind_test.go
  db/migrations/
    0001_initial.up.sql          (schéma complet + triggers + index)
    0002_seed_plan_comptable.up.sql  (88 comptes PME suisse)
    0003_auth_refresh.up.sql     (refresh_tokens, jti revocation)
  models/models.go
  services/accounting/ service.go, fiscal_year.go
  services/invoicing/service.go
  services/vat/service.go
```

---

## Ce qui reste à implémenter

### Priorité haute
- [ ] PDF génération factures (maroto ou WeasyPrint subprocess)
- [ ] QR-facture payload SPC 0200 (spec Six-Group, référence QRR)
- [ ] /auth/register — création compte utilisateur
- [ ] Seed admin user (endpoint bootstrap ou commande CLI)
- [ ] Tests d'intégration Go (sqlite in-memory, httptest)

### Priorité moyenne
- [ ] ISO 20022 pain.001 export (virements)
- [ ] ISO 20022 camt.053 import (relevés)
- [ ] Frontend : tester toutes les pages contre backend Go
- [ ] Merger PR#1 dans main quand validé

### Priorité basse
- [ ] /auth/register → Login doit persister refresh_token en DB
- [ ] Export ZIP légal annuel (CO art. 958f, conservation 10 ans)
- [ ] Dashboard stats endpoint

---

## Sprints de référence (pour mémoire)

### Sprint 1 ✅ — Blockers critiques (tout corrigé proprement en Go)
### Sprint 2 ✅ — Bugs actifs + performance (tout implémenté)
### Sprint 3 ✅ — Conformité complète (tout implémenté sauf PDF/QR/ISO20022)

---

## Commandes

```bash
# Repo
cd C:/Users/Paul/ledgeralps_final/ledgeralps
git checkout go-rewrite

# Lancer le serveur Go
cp .env.go.example .env
# Éditer .env : JWT_SECRET=<openssl rand -hex 32>
go run ./cmd/server

# Ou binaire compilé
go build -o ledgeralps ./cmd/server && ./ledgeralps

# Tests
go test ./...

# Frontend
cd frontend && npm install && npm run dev
# Pointer VITE_API_URL=http://localhost:8000/api/v1 dans .env.local

# Git
git push origin go-rewrite
# PR#1 : https://github.com/kmdn-ch/LedgerAlps/pull/1
```

---

## Agents de l'équipe (mode Team Agency)

| Agent | Rôle dans le rewrite Go |
|-------|------------------------|
| 🏛️ Software Architect | Architecture Go, DDD, patterns |
| 🏗️ Backend Architect | sqlc, pgx, gin, DB schema |
| 🔒 Security Engineer | JWT, nLPD, OWASP |
| 👁️ Code Reviewer | Qualité Go, idiomes |
| 📋 Compliance Auditor | CO/nLPD/TVA/QR |
