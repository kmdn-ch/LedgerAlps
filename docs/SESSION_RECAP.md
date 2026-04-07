# LedgerAlps — Récap session (2026-04-07)

> Charger ce fichier en début de session pour reprendre le contexte complet.

---

## État du projet

- **Version** : v0.1.0 taguée et publiée sur GitHub
- **Repo** : https://github.com/kmdn-ch/LedgerAlps
- **Branches** : `main` (stable) · `test` (intégration)
- **Score audit équipe** : 6.7 / 10 — fondations solides, non prêt production
- **Stack actuelle** : FastAPI + SQLAlchemy async / React + TypeScript + Tailwind / PostgreSQL 16

---

## Ce qui a été fait cette session

1. Implémenté `GET /api/v1/journal` — pagination + filtres (date, status, référence)
2. Implémenté `PATCH /api/v1/contacts/{id}` — mise à jour partielle
3. Créé migration Alembic initiale `0001_initial.py` — 9 tables, 4 enums PostgreSQL
4. Créé `InvoiceDetailPage.tsx` — détail facture, transitions statut, aperçu PDF inline
5. Créé composant `PDFPreview.tsx` — iframe avec objectURL + cleanup
6. Ajouté route `/invoices/:invoiceId` dans `router.tsx`
7. Enrichi `InvoiceResponse` avec `notes` et `terms`
8. Ajouté `ContactUpdate` schema (champs optionnels)
9. Écrit tests d'intégration : `TestInvoicesEndpoints` (8 tests), `TestJournalEndpoints` (3), `TestContactsPatch` (2)
10. Créé `.env.example` documenté
11. Réécrit `README.md` complet (installation, make, conformité)
12. Créé `CHANGELOG.md` (Keep a Changelog + SemVer)
13. Publié release GitHub `v0.1.0` avec tag annoté
14. Créé branche `test` sur GitHub
15. Audit complet par équipe 5 agents — rapport détaillé

---

## Question stratégique ouverte : Réécrire en Go ?

**Objectif formulé** : Simplifier et automatiser l'installation au maximum. Zéro configuration manuelle.

**Arguments pour Go :**
- Binaire unique compilé → `./ledgeralps` suffit pour démarrer
- Zéro dépendance Python/pip/venv/virtualenv
- Image Docker ~10 MB (scratch/alpine) vs ~500 MB Python
- Option SQLite embarqué → zéro infrastructure (pas de PostgreSQL à installer)
- Compilation croisée triviale (Windows/Mac/Linux depuis une seule machine)
- Typage strict → moins de bugs runtime
- Performance 5-10× supérieure

**Arguments pour garder Python :**
- Codebase existante (mais 4 semaines de travail, ~2000 lignes)
- Bibliothèques QR-facture (`qrbill`), PDF (`weasyprint`) matures en Python
- FastAPI + Pydantic = productivité élevée

**Recommandation d'équipe :** Réécrire en Go avec SQLite local-first
- `go build` → un seul exécutable
- SQLite embarqué pour usage local SME (migrations auto au démarrage)
- PostgreSQL optionnel via variable d'env pour usage multi-utilisateurs
- Migrations : `golang-migrate` avec fichiers SQL embarqués (`embed.FS`)
- HTTP : `gin` ou `echo`
- Crypto/JWT : `golang-jwt/jwt` + `golang.org/x/crypto/bcrypt`
- PDF : `maroto` ou appel weasyprint en sous-processus
- QR-facture : implémentation directe (spec publique Six-Group)

**Décision à prendre en début de prochaine session.**

---

## Sprints planifiés (si on garde Python)

