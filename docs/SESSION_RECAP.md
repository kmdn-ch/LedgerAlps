# LedgerAlps — Récap session (2026-04-08)

> **Charger ce fichier en début de chaque session pour reprendre le contexte complet.**
> Mode de travail : **Team Agency Agents** (5 agents en parallèle)

---

## 🎯 Décision stratégique : RÉÉCRITURE EN GO ✅

**Décidé le 2026-04-08.**

### Objectif
Zéro configuration manuelle. Installation = télécharger un binaire et le lancer.

```bash
./ledgeralps   # ← c'est tout. DB créée, migrations appliquées, serveur démarré.
```

### Architecture Go retenue

| Composant | Choix |
|-----------|-------|
| HTTP | `gin` |
| DB locale (défaut) | **SQLite** WAL mode — zéro infrastructure |
| DB production | PostgreSQL via `pgx` — même code, switch par env var |
| Queries | `sqlc` — SQL typé, généré |
| Migrations | `golang-migrate` + `embed.FS` embarquées dans le binaire |
| Auth | `golang-jwt/jwt` + `golang.org/x/crypto/bcrypt` |
| PDF | `maroto` ou WeasyPrint subprocess |
| QR-facture | Implémentation directe spec Six-Group |
| ISO 20022 | `encoding/xml` natif Go |
| Frontend | **Inchangé** — React + TypeScript + Tailwind |

### Pourquoi Go vs Python
- Binaire unique compilé — aucune dépendance runtime
- Image Docker ~12 MB vs ~500 MB Python
- SQLite embarqué → pas de PostgreSQL à installer pour usage local SME
- Cross-compilation triviale (Windows/Mac/Linux depuis une machine)
- Performance 5-10× supérieure
- Codebase Python v0.1.0 encore jeune → bon moment pour rebasculer

---

## État du projet

- **Version Python** : v0.1.0 taguée — https://github.com/kmdn-ch/LedgerAlps
- **Branches** : `main` (stable Python) · `test` (intégration) · `go-rewrite` (à créer)
- **Score audit équipe** : 6.7 / 10 — fondations solides, non prêt production
- **Stack cible** : Go + SQLite/PostgreSQL / React + TypeScript + Tailwind

---

## Prochaine session — Plan d'action

### Étape 1 — Scaffolding Go (branche `go-rewrite`)
```
cmd/server/main.go          Point d'entrée — autorun DB + migrations + serveur
internal/
  config/                   Config via env vars (SQLite ou PostgreSQL)
  db/                       sqlc generated + migrations embed.FS
  models/                   Structs Go (Account, JournalEntry, Invoice, Contact…)
  services/
    accounting/             Moteur partie double
    invoicing/              Facturation + PDF
    vat/                    Calcul TVA CH
    swiss/                  QR-facture + ISO 20022
  api/
    handlers/               Gin handlers
    middleware/             Auth JWT, rate limit, audit
  core/
    security/               JWT, bcrypt
    compliance/             Règles CO, nLPD
migrations/                 Fichiers SQL versionnés (embed)
frontend/                   Inchangé
```

### Étape 2 — Sprint 1 Go (blockers critiques, écrits proprement dès le départ)
Reprendre les 10 items du Sprint 1 mais implémentés correctement en Go :
- Hash chain `prev_hash` natif dès le départ
- Numérotation via séquences DB (SQLite autoincrement / PG SEQUENCE)
- Binaire non-root par design (pas de Docker root)
- Secrets validés au démarrage avec `os.Exit(1)` si valeur par défaut

---

## Agents de l'équipe (mode Team Agency)

| Agent | Fichier | Rôle dans le rewrite Go |
|-------|---------|------------------------|
| 🏛️ Software Architect | `engineering-software-architect.md` | Architecture Go, DDD, patterns |
| 🏗️ Backend Architect | `engineering-backend-architect.md` | sqlc, pgx, gin, DB schema |
| 🔒 Security Engineer | `engineering-security-engineer.md` | JWT, nLPD, OWASP |
| 👁️ Code Reviewer | `engineering-code-reviewer.md` | Qualité Go, idiomes |
| 📋 Compliance Auditor | `specialized/compliance-auditor.md` | CO/nLPD/TVA/QR |

Chemin agents : `~/.claude/agents/`

---

## Ce qui a été fait (session 2026-04-07/08)

1. `GET /api/v1/journal` — pagination + filtres
2. `PATCH /api/v1/contacts/{id}` — mise à jour partielle
3. Migration Alembic `0001_initial.py` — 9 tables, 4 enums PostgreSQL
4. `InvoiceDetailPage.tsx` + `PDFPreview.tsx` + route `/invoices/:invoiceId`
5. Tests d'intégration : factures (8), journal (3), contacts PATCH (2)
6. `.env.example`, README complet, CHANGELOG, release v0.1.0
7. Branche `test` + versioning SemVer sur GitHub
8. Audit équipe 5 agents — 32 issues identifiées, 3 sprints planifiés
9. **Décision : réécriture Go** pour installation zéro-config

---

## Sprints planifiés (référence — à implémenter en Go)

### Sprint 1 — Blockers critiques
`prev_hash` hash chain · Trigger immutabilité DB · Pas de port 5432 exposé · SEQUENCE numérotation · Arrondi 0.05 CHF · Validator IBAN · Non-root · Secrets validés · DEBUG=false par défaut

### Sprint 2 — Bugs actifs + performance
Route shadowing trial-balance · Handler global exceptions · Ordre journal/status dans transition() · Contrepassation SENT→CANCELLED · `@requires_double_entry` async · `sum(start=0)` · N+1 trial-balance → GROUP BY · Pagination list endpoints · Indexes DB · `pool_pre_ping` · Validator JournalLineCreate · `verify_entry_integrity()`

### Sprint 3 — Conformité complète
FiscalYearService.close_year() · require_admin appliqué · Isolation données utilisateur · /auth/refresh + révocation JWT · DPA template nLPD · mask_personal_data() avant AuditLog · TDFN dans _post_to_journal() · VATDeclarationService · CreDtTm 'Z' ISO 20022 · Parser CreDtTm camt.053 · Email hors JWT · HSTS conditionnel HTTPS

---

## Commandes utiles

```bash
# Repo
cd C:/Users/Paul/ledgeralps_final/ledgeralps

# Go (à venir)
go build -o ledgeralps ./cmd/server && ./ledgeralps

# Python (version actuelle)
make up && make migrate && make seed
make test

# Git
git checkout -b go-rewrite
git push -u origin go-rewrite
git tag -a vX.Y.Z -m "..." && git push origin vX.Y.Z
```
