# LedgerAlps — Checklist de mise en production

## 1. Sécurité

### Variables d'environnement obligatoires
```bash
# Générer une clé secrète robuste (minimum 32 caractères)
python -c "import secrets; print(secrets.token_hex(32))"

# .env production
SECRET_KEY=<clé générée ci-dessus>
POSTGRES_PASSWORD=<mot de passe fort — min 20 chars>
DEBUG=false
LOG_LEVEL=WARNING
```

### Certificats TLS
```bash
# Auto-signé pour local/intranet
./scripts/generate-certs.sh

# Let's Encrypt pour accès internet
certbot certonly --standalone -d ledgeralps.votre-domaine.ch
# Puis référencer les chemins dans docker/nginx/nginx.conf
```

### Vérifications sécurité
- [ ] `SECRET_KEY` ≥ 32 caractères, aléatoire
- [ ] `DEBUG=false` en production
- [ ] `POSTGRES_PASSWORD` robuste, unique
- [ ] TLS actif (HTTPS)
- [ ] Ports 8000 et 5432 non exposés publiquement (nginx en façade)
- [ ] Backup automatique PostgreSQL configuré

---

## 2. Base de données

```bash
# 1. Lancer PostgreSQL
docker compose up -d db

# 2. Appliquer les migrations
make migrate

# 3. Charger le plan comptable initial (une seule fois)
make seed

# 4. Créer le premier utilisateur administrateur
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
            password_hash=hash_password('CHANGER_CE_MOT_DE_PASSE'),
            is_admin=True,
        )
        db.add(admin)
        await db.commit()

asyncio.run(create_admin())
"
```

---

## 3. Déploiement complet

```bash
# Cloner et configurer
git clone https://github.com/votre-repo/ledgeralps.git
cd ledgeralps
cp .env.example .env
# Éditer .env avec les valeurs de production

# Générer les certificats
./scripts/generate-certs.sh

# Lancer en production (avec nginx)
docker compose --profile production up -d --build

# Vérifier les services
docker compose ps
curl -k https://ledgeralps.local/health
```

---

## 4. Sauvegardes (CO art. 958f — 10 ans)

### Backup PostgreSQL automatique
```bash
# Ajouter dans crontab (chaque nuit à 2h)
0 2 * * * docker exec ledgeralps_db pg_dump -U ledgeralps ledgeralps | \
  gzip > /backups/ledgeralps_$(date +%Y%m%d).sql.gz

# Vérifier l'intégrité
gunzip -t /backups/ledgeralps_$(date +%Y%m%d).sql.gz && echo "OK"
```

### Archive légale annuelle
1. Dans LedgerAlps → Rapports → Archive légale
2. Sélectionner l'exercice clôturé
3. Télécharger le ZIP (journal + Grand Livre + balance + manifest SHA-256)
4. Stocker sur support immuable (NAS, stockage chiffré)
5. **Durée minimale : 10 ans** (CO art. 958f)

---

## 5. Monitoring

### Healthcheck Docker
```yaml
# Déjà configuré dans docker-compose.yml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8000/health"]
  interval: 30s
  timeout: 10s
  retries: 3
```

### Logs
```bash
# Suivre les logs en production
docker compose logs -f backend | grep -E "ERROR|WARNING|Auth failure"

# Logs nginx (accès)
docker compose exec nginx tail -f /var/log/nginx/access.log
```

---

## 6. Mise à jour

```bash
# Tirer les nouvelles images
git pull
docker compose pull

# Appliquer les migrations AVANT de redémarrer
docker compose run --rm backend alembic upgrade head

# Redémarrer sans interruption
docker compose up -d --build --no-deps backend frontend
```

---

## 7. Conformité légale suisse

| Obligation | Implémentation | Statut |
|------------|----------------|--------|
| CO art. 957 — Comptabilité | Partie double vérifiée | ✅ |
| CO art. 957a — Immuabilité | Hash SHA-256 + statut POSTED | ✅ |
| CO art. 958f — Conservation 10 ans | Archive ZIP + manifest | ✅ |
| nLPD — Local-first | Pas de cloud, données locales | ✅ |
| nLPD — Chiffrement | pgcrypto activé | ✅ |
| TVA CH 2024 — Taux | 8.1% / 2.6% / 3.8% | ✅ |
| TVA — Arrondi 0.05 CHF | Implémenté | ✅ |
| QR-facture — Six-Group SPC 0200 | Validé, tests OK | ✅ |
| ISO 20022 — pain.001.001.09 | Compatible banques CH | ✅ |
| ISO 20022 — camt.053.001.08 | Parser avec réconciliation | ✅ |

**Avertissement** : Ce logiciel est conçu pour respecter les normes suisses.
L'utilisateur reste responsable de la validation fiduciaire et des déclarations fiscales.
