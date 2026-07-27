# Architecture

LedgerAlps est un **binaire unique**. Le serveur HTTP, le moteur comptable, la
génération PDF et l'interface React compilée vivent dans le même exécutable.
Aucun service à orchestrer, aucun conteneur, aucune dépendance externe à
installer sur la machine de l'utilisateur.

Ce choix découle du positionnement du produit : une PME suisse doit pouvoir
faire tourner sa comptabilité sur son propre poste, sans administrateur système
et sans cloud.

---

## Vue d'ensemble

```
┌──────────────────────────────────────────────────────────┐
│  ledgeralps.exe  (lanceur Windows, sans console)         │
│    démarre le serveur, ouvre le navigateur               │
└────────────────────────┬─────────────────────────────────┘
                         │
┌────────────────────────▼─────────────────────────────────┐
│  ledgeralps-server                                       │
│                                                          │
│   Gin (HTTP)  ──►  handlers  ──►  services  ──►  db      │
│                                                          │
│   Frontend React compilé, embarqué via go:embed          │
│   Migrations SQL embarquées, appliquées au démarrage     │
└────────────────────────┬─────────────────────────────────┘
                         │
              SQLite (WAL) par défaut
              PostgreSQL si POSTGRES_DSN est défini
```

## Organisation du code

| Chemin | Rôle |
|---|---|
| `cmd/server` | Point d'entrée du serveur : configuration, migrations, routes |
| `cmd/cli` | CLI d'administration (`migrate`, `bootstrap`, `health`, `backup`, `restore`) |
| `cmd/launcher` | Lanceur Windows (`-H=windowsgui`) : démarrage, assistant, navigateur |
| `internal/api/handlers` | Handlers HTTP — une famille de routes par fichier |
| `internal/api/middleware` | Authentification, CORS, en-têtes de sécurité, limitation de débit |
| `internal/core/compliance` | Règles suisses : QR-facture, IBAN, arrondi 5 centimes, avis de conformité |
| `internal/core/security` | Hachage de mots de passe, JWT, chaîne de hachage d'audit |
| `internal/services` | Logique métier : comptabilité, facturation, TVA, PDF, ISO 20022 |
| `internal/db` | Connexion, migrations embarquées, sauvegardes |
| `internal/models` | Structures de données partagées |
| `internal/frontend` | Frontend React compilé, embarqué dans le binaire |
| `compliance/` | Veille légale automatisée — voir [compliance/README.md](../compliance/README.md) |

## Décisions structurantes

**Migrations embarquées.** Les fichiers SQL sont compilés dans le binaire
(`embed.FS`) et appliqués au démarrage, une transaction par migration.
L'utilisateur n'a jamais à lancer d'outil de migration : mettre à jour
l'exécutable suffit.

**Frontend embarqué.** `go:embed` intègre `frontend/dist`. Il n'y a donc ni
serveur web à configurer, ni fichiers statiques à déployer séparément.

**SQLite par défaut, PostgreSQL possible.** Le mode WAL autorise des lectures
concurrentes avec un rédacteur sérialisé, ce qui suffit largement à une PME.
`db.Rebind` traduit les paramètres `?` en `$1` pour PostgreSQL : une seule
requête écrite couvre les deux moteurs.

**Chaîne de hachage d'audit.** Chaque écriture validée reçoit un SHA-256 chaîné
au précédent (`entry_hash`, `prev_hash`, `sequence_number`). Une modification
rétroactive casse la chaîne de façon détectable, comme l'exige le CO art. 957a.

**Partie double vérifiée à la validation.** `sum(débit) == sum(crédit)` est
contrôlé avant qu'une écriture ne passe au statut `posted` ; une écriture
déséquilibrée est refusée, pas corrigée silencieusement.

## Conformité — où regarder

| Domaine | Emplacement |
|---|---|
| QR-facture (SIX, SPC 0200) | `internal/core/compliance/qrbill.go` |
| TVA suisse, arrondi 0.05 | `internal/services/vat`, `internal/core/compliance/swiss.go` |
| ISO 20022 pain.001 / camt.053 | `internal/services/iso20022` |
| CO art. 957a — audit immuable | `internal/services/accounting` |
| CO art. 958f — archivage 10 ans | `internal/api/handlers/export.go`, `internal/db/backup.go` |
| nLPD — masquage, journalisation | `internal/services/accounting`, `internal/api/handlers/security_events.go` |
| Veille légale automatisée | [`compliance/`](../compliance/README.md) |

## Documents liés

- [DEVELOPMENT.md](DEVELOPMENT.md) — compiler, tester, contribuer
- [API.md](API.md) — référence des endpoints
- [PRODUCTION.md](PRODUCTION.md) — déploiement serveur
- [BRANCHING.md](BRANCHING.md) — branches, validation, releases
