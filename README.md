# LedgerAlps

**Comptabilité et facturation locale pour les PME et indépendants suisses.**

LedgerAlps est une application **local-first** — vos données restent sur votre machine ou votre serveur, sans cloud, sans abonnement. Conçu pour respecter le Code des obligations (CO), la nLPD, la QR-facture Six-Group et les standards ISO 20022.

---

## Stack

| Couche | Technologies |
|--------|-------------|
| Backend | Python 3.12 · FastAPI · SQLAlchemy async · PostgreSQL 16 · Alembic |
| Frontend | React 18 · TypeScript · Tailwind CSS · Vite · Zustand |
| Infrastructure | Docker · Docker Compose · Nginx |

---

## Fonctionnalités

- **Facturation** — Devis, factures, notes de crédit avec numérotation séquentielle CO
- **Comptabilité en partie double** — Journal, Grand Livre, Balance de vérification
- **QR-facture** — Génération native selon le standard Six-Group / STUZZA (SPC 0200)
- **TVA suisse** — Méthode effective et TDFN, taux 2024 (8.1% / 2.6% / 3.8%), arrondi à 0.05 CHF
- **ISO 20022** — Export pain.001 (virements), import camt.053/054 (relevés bancaires)
- **Audit immuable** — Chaque écriture validée est protégée par hash SHA-256 chaîné (CO art. 957a)
- **Export légal** — Archive ZIP annuelle avec manifest d'intégrité (conservation 10 ans, CO art. 958f)
- **Auth JWT** — Gestion des utilisateurs, rate limiting, headers de sécurité

---

## Prérequis

- [Docker](https://www.docker.com/get-started) ≥ 24
- [Docker Compose](https://docs.docker.com/compose/) ≥ 2.20
- Git

---

## Installation rapide

```bash
# 1. Cloner le dépôt
git clone https://github.com/kmdn-ch/LedgerAlps.git
cd LedgerAlps

# 2. Créer le fichier de configuration
cp .env.example .env
# (optionnel) Editer .env pour personnaliser les mots de passe

# 3. Démarrer tous les services
make up
```

Accès après démarrage :

| Service | URL |
|---------|-----|
| Frontend | http://localhost:5173 |
| API | http://localhost:8000 |
| Docs API (mode debug) | http://localhost:8000/api/docs |

---

## Configuration

Le fichier `.env` (copié depuis `.env.example`) contrôle l'ensemble de l'application :

```bash
# Base de données
POSTGRES_USER=ledgeralps
POSTGRES_PASSWORD=changeme         # ← à changer en production
POSTGRES_DB=ledgeralps

# JWT — générer avec : python -c "import secrets; print(secrets.token_hex(32))"
SECRET_KEY=changeme_in_production_use_32_chars_minimum

# Environnement
DEBUG=true                         # false en production
LOG_LEVEL=INFO
```

---

## Commandes `make`

```bash
make up           # Démarrer PostgreSQL + backend + frontend (avec rebuild)
make down         # Arrêter tous les services
make migrate      # Appliquer les migrations Alembic
make seed         # Charger le plan comptable PME suisse initial
make test         # Lancer les tests (pytest + couverture)
make lint         # Vérifier le code (ruff + mypy)
make logs         # Suivre les logs en temps réel
make shell-db     # Shell psql
make shell-backend # Shell bash dans le backend
```

---

## Initialisation de la base de données

```bash
# 1. Appliquer les migrations (crée toutes les tables)
make migrate

# 2. Charger le plan comptable PME suisse (à faire une seule fois)
make seed

# 3. Créer le premier compte administrateur
docker compose exec backend python -c "
import asyncio
from app.db.session import AsyncSessionLocal
from app.models import User
from app.core.security import hash_password

async def create_admin():
    async with AsyncSessionLocal() as db:
        admin = User(
            email='admin@votre-entreprise.ch',
            name='Administrateur',
            password_hash=hash_password('VotreMotDePasseFort!'),
            is_admin=True,
        )
        db.add(admin)
        await db.commit()
        print('Admin créé :', admin.email)

asyncio.run(create_admin())
"
```

---

## Installation en production (avec Nginx + TLS)

```bash
# 1. Configurer .env pour la production
cp .env.example .env
# Définir : DEBUG=false, SECRET_KEY robuste, POSTGRES_PASSWORD fort

# 2. Générer les certificats TLS (auto-signé pour intranet)
./scripts/generate-certs.sh

# 3. Démarrer avec le profil production (inclut Nginx)
docker compose --profile production up -d --build

# 4. Vérifier
curl -k https://ledgeralps.local/health
```

Pour Let's Encrypt :
```bash
certbot certonly --standalone -d ledgeralps.votre-domaine.ch
# Puis référencer les chemins dans docker/nginx/nginx.conf
```

---

## Tests

```bash
# Dans le conteneur backend
make test

# Ou directement
docker compose exec backend pytest -v --cov=app tests/
```

Les tests d'intégration requièrent une base PostgreSQL accessible. Le fichier `tests/integration/test_api.py` utilise automatiquement une base `ledgeralps_test`.

---

## Structure du projet

```
LedgerAlps/
├── backend/
│   ├── app/
│   │   ├── api/v1/endpoints/   # Routes FastAPI (auth, comptes, journal, factures…)
│   │   ├── core/               # Config, sécurité JWT, conformité CO
│   │   ├── db/                 # Session async, base SQLAlchemy, seeds
│   │   ├── models/             # Modèles (Account, JournalEntry, Invoice…)
│   │   ├── schemas/            # Schémas Pydantic v2
│   │   └── services/
│   │       ├── accounting/     # Moteur comptable (partie double)
│   │       ├── invoicing/      # Facturation + PDF
│   │       ├── vat/            # Calcul TVA suisse
│   │       └── swiss_standards/ # QR-facture, ISO 20022
│   ├── alembic/                # Migrations
│   └── tests/                  # Tests unitaires et d'intégration
├── frontend/
│   └── src/
│       ├── pages/              # Dashboard, Factures, Journal, Contacts…
│       ├── components/         # UI réutilisables, PDFPreview inline
│       ├── api/                # Client Axios centralisé
│       └── store/              # Auth store (Zustand)
├── docker/                     # Config PostgreSQL et Nginx
├── docs/                       # Architecture, checklist production
├── .env.example
├── docker-compose.yml
└── Makefile
```

---

## Conformité légale suisse

| Norme | Implémentation |
|-------|---------------|
| CO art. 957 — Comptabilité | Partie double, sum(débit) = sum(crédit) vérifié |
| CO art. 957a — Immuabilité | Écritures postées protégées par hash SHA-256 chaîné |
| CO art. 958f — Conservation 10 ans | Archive ZIP annuelle avec manifest d'intégrité |
| nLPD — Local-first | Aucun cloud, données sur votre infrastructure |
| TVA CH 2024 | Taux 8.1% / 2.6% / 3.8%, arrondi 0.05 CHF |
| QR-facture Six-Group SPC 0200 | Génération native, référence QRR/RF |
| ISO 20022 pain.001.001.09 | Ordres de virement compatibles banques CH |
| ISO 20022 camt.053.001.08 | Import relevés bancaires |

> **Avertissement** : LedgerAlps est conçu pour faciliter la conformité légale suisse, mais ne remplace pas la validation par un expert fiduciaire agréé.

---

## Contribution

Les contributions sont bienvenues. Merci de vous assurer que :
- Les écritures comptables respectent la partie double
- Les modifications de modèles sont accompagnées d'une migration Alembic
- Les nouveaux endpoints sont couverts par des tests d'intégration

---

## Licence

MIT — voir le fichier `LICENSE`.
