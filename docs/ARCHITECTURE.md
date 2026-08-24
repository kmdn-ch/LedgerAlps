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
| `internal/api/middleware` | Authentification, autorisation par rôle, CORS, en-têtes de sécurité, limitation de débit |
| `internal/core/compliance` | Règles suisses : QR-facture, IBAN, arrondi 5 centimes, avis de conformité |
| `internal/core/security` | Hachage de mots de passe, JWT, chaîne de hachage d'audit |
| `internal/core/authz` | Rôles et permissions — table de droits, refus par défaut |
| `internal/core/mfa` | Second facteur TOTP (RFC 6238), vérifié contre les vecteurs officiels |
| `internal/services` | Logique métier : comptabilité, facturation, TVA, PDF, ISO 20022 |
| `internal/db` | Connexion, migrations embarquées, sauvegardes |
| `internal/models` | Structures de données partagées |
| `internal/frontend` | Frontend React compilé, embarqué dans le binaire |
| `compliance/` | Veille légale automatisée — voir [compliance/README.md](../compliance/README.md) |

## Décisions structurantes

**Les droits se lisent dans la base, jamais dans le jeton.** Un jeton d'accès vit
une heure ; si le rôle y était inscrit, rétrograder ou désactiver quelqu'un le
laisserait agir avec ses anciens droits pendant tout ce temps. La base est locale
et la lecture est un accès par clé primaire : le coût est nul. Le jeton ne prouve
donc que l'identité — ce qu'un compte a le droit de faire se demande à la base,
au moment où il le fait.

**Ce qui part vers l'extérieur est relu dans les livres, jamais reçu du client.**
L'ordre de paiement ne transmet que des identifiants de factures ; le créancier,
l'IBAN, le montant et la référence sont relus côté serveur. Accepter des montants
depuis le navigateur reviendrait à laisser une page web dicter ce qui part à la
banque — un script injecté, une extension bavarde ou une erreur d'arrondi côté
interface suffiraient. La règle vaut pour toute sortie de valeur : le client
désigne, le serveur décide.

**Les refus vivent au point de passage obligé, pas route par route.** Trois
filtres globaux s'appliquent à tout le groupe authentifié : écriture interdite
aux rôles en lecture seule, mot de passe temporaire à remplacer, second facteur à
inscrire pour les administrateurs. Un quatrième refus vit dans le filtre
d'authentification lui-même : un jeton d'attente de second facteur n'ouvre aucune
route. Une garde qu'il faut penser à déclarer s'oublie une fois — et cette
fois-là ouvre un trou que rien ne signale. Les routes qui n'existent pas encore
sont couvertes.

Les seules routes montées **hors** du groupe filtré sont celles qui permettent de
sortir d'un de ces états : changement de mot de passe, inscription et
vérification du second facteur. Les y inclure enfermerait le compte
définitivement.

**Cinq permissions, deux frontières.** `read`, `write_documents`,
`write_accounting`, `manage`, `admin`. La frontière qui compte est entre les deux
dernières : **`manage` administre la comptabilité** — clôture d'exercice, contrôle
d'intégrité, sauvegardes, fiche entreprise, effacement nLPD — et **`admin`
administre le logiciel et qui y accède** : chiffrement, restauration, réseau, clé
de signature, journal de sécurité, comptes. Le comptable a la première, pas la
seconde. Le détail est dans [DROITS.md](DROITS.md).

**Aucune garde ne lit le jeton.** Neuf handlers vérifiaient encore `IsAdmin` dans
les claims — un drapeau figé à la connexion, donc encore valide une heure après
une rétrogradation. Ils ont été retirés au profit de la permission déclarée sur
la route, lue dans la base à chaque requête. Deux tests lisent le fichier des
routes et vérifient que la permission y figure : sans eux, retirer le middleware
laisserait le handler sans protection, et rien ne le signalerait.

**Toute route d'écriture déclare sa permission**, même lorsque le filtre global
la couvrirait déjà. Les deux barrières attrapent des erreurs différentes : le
filtre couvre la route qu'on oublie d'annoter, la permission déclarée couvre le
rôle qui écrit sans en avoir le droit sur une route précise. Une fonction
ajoutée plus tard doit donc rester inaccessible à un rôle en lecture seule sans
que personne ait à y penser.

**Migrations embarquées.** Les fichiers SQL sont compilés dans le binaire
(`embed.FS`) et appliqués au démarrage, une transaction par migration.
L'utilisateur n'a jamais à lancer d'outil de migration : mettre à jour
l'exécutable suffit.

