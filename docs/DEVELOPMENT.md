# Développement

## Prérequis

| Outil | Version | Pour |
|---|---|---|
| Go | 1.26+ (voir `go.mod`) | Serveur, CLI, lanceur |
| Node.js | 20+ | Frontend React |
| Python | 3.12+ (bibliothèque standard seule) | Veille de conformité |
| NSIS | 3.x | Installeur Windows — **optionnel**, la CI s'en charge |

## Démarrage rapide

```bash
git clone https://github.com/kmdn-ch/LedgerAlps.git
cd LedgerAlps

export JWT_SECRET=$(openssl rand -hex 32)   # < 32 caractères : le serveur refuse de démarrer
make build-frontend                          # compile React dans frontend/dist
make run                                     # serveur sur http://localhost:8000
```

Puis, dans un autre terminal, créer le premier administrateur :

```bash
go run ./cmd/cli bootstrap --email=dev@local.ch --password='motdepasse-solide'
```

### Frontend en rechargement à chaud

```bash
cd frontend && npm install && npm run dev    # Vite sur http://localhost:5173
```

Le frontend appelle l'API sur le port 8000 ; `ALLOWED_ORIGINS` inclut déjà
`http://localhost:5173` par défaut.

## Cibles Make

```bash
make help              # liste toutes les cibles
make build             # serveur + CLI pour l'OS courant
make build-windows     # serveur + lanceur Windows (sans console)
make build-frontend    # frontend de production
make test              # tous les tests, détecteur de concurrence actif
make test-coverage     # tests + rapport HTML de couverture
make lint fmt vet      # qualité de code
```

## Tests

```bash
go test ./...                              # tout
go test -race ./...                        # comme la CI
go test ./internal/core/compliance/... -v  # un paquet
```

`go test -race` exige cgo et donc un compilateur C. Sous Windows sans gcc,
lancez `go test ./...` en local : la CI exécute le détecteur de concurrence
sous Linux à chaque push.

Les tests d'intégration créent une base SQLite temporaire par test — ils ne
touchent jamais votre base réelle.

## Base de données

Les migrations vivent dans `internal/db/migrations/`, sont embarquées dans le
binaire et s'appliquent au démarrage, une transaction chacune. Pour en ajouter
une, créez `NNNN_description.up.sql` : la numérotation détermine l'ordre.

⚠️ **Piège de configuration.** `config.Load()` lit `config.json` dans le
répertoire de données applicatives **avant** les variables d'environnement.
Définir `SQLITE_PATH` est donc sans effet si ce fichier existe — et une commande
destinée à une base de test peut atteindre la base réelle. Pour les commandes
CLI, utilisez toujours l'option explicite :

```bash
go run ./cmd/cli backup  --sqlite-path=/tmp/essai.db --dir=/tmp/backups
go run ./cmd/cli restore --file=… --sqlite-path=/tmp/essai.db --confirm
```

`restore` refuse de s'exécuter sans `--confirm` et affiche la base qu'il
écraserait.

## Installeur Windows

Construit par la CI. Pour le reproduire localement, NSIS 3.x est nécessaire :

```bash
make build-windows && make build-frontend
cd infrastructure/windows
makensis /DVERSION=1.3.15 installer.nsi
```

⚠️ **`installer.nsi` doit conserver son BOM UTF-8.** NSIS ne lit un script en
UTF-8 que si le BOM est présent ; sans lui il retombe sur la codepage ANSI du
système et les accents français sont corrompus (« donnÃ©es » au lieu de
« données »). Certains éditeurs suppriment le BOM en silence. La régression est
déjà arrivée en production : le workflow « Release (dry run) » vérifie
désormais sa présence à chaque répétition.

## Veille de conformité

```bash
python scripts/compliance_watch.py            # vérifier les sources
python scripts/compliance_watch.py --update   # enregistrer les empreintes
```

Bibliothèque standard uniquement, par choix. Voir
[compliance/README.md](../compliance/README.md) — en particulier la règle
selon laquelle un avis de conformité est **toujours écrit par un humain**, avec
citation de la source.

## Branches et releases

Voir [BRANCHING.md](BRANCHING.md). En résumé : `feat/*` → `test` → `main`, en
avance rapide, puis tag. Pour essayer une release sans rien publier :

```bash
gh workflow run "Release (dry run)" --ref test
```

## Documents liés

- [ARCHITECTURE.md](ARCHITECTURE.md) — structure du code et décisions
- [API.md](API.md) — référence des endpoints
- [PRODUCTION.md](PRODUCTION.md) — déploiement serveur
