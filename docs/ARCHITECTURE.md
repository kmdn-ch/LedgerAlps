# Architecture

LedgerAlps est un **binaire unique**. Le serveur HTTP, le moteur comptable, la
génération PDF et l'interface React compilée vivent dans le même exécutable.
Aucun service à orchestrer, aucun conteneur, aucune dépendance externe à
installer sur la machine de l'utilisateur.

Ce choix découle du positionnement du produit : une PME suisse doit pouvoir
faire tourner sa comptabilité sur son propre poste, sans administrateur système
et sans cloud.

---

## Périmètre du produit

LedgerAlps est un logiciel de **facturation** qui tient derrière lui la
comptabilité que la loi suisse exige d'un indépendant qui facture. Ce n'est pas
une solution comptable complète.

Le critère d'admission d'une fonctionnalité tient en une question : *sans elle,
une facture suisse est-elle non conforme, ou inexploitable ?* Le verrouillage de
période, la chaîne d'intégrité et la déclaration TVA passent ce test ; une
comptabilité analytique ou une gestion de stock ne le passent pas.

La direction d'évolution retenue est **verticale** — des modules métier qui
ajoutent des façons de composer une facture sans toucher au noyau comptable.
Voir [ROADMAP.md](../ROADMAP.md#14--modules-métier).


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
`GET /audit-logs/verify-chain` parcourt l'ensemble et distingue quatre ruptures ;
l'écran **Paramètres → Maintenance → Piste d'audit** l'expose.

*Règle apprise à ses dépens :* l'empreinte doit couvrir **exactement les valeurs
enregistrées**. Jusqu'à la v1.4.6 elle était calculée sur un `after_state` rédigé
séparément de celui inséré et sur un horodatage venu de Go alors que la colonne
était remplie par le `DEFAULT CURRENT_TIMESTAMP` de SQLite : aucune écriture ne
pouvait se revérifier. Le défaut a traversé toute la suite de tests parce que
chaque test fabriquait ses propres lignes avec les mêmes valeurs des deux côtés ;
il est apparu au premier appel réel. `internal/services/accounting/integrity_test.go`
écrit désormais par le vrai chemin puis relit depuis la base — recalculer en
mémoire ce qu'on vient de calculer en mémoire ne prouve rien.

**Exercice comptable et verrouillage de période.** Chaque écriture est rattachée
à l'exercice couvrant sa date, dans la transaction qui l'insère ; sans exercice
couvrant, l'année civile est créée. Un exercice clos refuse création **et**
comptabilisation (CO art. 958f, Olico art. 3) — le second contrôle porte le
chemin qui compte, un brouillon créé avant la clôture et comptabilisé après.
L'écriture de clôture passe par le même ajout au chaînage que les autres :
`accounting.AppendAuditEntry`, dont la lecture du maillon précédent se fait
**dans** la transaction, faute de quoi deux comptabilisations concurrentes
fourcheraient la chaîne.

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