**Frontend embarqué.** `go:embed` intègre `frontend/dist`. Il n'y a donc ni
serveur web à configurer, ni fichiers statiques à déployer séparément.

### Le pilote SQLite, et pourquoi il a changé

`github.com/ncruces/go-sqlite3` depuis la v1.4.8, `modernc.org/sqlite` avant.

Les deux sont en Go pur et compilent avec `CGO_ENABLED=0` — c'est non négociable,
c'est ce qui donne le binaire unique et la compilation croisée. Le changement
tient à un seul point : **le chiffrement au repos de la base**. Le paquet `vfs`
de modernc est en lecture seule, donc aucune couche chiffrante ne pouvait
s'insérer sous lui ; ncruces expose un VFS écrivable, et livre `vfs/adiantum`.

Ce que cela coûte, mesuré sur la charge réelle de l'application (2 000 requêtes
typiques) : **1,00 ms par requête contre 0,64 ms**, soit +0,36 ms — invisible.
La mémoire du processus passe de 17 à 81 Mo (SQLite tourne dans un bac à sable
WebAssembly), et le binaire grossit de 2,4 Mo. La suite de tests complète est
passée sans qu'une seule requête SQL soit modifiée.

Deux pièges rencontrés en migrant, tous deux invisibles à la compilation :

- **Les pragmas ne peuvent pas rester dans la DSN quand la base est chiffrée.**
  Ils s'exécutent avant le rappel qui fournit la clé, touchent le fichier, et
  échouent. La clé passe donc par un `PRAGMA` en tête du rappel d'initialisation
  de connexion — ce qui a l'avantage de la tenir hors de toute chaîne susceptible
  d'atterrir dans un journal.
- **`VACUUM INTO 'chemin'` échoue depuis une connexion chiffrée** : la cible
  hérite du VFS de la connexion, sans clé. C'est la ligne qui produit les
  sauvegardes ; sans le voir, la migration les aurait cassées en silence. La
  cible est désormais nommée `file:…?vfs=` — le VFS par défaut, explicitement.

**SQLite par défaut, PostgreSQL possible.** Le mode WAL autorise des lectures
concurrentes avec un rédacteur sérialisé, ce qui suffit largement à une PME.
`db.Rebind` traduit les paramètres `?` en `$1` pour PostgreSQL : une seule
requête écrite couvre les deux moteurs.

**Chaîne de hachage d'audit.** Chaque écriture validée reçoit un SHA-256 chaîné
au précédent (`entry_hash`, `prev_hash`, `sequence_number`). Une modification
rétroactive casse la chaîne de façon détectable, comme l'exige le CO art. 957a.
`GET /audit-logs/verify-chain` parcourt l'ensemble et distingue quatre ruptures ;
l'écran **Paramètres → Maintenance → Piste d'audit** l'expose.

**Audit différentiel — ce qu'une action a remplacé.** Le maillon portait l'état
APRÈS chaque action, jamais celui d'avant : `before_state` était systématiquement
nul. On savait qu'une facture valait 1500.- après modification, sans pouvoir dire
qu'elle valait 1000.- avant.

Les appelants transmettent désormais une `accounting.Transition` — `Creation`,
`Modification` ou `Suppression`. Des champs nommés plutôt que deux
`map[string]any` côte à côte : les intervertir produirait une piste qui affirme
le contraire de ce qui s'est passé, avec l'autorité d'une chaîne valide.

*Le masquage change la donne.* `maskPersonalData` remplace la valeur des champs
personnels (nLPD art. 6) : un IBAN modifié donnerait `[MASKED]` des deux côtés,
et le changement disparaîtrait — précisément le cas qui motive la
fonctionnalité, puisque cet IBAN est le compte qui reçoit les virements de tous
les clients. La liste des champs qui ont bougé est donc calculée sur les valeurs
**brutes, avant masquage**, et jointe à l'état suivant sous `champs_modifies`.
On sait que l'IBAN a changé, et qui l'a changé, **sans conserver aucun des deux
IBAN** — plus utile que deux valeurs masquées, et moins de données personnelles
retenues.

Cette liste entre dans l'empreinte : l'effacer pour cacher qu'un IBAN a bougé
casse la chaîne. Une trace des changements réinscriptible ne prouverait rien.

*Rétrocompatible sans migration.* Une création écrit `NULL`, la vérification
relit `COALESCE(before_state, '')`, et les maillons antérieurs se recalculent
donc à l'identique. Le masquage a par ailleurs été élargi aux variantes
composées (`company_name`, `address_street`, `supplier_name`…) : la règle
n'acceptait que les clés exactes et laissait en clair, chez un indépendant, son
propre nom et son adresse privée.

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