### Sprint 1 — Blockers critiques (AVANT déploiement)
```
S1-01  journal.py/_write_audit()     Calculer prev_hash (chaîne SHA-256)
S1-02  Migration Alembic              Trigger PG immutabilité écritures POSTED
S1-03  docker-compose.yml            Supprimer ports: "5432:5432"
S1-04  service.py invoicing          PostgreSQL SEQUENCE pour numéros factures
S1-05  journal.py                    PostgreSQL SEQUENCE pour références journal
S1-06  service.py/_compute_totals()  Arrondi total facture à 0.05 CHF
S1-07  schemas/__init__.py           Validator IBAN dans InvoiceCreate + ContactCreate
S1-08  Dockerfile                    USER ledgeralps (non-root)
S1-09  core/config.py               Rejeter SECRET_KEY = valeurs par défaut connues
S1-10  .env.example                  DEBUG=false par défaut
```

### Sprint 2 — Bugs actifs + performance
```
S2-01  main.py:42-53               Route /trial-balance avant /{number}/balance
S2-02  app/main.py                  Handler global AccountingError → HTTP 422
S2-03  service.py:108-112          Journal d'abord, mutation status ensuite
S2-04  service.py                   Contrepassation sur SENT→CANCELLED
S2-05  compliance.py:23-33         @requires_double_entry async + logique réelle
S2-06  journal.py:238               sum(start=Decimal("0"))
S2-07  journal.py:208-232          get_trial_balance() → requête GROUP BY unique
S2-08  main.py                      Pagination sur list_invoices/contacts/accounts
S2-09  Migration 0002               Indexes journal_lines + invoices
S2-10  session.py                   pool_pre_ping=True
S2-11  schemas/__init__.py          Validator croisé JournalLineCreate
S2-12  journal.py                   verify_entry_integrity() en lecture
```

### Sprint 3 — Conformité complète
```
S3-01  Nouveau service               FiscalYearService.close_year()
S3-02  deps.py + routes             require_admin appliqué aux routes admin
S3-03  Tous endpoints               Isolation données par utilisateur
S3-04  auth.py                      /auth/refresh + révocation JWT (jti + blacklist)
S3-05  docs/legal/                  DPA_template.md nLPD
S3-06  compliance.py               mask_personal_data() avant AuditLog
S3-07  service.py                   Branchement TDFN dans _post_to_journal()
S3-08  Nouveau service               VATDeclarationService (AFC 318/100)
S3-09  iso20022_pain001.py          CreDtTm avec suffixe 'Z'
S3-10  iso20022_camt.py             Parser CreDtTm depuis XML
S3-11  core/security.py            Supprimer email du payload JWT
S3-12  middleware/security.py      HSTS conditionnel HTTPS seulement
```

---

## Fichiers clés modifiés cette session

| Fichier | Modification |
|---------|-------------|
| `backend/app/api/v1/endpoints/main.py` | +GET /journal, +GET+PATCH /contacts/{id} |
| `backend/app/schemas/__init__.py` | +ContactUpdate, +JournalPageResponse, +notes/terms dans InvoiceResponse |
| `backend/alembic/versions/0001_initial.py` | Nouveau — migration initiale complète |
| `frontend/src/pages/InvoiceDetailPage.tsx` | Nouveau — page détail facture |
| `frontend/src/components/ui/PDFPreview.tsx` | Nouveau — composant PDF inline |
| `frontend/src/router.tsx` | +route /invoices/:invoiceId |
| `frontend/src/types/index.ts` | +notes, +terms dans Invoice |
| `backend/tests/integration/test_api.py` | +TestInvoicesEndpoints, +TestJournalEndpoints, +TestContactsPatch |
| `.env.example` | Nouveau |
| `README.md` | Réécrit complet |
| `CHANGELOG.md` | Nouveau |

---

## Commandes utiles

```bash
# Démarrer le projet
cd C:/Users/Paul/ledgeralps_final/ledgeralps
make up && make migrate && make seed

# Lancer les tests
make test

# Pousser sur GitHub
git add . && git commit -m "..." && git push

# Créer un nouveau tag de version
git tag -a vX.Y.Z -m "..." && git push origin vX.Y.Z
gh release create vX.Y.Z --title "..." --notes "..."
```
