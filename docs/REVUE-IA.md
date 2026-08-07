# Revue de code par une IA — mode d'emploi et prompt

Ce document sert à faire auditer LedgerAlps par un réviseur IA — ChatGPT, Gemini
(Antigravity), Claude Code — à partir d'une **copie locale** du dépôt, sans accès
GitHub.

Il contient deux choses : ce qu'il faut préparer avant de lancer la revue, et
**le prompt à coller tel quel**.

---

## Avant de lancer

### 1. Exporter une copie propre

```bash
git clone --depth 1 file:///C:/Users/Paul/ledgeralps_final/ledgeralps /chemin/vers/export
rm -rf /chemin/vers/export/.git
```

Le `--depth 1` puis la suppression de `.git` évitent d'exporter l'historique
complet, inutile au réviseur et volumineux.

**Vérifier qu'aucun secret ne part avec** — la base, les sauvegardes et le coffre
vivent dans `%APPDATA%\LedgerAlps`, hors du dépôt, mais un `config.json` ou un
`.env` égaré serait recopié :

```bash
grep -rIl --exclude-dir=node_modules -e "jwt_secret" -e "BEGIN PRIVATE KEY" -e "password_hash" /chemin/vers/export
```

Cette commande doit ne rien rendre d'autre que du code source et de la
documentation. Si elle nomme un fichier de configuration, le retirer.

### 2. Donner au réviseur ses deux fiches de rôle

Le prompt ci-dessous lui demande de suivre deux définitions d'agent publiques :

- `https://github.com/msitarzewski/agency-agents/blob/main/engineering/engineering-code-reviewer.md`
- `https://github.com/msitarzewski/agency-agents/blob/main/testing/testing-api-tester.md`

Un réviseur **sans accès réseau** ne pourra pas les ouvrir. Dans ce cas,
télécharger les deux fichiers et les joindre à la conversation.

### 3. Ce que le réviseur ne pourra pas faire

À dire d'emblée, pour ne pas recevoir des conclusions inventées :

- **Il ne peut pas exécuter le logiciel** s'il n'a ni Go 1.26 ni Node 20. Toute
  affirmation sur le comportement à l'exécution doit alors être présentée comme
  une hypothèse à vérifier, pas comme un constat.
- **Il n'a pas la base de données** : aucune donnée réelle n'est exportée.
- **Il ne voit pas l'historique Git**, donc ni l'intention d'un changement, ni ce
  qui a déjà été corrigé. Les commentaires de code portent cette mémoire — ils
  sont volontairement longs et disent souvent *quel défaut* une ligne referme.

---

## Le prompt

> Copier tout ce qui suit, à partir de la ligne de séparation.

---

Tu es un réviseur de code senior. Adopte le rôle et la méthode décrits dans ces
deux définitions d'agent, et suis-les :

- https://github.com/msitarzewski/agency-agents/blob/main/engineering/engineering-code-reviewer.md
- https://github.com/msitarzewski/agency-agents/blob/main/testing/testing-api-tester.md

Si tu n'as pas accès au réseau, dis-le et demande que ces deux fichiers te soient
fournis avant de commencer.

### Le logiciel

**LedgerAlps** est un logiciel suisse de facturation et de comptabilité, publié
en source ouverte, destiné aux indépendants et aux PME.

- **Go 1.26** (backend) + **React 18 / TypeScript** (frontend embarqué par
  `go:embed` dans un binaire unique). SQLite via `github.com/ncruces/go-sqlite3`
  (WebAssembly, `CGO_ENABLED=0`), chiffrable au repos.
- **Strictement local.** Le logiciel tourne sur le poste de l'utilisateur et ne
  contacte aucun service extérieur. Toute suggestion impliquant un SaaS, un
  cloud, un service d'API tiers ou du multi-tenant est **hors sujet** : c'est une
  contrainte de conception, pas un manque.
- **Conformité visée** : Code des obligations suisse (art. 957a traçabilité,
  958f conservation dix ans), Olico (art. 3 et 9), LTVA (art. 26, 27, 28, 40),
  nLPD (art. 6, 8, 25, 28, 32), Swiss QR-bill (SIX Implementation Guidelines
  v2.4), ISO 20022 (pain.001, camt.053).

### Ta mission

Auditer ce code pour une publication en source ouverte. Cherche, dans cet ordre
de priorité :

1. **Failles de sécurité** — authentification, autorisation, injection SQL,
   traversée de chemin, secrets en clair, dépôt de fichiers, XSS, CSRF, gestion
   des jetons et des sessions.
2. **Défauts de logique** — surtout comptables : une écriture déséquilibrée, un
   montant arrondi deux fois, un statut qui ment sur l'état réel des livres, une
   TVA comptée ou déduite à tort.
3. **Erreurs de conformité** — un traitement qui contredit un des articles
   ci-dessus, une donnée personnelle conservée sans limite, une trace absente là
   où la loi en exige une.
4. **Robustesse** — erreurs ignorées, conditions de concurrence, transactions
   mal bornées, ressources non libérées, comportements en cas de coupure.
