# LedgerAlps — Architecture Technique

## Structure du projet

```
ledgeralps/
├── backend/
│   ├── app/
│   │   ├── api/v1/endpoints/    # Routes FastAPI (Phase 2)
│   │   ├── core/
│   │   │   ├── config.py        # Configuration (pydantic-settings)
│   │   │   ├── compliance.py    # Règles CO / nLPD / TVA
│   │   │   └── security.py      # JWT, hashing (Phase 2)
│   │   ├── db/
│   │   │   ├── base.py          # Base SQLAlchemy + mixins audit
│   │   │   ├── session.py       # Engine async + dépendance FastAPI
│   │   │   └── seeds/           # Données initiales (plan comptable)
│   │   ├── models/              # Modèles SQLAlchemy
│   │   ├── schemas/             # Schémas Pydantic (Phase 2)
│   │   ├── services/
│   │   │   ├── accounting/      # Moteur de comptabilité (Phase 2)
│   │   │   ├── invoicing/       # Facturation + PDF (Phase 2)
│   │   │   ├── vat/             # Calcul TVA CH (Phase 2)
│   │   │   └── swiss_standards/ # QR-facture, ISO 20022 (Phase 3)
│   │   └── main.py              # Entry point FastAPI
│   ├── alembic/                 # Migrations
│   ├── tests/
│   └── pyproject.toml
├── frontend/                    # React + TypeScript (Phase 4)
├── docker/
│   ├── postgres/init.sql
│   └── nginx/
├── docs/
│   ├── legal/                   # Références CO, nLPD
│   └── api/                     # Documentation OpenAPI
├── .env.example
├── docker-compose.yml
└── Makefile
```

## Phases de développement

| Phase | Agents | Statut |
|-------|--------|--------|
| 1 — Foundation | Architect, Schema, Compliance, DevOps | ✅ Complète |
| 2 — Core Backend | Accounting, Invoice, VAT, API | ⏳ Suivante |
| 3 — Swiss Standards | QR-Invoice, ISO 20022, Export, Test | — |
| 4 — Frontend | UI, Forms, Reports, Auth | — |
| 5 — Production | Security, CI/CD, Docs, QA | — |

## Démarrage rapide

```bash
make up       # Lance PostgreSQL + backend + frontend
make migrate  # Crée les tables
make seed     # Charge le plan comptable PME suisse
```

## Conformité légale

- **CO art. 957–963** : Comptabilité en partie double, traçabilité, conservation 10 ans
- **nLPD** : Privacy by Design, chiffrement, données minimales
- **ISO 20022** : pain.001 (virements), camt.053/054 (relevés)
- **QR-facture** : Standard Six-Group / STUZZA
- **TVA** : Méthode effective + TDFN, taux 2024 (8.1% / 2.6% / 3.8%)