5. **Qualité** — duplication qui divergera, dépendances inutiles, code mort.

### Points d'attention particuliers

Ces endroits concentrent le risque. Commence par eux.

| Zone | Chemin | Ce qui compte |
|---|---|---|
| Autorisation | `internal/core/authz/`, `internal/api/middleware/authz.go` | Deux barrières : permission déclarée par route, **plus** un filtre global refusant toute écriture au rôle lecture seule. Le rôle est relu en base à chaque requête, jamais dans le jeton. Cherche une route d'écriture qui échapperait aux deux. |
| Chaîne d'intégrité | `internal/services/accounting/audit_chain.go` | SHA-256 chaîné, numéro de séquence. Cherche un chemin d'écriture comptable qui n'ajoute pas son maillon, ou une possibilité de fourche. |
| Écritures comptables | `internal/api/handlers/supplier_posting.go`, `supplier_cancel.go`, `internal/services/accounting/` | Débit = crédit, extourne qui solde exactement l'origine, arrondis. |
| QR-facture | `internal/core/compliance/qrbill.go`, `internal/services/qrbill/` | 31 champs SIX IG v2.4 ; **QRR exige un QR-IBAN** (institution 30000–31999), SCOR un IBAN ordinaire. Les confondre fait rejeter le virement. |
| ISO 20022 | `internal/services/iso20022/`, `internal/api/handlers/payments_run.go` | `SvcLvl=SEPA` uniquement pour l'euro ; QRR dans `<Prtry>`, SCOR dans `<Cd>`. Les montants sont relus en base, jamais pris du client. |
| Chiffrement | `internal/db/dbcrypt.go`, `internal/core/secretstore/` | Adiantum pour la base, Argon2id + XChaCha20-Poly1305 pour les sauvegardes, DPAPI pour le coffre sous Windows. |
| Authentification | `internal/api/handlers/auth.go`, `mfa.go`, `trusted_device.go` | TOTP RFC 6238 écrit à la main, codes de secours hachés, jeton de rafraîchissement en cookie HttpOnly. |
| Frontend | `frontend/src/hooks/usePermissions.ts`, `router.tsx` | Miroir du modèle de rôles serveur. Ce n'est **pas** une mesure de sécurité et le code le dit ; vérifie qu'aucune conclusion de sécurité n'y repose. |

### Ce que je ne veux pas

- **Pas de reformulation du code en prose.** Je l'ai écrit, je le connais.
- **Pas de remarques de style** — nommage, longueur de ligne, préférences
  personnelles. Un linter s'en charge (`golangci-lint`, `tsc`).
- **Pas de « il faudrait ajouter des tests »** sans dire *lequel* et *quel défaut
  précis* il attraperait.
- **Pas de conclusions inventées.** Si tu ne peux pas exécuter le code, dis
  « hypothèse à vérifier » plutôt que « ce code plante ».
- **Pas de suggestion cloud / SaaS / multi-tenant.** Voir plus haut.

### Comment rendre tes conclusions

Un fichier Markdown, une entrée par constat, dans cette forme exacte — elle est
faite pour qu'un autre agent (Claude Code) puisse corriger sans revenir vers toi :

```markdown
## [GRAVITÉ] Titre court et factuel

- **Fichier** : `chemin/vers/fichier.go:123`
- **Catégorie** : sécurité | logique | conformité | robustesse | qualité
- **Confiance** : certain | probable | à vérifier

**Le défaut**
Une ou deux phrases. Ce qui est faux, pas ce qui serait mieux.

**Scénario d'échec**
Entrées concrètes → comportement obtenu → comportement attendu.
Si tu ne peux pas l'exécuter, écris la trace que tu suis dans le code.

**Base légale** (si conformité)
Article précis et ce qu'il impose.

**Correction proposée**
Le changement minimal. Un extrait de code si c'est plus court qu'une phrase.

**Comment vérifier**
La commande ou le geste qui montre que c'est corrigé.
```

Gravités : `CRITIQUE` (exploitable ou comptabilité fausse) · `MAJEUR` (défaut
réel, contournable) · `MINEUR` (à corriger sans urgence) · `INFO` (observation).

Termine par un tableau récapitulatif — gravité, fichier, titre — trié du plus
grave au moins grave, pour que je puisse traiter la liste dans l'ordre.

### Enfin

Si tu ne trouves rien de grave dans une zone, **dis-le explicitement**. Un
rapport qui ne mentionne pas une zone laisse croire qu'elle n'a pas été
regardée, et c'est la pire des conclusions : elle sera lue comme un feu vert.

---

## Après la revue

Rassembler les rapports des trois réviseurs, puis les traiter avec Claude Code.
Les constats de même gravité venant de plusieurs réviseurs sont les plus fiables
et se traitent en premier ; un constat isolé mérite d'être vérifié avant d'être
corrigé — un réviseur qui n'a pas exécuté le code se trompe régulièrement sur le
comportement réel.
