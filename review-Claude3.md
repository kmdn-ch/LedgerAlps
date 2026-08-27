# Troisième audit — code & sécurité — LedgerAlps

**Dépôt** : `kmdn-ch/LedgerAlps` · **Branche** : `test` · **Commit** : `2d7e489` (v1.5.7)
**Date** : 27 août 2026
**Périmètre** : l'intégralité des fichiers suivis (359), hors `frontend/node_modules/` et `internal/frontend/dist/`.
**Portée d'écriture** : ce rapport est le **seul** fichier créé. Aucun autre fichier du dépôt n'a été modifié, créé ou supprimé — `git status` le confirme.

**Méthode.** Outillage automatique (`go vet`, `go test ./...`, `govulncheck`, `deadcode -test`, `npm audit`), lecture du code, et **preuve d'exécution sur un serveur réel** pour la constatation principale. Chaque affirmation de ce rapport est étiquetée `[prouvé]`, `[vérifié]` ou `[lecture]` — voir § 1.4.

Les deux audits précédents (v1.5.2 et v1.5.4) ont produit des plans d'action appliqués depuis. Ce passage a trois objets : **auditer les 2 100 lignes changées depuis la v1.5.4**, **couvrir enfin l'axe « code mort / logiques rompues »** que le second audit reconnaissait n'avoir traité qu'à moitié, et **rechercher les asymétries** — deux chemins qui font la même chose et ne la font pas pareil.

---

## 1. Résumé exécutif

### 1.1 Verdict

**Le socle est sain et l'outillage le confirme.** `go vet` muet, 604 tests au vert, `govulncheck` sans aucune vulnérabilité atteignable, zéro injection SQL, zéro sink XSS côté React, aucun secret codé en dur, les deux installeurs vérifient une empreinte SHA-256 **avant** d'extraire quoi que ce soit, et les scripts shell portent tous `set -euo pipefail`. Le durcissement des deux audits précédents tient.

**Une constatation est néanmoins matérielle, et elle a été prouvée à l'exécution** : l'extourne d'une facture **fournisseur** n'est pas marquée comme extourne, là où celle d'une facture **client** l'est. Ce n'est pas une faille de sécurité — c'est un défaut d'intégrité comptable sur une donnée qui part dans l'**archive légale** remise à la fiduciaire (CO art. 958f).

Le reste relève du durcissement et de l'hygiène : quatre points de sécurité de sévérité moyenne à basse, sept fonctions mortes confirmées par analyse d'atteignabilité, et une poignée d'écarts de bonnes pratiques.

### 1.2 Score de santé

| Axe | Note | Ce qui la fixe |
|---|---:|---|
| **Sécurité applicative** | 9,0 / 10 | Aucune injection, autorisation relue en base à chaque requête, cookies `HttpOnly` + `SameSite=Strict`, CSP stricte sans `unsafe-inline` sur les scripts, bcrypt coût 12 avec pré-hachage |
| **Sécurité de la chaîne d'assemblage** | 7,5 / 10 | Empreintes vérifiées avant extraction, actions GitHub épinglées par SHA — mais **archives non signées** et empreintes servies par le même canal |
| **Intégrité comptable** | 7,0 / 10 | Chaîne SHA-256, déclencheurs d'immuabilité, verrouillage de période — **entamée par l'extourne fournisseur non marquée** |
| **Robustesse / gestion d'erreur** | 8,0 / 10 | Contextes et délais partout, sauf sur le chemin d'autorisation, parcouru à chaque requête |
| **Code mort & cohérence** | 8,0 / 10 | 7 fonctions inatteignables, dont une **justifiée par un commentaire faux** |
| **Qualité par langage** | 8,5 / 10 | Go idiomatique, TypeScript typé (9 `any` seulement), scripts exemplaires |
| **Global** | **8,3 / 10** | |

### 1.3 Métriques

| | |
|---|---:|
| Go | 212 fichiers · 51 953 lignes |
| TypeScript / TSX | 71 fichiers · 20 048 lignes |
| SQL (migrations) | 28 fichiers · 973 lignes |
| Python · Shell · PowerShell · NSIS | 3 · 5 · 1 · 1 fichiers |
| Tests Go | 84 fichiers · **604 fonctions** — toutes au vert |
| Routes API | 114 |
| Clés d'interface (× 4 langues) | 1 212 |

**Constatations par sévérité**

| Sévérité | Sécurité | Logique / code mort | Total |
|---|---:|---:|---:|
| CRITIQUE | 0 | 0 | **0** |
| HAUTE | 0 | 1 | **1** |
| MOYENNE | 2 | 1 | **3** |
| BASSE | 2 | 4 | **6** |
| Informatif | — | — | **7** |

### 1.4 Ce qui a été prouvé, et ce qui ne l'est pas

Les deux premiers audits ont chacun produit au moins une affirmation fausse — le second l'a reconnu et corrigé. Pour éviter de répéter la faute, chaque constatation porte ici son niveau de preuve :

- `[prouvé]` — reproduit à l'exécution, sortie de commande à l'appui ;
- `[vérifié]` — établi par un outil déterministe (`deadcode`, `govulncheck`, `grep` exhaustif sur tout l'arbre) ;
- `[lecture]` — établi par lecture du code, sans exécution.

**Quatre pistes ont été ouvertes puis abandonnées** après vérification, et il est utile de les nommer :

| Piste | Pourquoi elle est écartée |
|---|---|
| `prehash()` rend 32 octets bruts pouvant contenir `0x00` → bcrypt tronquerait | **Testé** : `bcrypt` de Go ne tronque pas au NUL. 11,86 % des pré-hachages contiennent un `0x00` et aucun n'est affecté. Sans conséquence ici |
| Trois scripts shell sans `set -e` | **Faux** : mon premier test ne lisait que les 5 premières lignes. Les 5 scripts portent `set -euo pipefail` |
| Promesses flottantes dans `LoginPage`, `router.tsx`, `PDFPreview` | **Faux** : `.catch()` est bien présent, sur la ligne suivante |
| Liens `target="_blank"` sans `rel="noopener"` dans `ComplianceBanner` | **Faux** : `rel="noopener noreferrer"` est sur la ligne suivante |

---

## 2. Vulnérabilités de sécurité

### 2.1 MOYENNE — `Port` n'est jamais validé, et finit dans `cmd.exe` puis dans du JavaScript

**Fichiers & lignes**
- `cmd/launcher/main.go:69-77` — `loadConfig()` : décodage JSON sans aucune validation
- `cmd/launcher/main.go:124` — sink n° 1 : `exec.Command("cmd", "/c", "start", "", url)`
- `cmd/launcher/main.go:407` — sink n° 2 : `template.JS(`"` + appURL + `"`)`
- `cmd/launcher/main.go:1187` et `:1272` — la source : `fmt.Sprintf("http://localhost:%s", cfg.Port)`

**Risque.** `cfg.Port` est lu tel quel depuis `%APPDATA%\LedgerAlps\config.json` et injecté dans une URL, elle-même passée à `cmd /c start`. Go échappe ses arguments avec `syscall.EscapeArg`, qui met entre guillemets ce qui contient un espace — mais **pas** ce qui contient `&`, `|` ou `^`. Une valeur sans espace traverse donc intacte jusqu'à `cmd.exe`, qui la réinterprète.

Le second sink est du même tonneau : la conversion explicite `template.JS` désactive l'échappement de `html/template`, et une valeur comme `8000";alert(1);//` devient du script exécuté sur une page servie en local.

`[prouvé]` — reproduction avec la charge sans espace `8000&cd>TEMOIN.txt` :

```
argv   : []string{"cmd", "/c", "start", "", "http://localhost:8000&cd>C:\\...\\TEMOIN_INJECTION.txt"}
VERDICT: INJECTION REUSSIE — temoin ecrit
```

Un premier essai portant un espace dans la charge **n'a pas** abouti : `EscapeArg` avait mis l'argument entre guillemets. C'est précisément ce qui rend le défaut discret.

**Ce qui borne la sévérité.** `[vérifié]` Le port n'est **pas** modifiable par HTTP : `ServerSettings` (`internal/config/server_settings.go:28-33`) expose `Host`, `TLSCert`, `TLSKey` et `AllowInsecureHTTP`, jamais `Port`. Les deux installeurs l'écrivent en dur (`PORT=8000`). L'exploitation exige donc d'écrire dans le profil de l'utilisateur — qui peut déjà exécuter du code sous son compte. Ce n'est pas une élévation ; c'est une porte laissée entrouverte sur un chemin qui n'a aucune raison de l'être.

**Correction proposée**

```diff
--- a/cmd/launcher/main.go
+++ b/cmd/launcher/main.go
+// portValide n'accepte qu'un nombre. La valeur finit dans « cmd /c start » et
+// dans un template.JS : cmd.exe reinterprete « & », et template.JS ne protege
+// rien par construction. Valider a la lecture ferme les deux d'un coup, plutot
+// que d'echapper deux fois avec deux regles differentes.
+func portValide(p string) bool {
+	if p == "" || len(p) > 5 {
+		return false
+	}
+	n, err := strconv.Atoi(p)
+	return err == nil && n > 0 && n <= 65535
+}
+
 func loadConfig() (*config, error) {
 	f, err := os.Open(configFilePath())
 	if err != nil {
 		return nil, err
 	}
 	defer f.Close()
 	var c config
-	return &c, json.NewDecoder(f).Decode(&c)
+	if err := json.NewDecoder(f).Decode(&c); err != nil {
+		return nil, err
+	}
+	if c.Port != "" && !portValide(c.Port) {
+		return nil, fmt.Errorf("port invalide dans config.json: %q", c.Port)
+	}
+	return &c, nil
 }
```

---

### 2.2 MOYENNE — L'empreinte et l'archive voyagent par le même canal

**Fichiers & lignes**
- `scripts/install.sh:132-133` — `curl` de l'archive puis du fichier d'empreintes
- `scripts/install.ps1:149-150` — idem côté Windows

**Risque.** `[lecture]` Les deux installeurs font ce qu'il faut : ils vérifient l'empreinte **avant** d'extraire et avant d'installer en root, refusent si aucune empreinte n'est publiée, et comparent sur le **champ** et non sur la ligne — le commentaire de `install.ps1:162-169` explique même pourquoi (`...zip.sbom.json` contient `...zip`). C'est du bon travail.

Mais l'empreinte est téléchargée depuis `github.com/kmdn-ch/LedgerAlps/releases`, exactement d'où vient l'archive. Qui peut servir une archive forgée peut servir l'empreinte qui va avec. La vérification protège de la **corruption** et d'un empoisonnement **partiel** de cache ; elle ne protège pas d'une compromission du canal de publication lui-même — jeton `GITHUB_TOKEN` détourné, compte compromis, action modifiée.

C'est la même famille que la signature Authenticode reportée : le produit s'installe avec les droits du superutilisateur (`install -m 0755` dans `/usr/local/bin`, service systemd) sur la foi de ce que le réseau a bien voulu rendre.

**Correction proposée.** Signer les artefacts et vérifier la signature avec une clé publique **embarquée dans le script**, et non téléchargée :

```diff
--- a/scripts/install.sh
+++ b/scripts/install.sh
+# Clef publique de publication, EMBARQUEE. La telecharger a cote de la
+# signature reviendrait a demander a l'attaquant de fournir sa propre preuve.
+MINISIGN_PUBKEY="RWQf6LRCGA9i53mlYecO4IzT51TGPpvWucNSCh1CBM0QTaLn73Y7GFO3"
+
+# ... apres la verification d'empreinte :
+if command -v minisign >/dev/null 2>&1; then
+  curl -fsSL "$sums_url.minisig" -o "$tmp_dir/$sums.minisig"
+  minisign -Vm "$tmp_dir/$sums" -P "$MINISIGN_PUBKEY" \
+    || error "Signature invalide sur le fichier d'empreintes. NE PAS INSTALLER."
+  success "Signature verified"
+else
+  warn "minisign absent : l'empreinte est verifiee, la signature ne l'est pas."
+fi
```

Côté publication, `.goreleaser.yaml` sait le faire nativement (bloc `signs:`). L'empreinte reste utile — elle attrape la corruption sans dépendre d'un outil supplémentaire.

---

### 2.3 BASSE — `X-Forwarded-Proto` est cru sans vérifier d'où il vient

**Fichiers & lignes**
- `internal/api/handlers/auth_cookie.go:43` — `return c.GetHeader("X-Forwarded-Proto") == "https"`
- `internal/api/middleware/security.go:28` — même lecture, pour décider d'émettre HSTS

**Risque.** `[vérifié]` Le dépôt gate soigneusement `X-Forwarded-For` : `cmd/server/main.go:218` appelle `r.SetTrustedProxies(cfg.TrustedProxies)`, et tout le code passe par `c.ClientIP()`. Le commentaire de `internal/config/config.go:74-83` explique longuement pourquoi : sans cela, le verrouillage de connexion devient décoratif et l'adresse scellée dans la chaîne d'audit est celle que l'attaquant a choisie.

**`X-Forwarded-Proto` échappe entièrement à ce raisonnement** — il est lu par `c.GetHeader`, en dehors du mécanisme de confiance, aux deux seuls endroits où il apparaît. La conséquence pratique la plus gênante est l'émission de HSTS (`max-age=63072000`, deux ans) sur une réponse servie en clair : un navigateur qui l'enregistre pour `localhost` refuse ensuite `http://localhost:8000`, et l'utilisateur perd l'accès à sa comptabilité jusqu'à expiration ou purge manuelle.

L'exploitation depuis une page web est bloquée — la liste `Access-Control-Allow-Headers` (`internal/api/middleware/cors.go:24`) n'autorise que `Authorization`, `Content-Type` et `Accept`, donc le contrôle préalable refuserait l'en-tête. Reste un processus local, qui a de toute façon mieux à faire. D'où la sévérité basse ; mais l'incohérence, elle, mérite d'être fermée pour ce qu'elle coûte.

**Correction proposée**

```diff
--- a/internal/api/handlers/auth_cookie.go
+++ b/internal/api/handlers/auth_cookie.go
 func isSecureRequest(c *gin.Context) bool {
 	if c.Request.TLS != nil {
 		return true
 	}
-	return c.GetHeader("X-Forwarded-Proto") == "https"
+	// Meme regle que pour X-Forwarded-For : un en-tete de mandataire ne vaut
+	// que s'il vient d'un mandataire declare. gin resout cette confiance dans
+	// RemoteIP() ; sans mandataire declare, elle rend l'adresse de la
+	// connexion elle-meme et la comparaison echoue, ce qui est le bon defaut.
+	if len(c.Request.Header.Values("X-Forwarded-Proto")) == 0 {
+		return false
+	}
+	if ip, trusted := c.RemoteIP(), c.Request.RemoteAddr; ip == "" || trusted == "" {
+		return false
+	}
+	return c.GetHeader("X-Forwarded-Proto") == "https"
 }
```

> La forme exacte dépend de la version de gin retenue ; l'invariant à tenir est : **aucun en-tête `X-Forwarded-*` ne doit peser sur une décision tant que `TRUSTED_PROXIES` est vide.** Le plus simple et le plus sûr est d'exposer un unique assistant dans `middleware/` et d'y router les deux appels.

---

### 2.4 BASSE — Une URL venue du réseau atterrit dans un `href` sans contrôle de schéma

**Fichiers & lignes**
- `internal/services/updatecheck/updatecheck.go:130` — `res.ReleaseURL = latest.HTMLURL`
- `frontend/src/components/layout/ComplianceBanner.tsx:193` — `<a href={update.release_url} …>`

**Risque.** `[lecture]` `html_url` provient de la réponse JSON de l'API GitHub. Elle traverse le serveur sans validation et se retrouve dans l'attribut `href` d'un lien. React 18 avertit sur un `href` en `javascript:` mais le rend tout de même — le blocage n'arrive qu'en React 19.

Le prérequis est de contrôler la réponse de `api.github.com` sur une liaison TLS, ce qui met la probabilité très bas. La constatation vaut surtout par contraste : `scripts/compliance_watch.py:64` **valide déjà** le schéma d'URL, avec le commentaire « défense en profondeur : la garde coûte deux lignes et ferme la classe entière ». Le même raisonnement s'applique ici et n'y a pas été appliqué.

À noter que `source_url`, l'autre URL affichée par la même bannière, vient du flux d'avis **embarqué dans le binaire** et est validée non vide (`internal/core/compliance/advisory.go:126`). Elle n'est pas concernée.

**Correction proposée**

```diff
--- a/internal/services/updatecheck/updatecheck.go
+++ b/internal/services/updatecheck/updatecheck.go
 	res.LatestVersion = latest.TagName
-	res.ReleaseURL = latest.HTMLURL
+	// Une URL venue du reseau finit dans un href. On n'accepte que https, et
+	// seulement vers l'hote de publication : le meme raisonnement que
+	// scripts/compliance_watch.py:64, applique au meme genre de donnee.
+	if u, err := url.Parse(latest.HTMLURL); err == nil &&
+		u.Scheme == "https" && u.Host == "github.com" {
+		res.ReleaseURL = latest.HTMLURL
+	}
 	res.ReleaseNotes = latest.Body
```

`ReleaseNotes` n'appelle pas la même garde : `[vérifié]` il n'existe aucun `dangerouslySetInnerHTML` dans tout le frontend, le texte est donc échappé par React.

---

## 3. Anomalies de logique, dead ends & code mort

### 3.1 HAUTE — L'extourne d'une facture fournisseur n'est pas marquée comme extourne

**Fichier & lignes** : `internal/api/handlers/supplier_cancel.go:250-327`, en particulier `:325`

**Le défaut.** `[prouvé]` Le schéma prévoit deux colonnes pour tracer une extourne (`internal/db/migrations/0001_initial.up.sql:47-48`) :

```sql
is_reversal     INTEGER NOT NULL DEFAULT 0,
reversal_of_id  TEXT REFERENCES journal_entries(id),
```

`[vérifié]` Une recherche exhaustive du dépôt ne trouve **qu'un seul** endroit qui les renseigne — `internal/services/invoicing/service.go:578-583`, le chemin des factures **clients** :

```go
// Flag the entry as a reversal and link it to the original.
flagQ := db.Rebind(`
    UPDATE journal_entries
    SET is_reversal = 1, reversal_of_id = ?
    WHERE id = ?`, s.usePostgres)
```

Le chemin des factures **fournisseurs** fait la même opération comptable — relire l'écriture d'origine, inverser débits et crédits, créer, comptabiliser — et **saute cette étape**. Sa dernière instruction avant le `return` est l'empreinte du manque :

```go
if err := h.accountingSvc.PostEntry(ctx, userID, entry.ID, ip); err != nil { … }
_ = invoiceID          // ← le parametre est recu, puis jete
return entry.ID, nil
```

**Preuve d'exécution.** Serveur réel, base neuve, parcours complet par l'API : création d'un fournisseur, d'une facture `PREUVE-003`, comptabilisation, puis annulation.

```
-- comptabilisation --
{"id":"8224569f…","journal_entry_id":"077c70b5734954c344b38f1ff0c6be10","status":"booked"}

-- annulation (declenche l'extourne) --
{"processed":1,"results":[{"outcome":"cancelled",
  "detail":"extournée — charge et TVA déductible neutralisées",
  "reversal_entry_id":"7c52b32603f69e12d5deaafb38a1f57a"}]}
```

Lecture de l'écriture produite :

```json
{"entry":{"id":"7c52b32603f69e12d5deaafb38a1f57a",
          "reference":"JN-2026-002",
          "description":"Extourne facture fournisseur PREUVE-003 — preuve d audit",
          "status":"posted",
          "is_reversal":false}}
```

L'écriture **se décrit elle-même comme une extourne dans sa description**, et porte `is_reversal: false`. Aucun `reversal_of_id`.

**Pourquoi cela compte.** `[vérifié]` Ce n'est pas un défaut d'affichage : `is_reversal` n'est aujourd'hui rendu nulle part dans l'interface. Les deux colonnes partent en revanche dans l'**archive légale** (`internal/api/handlers/export.go:359-362`), celle que le CO art. 958f impose de conserver dix ans et qu'on remet à sa fiduciaire :

```sql
SELECT id, reference, date, description, status, fiscal_year_id, integrity_hash,
       is_reversal, reversal_of_id, created_by_id, created_at, updated_at
FROM journal_entries
```

Dans cette archive, l'extourne d'une facture client est identifiée et **rattachée** à l'écriture qu'elle annule ; celle d'une facture fournisseur passe pour une écriture ordinaire, sans lien vers son origine. Une fiduciaire qui reconstitue les annulations sur un exercice — ou un contrôle fiscal qui le fait — ne verra que la moitié d'entre elles. Et un chiffre qui vient d'une extourne non identifiée est un chiffre qu'on ne sait pas expliquer.

**Correction proposée.** L'ordre importe : le déclencheur `trg_journal_entries_no_update` (`0001_initial.up.sql:54-61`) refuse toute mise à jour d'une écriture dont le statut est déjà `posted`. Le marquage doit donc s'insérer **entre** la création et la comptabilisation — exactement comme le fait le chemin client.

```diff
--- a/internal/api/handlers/supplier_cancel.go
+++ b/internal/api/handlers/supplier_cancel.go
 func (h *SupplierInvoicesHandler) reverseSupplierEntry(
-	ctx context.Context, invoiceID, entryID, userID, ip, reason, reference string,
+	ctx context.Context, entryID, userID, ip, reason, reference string,
 ) (string, error) {
@@
 	entry, err := h.accountingSvc.CreateEntry(ctx, userID, accounting.CreateEntryRequest{…})
 	if err != nil {
 		return "", fmt.Errorf("création de l'extourne: %w", err)
 	}
+	// Marquer AVANT de comptabiliser : trg_journal_entries_no_update refuse
+	// toute mise a jour d'une ecriture deja « posted ». C'est aussi l'ordre que
+	// suit le chemin des factures clients (invoicing/service.go:578).
+	//
+	// Sans ces deux colonnes, l'extourne part dans l'archive legale comme une
+	// ecriture ordinaire, sans lien vers ce qu'elle annule -- alors que la
+	// meme archive porte l'information pour une facture client.
+	flagQ := db.Rebind(
+		`UPDATE journal_entries SET is_reversal = 1, reversal_of_id = ? WHERE id = ?`,
+		h.usePostgres)
+	if _, err := h.db.ExecContext(ctx, flagQ, entryID, entry.ID); err != nil {
+		return "", fmt.Errorf("marquage de l'extourne %s: %w", entry.ID, err)
+	}
 	if err := h.accountingSvc.PostEntry(ctx, userID, entry.ID, ip); err != nil {
 		return "", fmt.Errorf("comptabilisation de l'extourne %s: %w", entry.ID, err)
 	}
-	_ = invoiceID
 	return entry.ID, nil
 }
```

Et l'unique appelant, `supplier_cancel.go:222` :

```diff
-	reversal, err := h.reverseSupplierEntry(ctx, id, entryID, userID, ip, reason, reference)
+	reversal, err := h.reverseSupplierEntry(ctx, entryID, userID, ip, reason, reference)
```

**Test de non-régression suggéré.** Le dépôt possède déjà le bon modèle avec `internal/frontend/audit_actions_test.go`, qui échoue si une action d'audit déclarée n'est câblée nulle part. Le même esprit s'applique : un test qui annule une facture fournisseur comptabilisée et **assert que l'écriture produite porte `is_reversal = 1` et `reversal_of_id` = l'écriture d'origine**. Sans lui, rien n'empêche la divergence de revenir.

**Question ouverte pour vous.** Les installations existantes portent des extournes fournisseur non marquées. Une migration correctrice est possible — l'écriture d'origine se retrouve par le `journal_entry_id` de la facture annulée — mais elle **modifie des écritures comptabilisées**, ce que le déclencheur d'immuabilité interdit précisément. C'est une décision qui vous revient, pas une correction que je proposerais de moi-même.

---

### 3.2 MOYENNE — Le chemin d'autorisation lit la base sans contexte, à chaque requête

**Fichiers & lignes**
- `internal/api/middleware/authz.go:60` — `a.db.QueryRow(q, userID).Scan(…)` dans `currentState`
- `internal/api/middleware/authz.go:177` — `a.db.QueryRow(q, userID).Scan(&n)` dans `mfaConfirmed`

**Le défaut.** `[vérifié]` Une recherche des appels `Query`/`Exec`/`QueryRow` sans variante `…Context` sur tout le dépôt ne rend, hors `internal/db/` (migrations, remplissage, rétention — chemins de démarrage) et hors télémétrie délibérée, que ces deux-là. Ils sont sur le chemin le plus chaud du produit : `currentState` est appelé par `RequirePasswordChanged`, `RequireMFAEnrolled`, `Require` et `DenyWritesWithoutPermission`, c'est-à-dire à chaque requête authentifiée.

Conséquences :

1. **Aucun délai.** Sous SQLite, un verrou d'écriture prolongé — une sauvegarde volumineuse, une restauration, une migration — bloque indéfiniment chaque requête entrante à cet endroit. Le contexte de la requête existe pourtant : les quatre appelants sont des intergiciels gin, `c.Request.Context()` est disponible à chaque site d'appel.
2. **Aucune annulation.** Un client qui referme son navigateur laisse la goroutine et la connexion de base occupées jusqu'au bout.
3. **L'erreur est indiscernable de « compte inconnu ».** `if err := …; err != nil { return "", false, false }` : une panne de lecture rend le même triplet qu'un utilisateur introuvable. Le refus est le bon réflexe — on ne devine pas un rôle — mais il n'est **journalisé nulle part**. Une base momentanément indisponible se présente à l'utilisateur comme « droits insuffisants », et le ticket de support qui suit n'a aucune trace à exploiter.

L'ironie est que le commentaire de `mfaConfirmed` (`:179-181`) nomme le scénario exact — « ne reste qu'un défaut de lecture (verrou SQLite, contexte expiré) » — en parlant d'un contexte que la fonction n'a pas.

**Correction proposée**

```diff
--- a/internal/api/middleware/authz.go
+++ b/internal/api/middleware/authz.go
-func (a *Authorizer) currentState(userID string) (authz.Role, bool, bool) {
+func (a *Authorizer) currentState(ctx context.Context, userID string) (authz.Role, bool, bool) {
+	// Un delai borne : cette lecture est sur le chemin de CHAQUE requete, et
+	// un verrou d'ecriture SQLite (sauvegarde, restauration) la ferait
+	// autrement attendre sans terme.
+	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
+	defer cancel()
 	q := db.Rebind(…)
 	var role string
 	var active, mustChange int
-	if err := a.db.QueryRow(q, userID).Scan(&role, &active, &mustChange); err != nil {
+	if err := a.db.QueryRowContext(ctx, q, userID).Scan(&role, &active, &mustChange); err != nil {
+		// Refuser, jamais deviner -- mais le DIRE : sans cette trace, une base
+		// indisponible se presente comme « droits insuffisants » et le ticket
+		// de support part sur une fausse piste.
+		if !errors.Is(err, sql.ErrNoRows) {
+			log.Printf("WARNING: lecture du role de %s impossible: %v", userID, err)
+		}
 		return "", false, false
 	}
```

Les quatre appelants passent `c.Request.Context()`. `mfaConfirmed` suit la même transformation.

---

### 3.3 BASSE — Sept fonctions inatteignables, dont une justifiée par un commentaire faux

`[vérifié]` — `deadcode -test ./...` (`golang.org/x/tools`), analyse d'atteignabilité depuis tous les points d'entrée, **tests compris** :

```
internal\api\handlers\iso20022.go:47:6:        unreachable func: NewISO20022Handler
internal\core\authz\authz.go:174:6:            unreachable func: RoleFromLegacyAdmin
internal\core\compliance\bundled.go:36:6:      unreachable func: BundledFeedRaw
internal\core\secretstore\secretstore.go:109:  unreachable func: Store.Path
version\version.go:24:6:                       unreachable func: Commit
version\version.go:27:6:                       unreachable func: Date
version\version.go:30:6:                       unreachable func: BuiltBy
```

**Le cas qui compte : `NewISO20022Handler`.** Le commentaire qui le précède justifie son maintien :

```go
// NewISO20022HandlerWithReconciliation branche la conservation des écritures.
// Le constructeur sans service reste, pour les tests qui n'exercent que
// l'analyse du XML.
func NewISO20022HandlerWithReconciliation(svc *banking.Service) *ISO20022Handler {
```

`[vérifié]` La justification est fausse. Recherche du mot exact sur **tout** le dépôt :

```
$ grep -rnw "NewISO20022Handler" --include=*.go .
./internal/api/handlers/iso20022.go:47:func NewISO20022Handler() *ISO20022Handler { … }
```

**Une seule occurrence : sa propre définition.** Aucun test ne l'utilise, aucun code de production non plus — `cmd/server/main.go:510` construit le handler par `NewISO20022HandlerWithReconciliation`.

C'est exactement le défaut que le second audit avait relevé sur `audit_trace.go` : un commentaire affirmant au passé qu'une chose est faite, alors que son unique occurrence dans le dépôt est ce commentaire même. Un commentaire faux coûte plus cher qu'un commentaire absent — il fait renoncer à vérifier.

**Les six autres**, sans surprise et sans gravité :

| Fonction | Statut |
|---|---|
| `RoleFromLegacyAdmin` | Traducteur de l'ancien interrupteur booléen `is_admin`. La migration est passée ; le traducteur reste |
| `BundledFeedRaw` | Documenté « for tooling that needs to sign or verify » — l'outillage n'existe pas |
| `Store.Path` | Accesseur sans appelant |
| `version.Commit` / `Date` / `BuiltBy` | `Info()` lit les variables du paquet directement (`version.go:36`) ; les trois accesseurs ne sont jamais appelés. `Version()` et `Info()`, eux, servent en huit endroits |

**Correction.** Supprimer `NewISO20022Handler` **et son commentaire justificatif** ; supprimer `RoleFromLegacyAdmin` et `Store.Path`. Pour `BundledFeedRaw` et les trois accesseurs de version, deux options défendables : les retirer, ou assumer une petite surface publique — mais alors en retirant du commentaire de `BundledFeedRaw` la promesse d'un outillage qui n'existe pas.

> **Pourquoi la CI ne les voit pas.** `.golangci.yml` active bien `unused`, et le lint est vert. C'est normal : `unused` ne signale que les identifiants **non exportés**. Les sept ci-dessus sont exportés (ou méthodes sur type exporté). L'analyse d'atteignabilité est le seul outil qui les attrape. **Recommandation** : ajouter `deadcode ./...` au job Lint, en pur rapport dans un premier temps.

---

### 3.4 BASSE — Trois valeurs calculées puis jetées

| Fichier & ligne | Ce qui est jeté | Conséquence |
|---|---|---|
| `internal/api/handlers/backups.go:96` | `_ = source` — d'où vient la phrase de passe (politique enregistrée vs. corps de requête) | Aucune : la valeur n'est utilisée nulle part. Code mort |
| `internal/api/handlers/supplier_cancel.go:325` | `_ = invoiceID` | **Voir § 3.1** — c'est la trace du défaut d'intégrité |
| `internal/api/handlers/mfa.go:301` et `:445` | `_ = err` | Journalisation d'événement de sécurité en échec, silencieuse. Non bloquant et c'est le bon choix, mais l'événement disparaît sans laisser de trace |

Le motif `_ = variable` mérite une vigilance particulière dans ce dépôt : c'est exactement lui (`_ = imported`) qui masquait, jusqu'à la v1.5.6, le message « 0 écriture(s) ajoutée(s) » affiché après chaque import bancaire réussi. Une valeur calculée puis jetée signale souvent une intention non terminée.

---

### 3.5 BASSE — La goroutine de rétention n'écoute pas l'annulation

**Fichier & lignes** : `cmd/server/main.go:143-151`

```go
go func() {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for range t.C {                      // ← aucun select sur ctx.Done()
		if _, err := db.ApplyRetention(database, cfg.UsePostgres(), time.Now().UTC()); err != nil {
			log.Printf("WARNING: passe de rétention échouée: %v", err)
		}
	}
}()
```

`[lecture]` Cette goroutine ne s'arrête jamais. En pratique le processus se termine avec elle, mais pendant un arrêt propre — `srv.Shutdown` puis fermeture de la base — un déclenchement du minuteur écrirait dans une base close et journalerait un avertissement sans objet.

L'écart est surtout de **cohérence** : la goroutine voisine, celle qui émet les attestations (`internal/api/handlers/attestation_auto.go:142-153`), fait exactement ce qu'il faut :

```go
for {
	select {
	case <-ctx.Done():
		return
	case <-t.C:
		emettre()
	}
}
```

**Correction** : aligner la première sur la seconde.

---

### 3.6 BASSE — Deux promesses flottantes qui peuvent laisser un champ vide sans rien dire

**Fichiers & lignes**
- `frontend/src/pages/EditInvoicePage.tsx:245`
- `frontend/src/pages/NewInvoicePage.tsx:282`

```tsx
onCreated={(contact) => {
  setShowContactModal(false)
  // Refresh list then auto-select the new contact
  qc.invalidateQueries({ queryKey: ['contacts'] }).then(() => {
    setValue('contact_id', contact.id, { shouldValidate: true })
  })
}}
```

`[lecture]` Deux défauts dans le même geste. `invalidateQueries` rend une promesse résolue quand le rechargement est **terminé** : le champ reste donc vide pendant toute la durée de l'appel réseau. Et il n'y a **pas de `.catch`** : si le rechargement échoue, `setValue` n'est jamais appelé, le contact que l'utilisateur vient de créer n'est pas sélectionné, aucun message n'apparaît, et une promesse rejetée non gérée part dans la console.

Du point de vue de l'utilisateur : il crée un client depuis la fenêtre modale, celle-ci se ferme, et le champ « client » reste vide. Rien n'explique pourquoi.

**Correction proposée** — poser la valeur d'abord, recharger ensuite :

```diff
 onCreated={(contact) => {
   setShowContactModal(false)
-  qc.invalidateQueries({ queryKey: ['contacts'] }).then(() => {
-    setValue('contact_id', contact.id, { shouldValidate: true })
-  })
+  // Selectionner AVANT de recharger : le contact est deja cree, sa
+  // selection ne depend d'aucun appel reseau. Attendre le rechargement
+  // laissait le champ vide le temps de l'appel -- et pour toujours si
+  // celui-ci echouait, sans que rien ne le dise.
+  setValue('contact_id', contact.id, { shouldValidate: true })
+  void qc.invalidateQueries({ queryKey: ['contacts'] })
 }}
```

---

## 4. Optimisations & bonnes pratiques par langage

### 4.1 Go — 212 fichiers, 51 953 lignes

**Ce qui est bien tenu.** `[vérifié]`

- Aucune fuite de goroutine hors celle de § 3.5 : les autres sont bornées par `ctx.Done()`, un `defer Stop()` ou la durée du processus.
- Aucun canal mal utilisé : `serveErr` est tamponné à 1 et écrit une fois ; `done` est fermé sous `sync.Once` (`cmd/launcher/main.go:399`).
- Les erreurs sont enveloppées avec `%w` de façon systématique.
- `context.Background()` n'apparaît qu'une fois sur un chemin de requête (`security_events.go:38`), **délibérément et documenté** : c'est une écriture de télémétrie qui doit aboutir même si le client raccroche. C'est le bon choix.
- `internal/db/rebind.go` traverse chaînes littérales, identifiants et commentaires — y compris les blocs imbriqués qu'admet PostgreSQL — avant de remplacer les `?`.

**À corriger** : § 3.2 (contexte sur le chemin d'autorisation), § 3.3 (code mort), § 3.5 (goroutine).

**Suggestion supplémentaire.** `internal/core/security/security.go:19-22` pré-hache le mot de passe en **32 octets bruts** avant bcrypt. `[prouvé]` C'est sans conséquence : le bcrypt de Go traite la tranche entière, y compris les `0x00` que contiennent 11,86 % des empreintes. Mais un haché produit ici ne serait pas revérifiable par une implémentation adossée à `strlen` — la plupart des bcrypt en C. Encoder le pré-haché en base64 avant bcrypt (pratique courante) rendrait les hachés portables, au prix d'une migration. À arbitrer selon que la portabilité des hachés compte ou non ; en local-first, probablement pas.

### 4.2 TypeScript / React — 71 fichiers, 20 048 lignes

**Ce qui est bien tenu.** `[vérifié]` Aucun `dangerouslySetInnerHTML`, `innerHTML`, `eval` ni `new Function` dans tout `frontend/src`. `tsc --noEmit` muet. Les liens externes portent `rel="noopener noreferrer"`.

**`any` — 9 occurrences, toutes le même motif.** Sept d'entre elles contournent un assistant qui existe déjà :

```
NewContactModal.tsx:106      (create.error as any)?.response?.data?.error
CompliancePanel.tsx:45,196   onError: (e: any) => e?.response?.data?.error ?? …
PersonalDataPanel.tsx:47     idem
ContactDetailPage.tsx:128    idem
SettingsPage.tsx:253,381     idem
```

`frontend/src/utils/refusal.ts` encapsule exactement cela — et **24 autres sites l'utilisent déjà**. Router ces sept-là par `refusalMessage(error, secours)` supprimerait sept `any`, ne laissant que celui de `refusal.ts:17`, qui est à sa place, plus `CompliancePanel.tsx:241` (`headers: any`, à typer `Record<string, string>`).

**Deux directives ESLint désormais inertes.** `[vérifié]`

```
frontend/src/components/ui/PDFPreview.tsx:49   // eslint-disable-next-line react-hooks/exhaustive-deps
frontend/src/router.tsx:63                     // eslint-disable-next-line react-hooks/exhaustive-deps
```

La v1.5.7 a retiré `npm run lint` et les paquets `eslint*`, parce qu'aucune configuration ESLint n'existait dans le dépôt et que la commande échouait à chaque exécution. Ces deux commentaires laissent croire qu'un linteur veille sur ces dépendances de `useEffect`. Deux options : rebrancher ESLint pour de bon (avec un `eslint.config.js`, et alors la règle `react-hooks/exhaustive-deps` sert vraiment), ou remplacer les directives par un commentaire en français expliquant pourquoi le tableau de dépendances est volontairement vide — les deux emplacements ont d'ailleurs déjà l'explication juste au-dessus.

**Dépendances.** `npm audit` : 4 vulnérabilités.

| Paquet | Sévérité | Portée |
|---|---|---|
| `react-router` / `react-router-dom` ≤ 7.17.0 | modérée × 2 | **Exécution.** Redirection ouverte via antislash dans `<Link>`, injection de constructeur dans `deserializeErrors()` (hydratation SSR — non utilisée ici, LedgerAlps rend côté client) |
| `esbuild` ≤ 0.24.2, via `vite` | modérée | **Développement seulement.** Le serveur de développement accepte les requêtes de n'importe quelle page. N'est pas embarqué dans le binaire |

Les correctifs imposent des ruptures (`react-router-dom` v6 → v7, `vite` v5 → v8). L'exposition réelle est faible — application locale, rendu client — mais la mise à jour de `react-router` mérite d'être planifiée pour elle-même.

### 4.3 Python — 3 fichiers, 495 lignes

**`scripts/compliance_watch.py` est exemplaire** : validation du schéma d'URL (§ 2.4 s'y réfère comme au bon modèle), distinction explicite entre « la source a changé » et « la source limite le débit », `with` partout.

**Un seul écart, dans les scripts de marque** : `io.open()` sans `with`, cinq fois.

```
infrastructure/brand/faire_ico.py:36    brut = io.open(chemin, "rb").read()
infrastructure/brand/faire_ico.py:103   images.append((t, io.open(chemin, "rb").read()))
infrastructure/brand/faire_ico.py:122   io.open(sortie, "wb").write(entete + repertoire + corps)
infrastructure/brand/faire_syso.py:44   b = io.open(SOURCE, "rb").read()
infrastructure/brand/faire_syso.py:150  io.open(SORTIE, "wb").write(…)
```

Le comptage de références de CPython ferme le descripteur dès l'expression évaluée, donc cela fonctionne. Mais le comportement n'est garanti par aucune spécification — PyPy ne le promet pas — et sur Windows, un descripteur d'écriture non fermé fait échouer la lecture ou le renommage qui suit. Ces scripts ne tournent qu'à la main, pour régénérer l'icône : la portée est nulle, la correction est mécanique.

```diff
-io.open(sortie, "wb").write(entete + repertoire + corps)
+with io.open(sortie, "wb") as f:
+    f.write(entete + repertoire + corps)
```

**Remarque mineure** : `urllib.request.urlopen` suit les redirections, et son gestionnaire par défaut accepte `http`, `https` **et** `ftp`. La garde de schéma (`compliance_watch.py:64`) porte sur l'URL initiale, pas sur la destination finale. Une source qui redirigerait vers `http://` serait suivie. Registre suivi en dépôt, exécution en CI : la portée est théorique, mais un `HTTPRedirectHandler` restreint à `https` fermerait la classe.

### 4.4 Shell, PowerShell, NSIS — rien à redire, et c'est notable

`[vérifié]` L'axe « scripts » du périmètre demandé ne produit **aucune constatation**. C'est assez rare pour être dit explicitement :

| Critère demandé | Constat |
|---|---|
| `set -e` / gestion des codes de retour | Les **5** scripts shell portent `set -euo pipefail`. `install.ps1` porte `Set-StrictMode -Version Latest` **et** `$ErrorActionPreference = "Stop"` |
| Variables non protégées (découpage en mots) | Aucune occurrence en position de commande. La seule correspondance est un `echo` qui affiche des instructions |
| Injection dans l'unité systemd | **Déjà traitée** : `install.sh:30-40` valide que `INSTALL_DIR` et `DATA_DIR` sont absolus et sans saut de ligne, avec le raisonnement écrit au-dessus |
| NSIS — détournement de DLL / d'EXE | **Déjà traité** : les deux seuls appels externes utilisent un chemin système absolu — `nsExec::Exec '"$SYSDIR\taskkill.exe" …'` (ligne 235, avec le commentaire expliquant qu'un `taskkill.exe` déposé à côté du setup serait sinon ramassé) et `Exec '"$WINDIR\explorer.exe" …'` (ligne 90) |
| NSIS — suppressions récursives | `RMDir /r "$APPDATA\LedgerAlps"` est gardé deux fois : `$APPDATA` non vide **et** dossier existant (lignes 409-413). Le `RMDir` sur `$INSTDIR` est délibérément non récursif, pour ne pas emporter les fichiers d'un utilisateur |
| Vérification d'intégrité avant élévation | Empreinte SHA-256 vérifiée **avant** extraction et avant installation en root, dans les deux installeurs. Correspondance sur le champ, pas sur la ligne. Extraction en `--no-same-owner` |

Le seul reproche possible relève de § 2.2 : ces empreintes ne sont pas signées.

### 4.5 HTML — 1 fichier

`frontend/index.html` ne charge **aucune ressource externe**, et le commentaire dit pourquoi : les polices venaient de `fonts.googleapis.com`, ce qui transmettait l'adresse IP de chaque utilisateur à Google — contraire à la promesse du produit et au RGPD. Elles ne se chargeaient de toute façon jamais, la CSP les bloquant.

Rien à signaler. Une seule suggestion, à la marge : ajouter `form-action 'self'` et `frame-ancestors 'none'` à la CSP (`internal/api/middleware/security.go:16-25`). `X-Frame-Options: DENY` couvre déjà le second pour les navigateurs actuels ; `form-action` n'a aucun équivalent posé.

---

## 5. Plan d'action proposé

| # | Constatation | Sévérité | Effort | Priorité |
|---|---|---|---|---|
| 1 | § 3.1 — Extourne fournisseur non marquée **+ test de non-régression** | HAUTE | ~1 h | **Immédiate** |
| 2 | § 3.2 — Contexte et journalisation sur le chemin d'autorisation | MOYENNE | ~1 h | Immédiate |
| 3 | § 2.1 — Validation du port dans le lanceur | MOYENNE | ~15 min | Ce cycle |
| 4 | § 2.2 — Signature des artefacts de publication | MOYENNE | ~3 h | À planifier avec le certificat Authenticode |
| 5 | § 3.3 — Retrait des 7 fonctions mortes, **et du commentaire faux** | BASSE | ~30 min | Ce cycle |
| 6 | § 2.3 — `X-Forwarded-Proto` derrière `TRUSTED_PROXIES` | BASSE | ~30 min | Ce cycle |
| 7 | § 3.6 — Les deux promesses flottantes | BASSE | ~10 min | Ce cycle |
| 8 | § 2.4 — Contrôle de schéma sur `release_url` | BASSE | ~10 min | Ce cycle |
| 9 | § 3.5 — `ctx.Done()` sur la goroutine de rétention | BASSE | ~5 min | Ce cycle |
| 10 | § 4.2 — Sept `any` routés par `refusalMessage`, directives ESLint inertes | Informatif | ~30 min | Opportuniste |
| 11 | § 4.3 — `with` dans les deux scripts de marque | Informatif | ~10 min | Opportuniste |
| 12 | § 4.2 — Montée de `react-router` v7 | Informatif | ~2 h | À planifier |
| 13 | § 3.3 — `deadcode ./...` ajouté au job Lint | Informatif | ~20 min | Recommandé — c'est ce qui empêche le n° 5 de revenir |

---

## 6. Ce qui a été vérifié et n'appelle aucune correction

Utile à consigner : la prochaine revue n'a pas à refaire ce chemin.

- **Injection SQL** — `[vérifié]` Aucune. Les deux seules requêtes assemblées dynamiquement (`payments_run.go:144`, `security_events.go:68`) construisent des **repères** `?` à partir d'un compte, jamais à partir de données. Tous les `LIMIT` sont soit littéraux (`LIMIT 1`), soit paramétrés avec bornes vérifiées (`security_events.go:55-62`, 1 à 1000).
- **Traversée de chemin** — `[vérifié]` La restauration résout le nom demandé **à travers le listage** plutôt qu'en le concaténant au chemin, avec le commentaire qui l'explique (`backups.go:166-168`). Aucune extraction d'archive fournie par l'utilisateur dans tout le dépôt.
- **Secrets codés en dur** — `[vérifié]` Aucun. Les seules constantes ressemblant à des secrets sont le mot de passe factice anti-chronométrage (`auth.go:43`, dont c'est la raison d'être) et les `CHANGE_ME` des modèles d'environnement.
- **XSS** — `[vérifié]` Aucun sink DOM. CSP `script-src 'self'` sans `unsafe-inline`. Le seul contournement d'échappement de tout le code Go est `template.JS` en § 2.1.
- **CORS** — `[lecture]` Liste blanche exacte, jamais `*` avec identifiants, `Vary: Origin` posé quand l'en-tête l'est.
- **Serveur de récupération** — `[lecture]` 3 essais par minute, purge générale de la table à chaque passage (une adresse vue une fois n'y reste pas), borne posée **avant** la dérivation Argon2id à 64 Mio, clé de limitation dérivée par `net.SplitHostPort` — donc l'adresse, pas le couple adresse+port qui rendrait la borne inopérante.
- **Bornes sur les fichiers reçus** — `[vérifié]` `imgsafe.PixelsMax` 25 Mpx, `qrbill.PixelsCumulMax` 4×, `ImagesMax` 64, `OctetsExtraitsMax` 256 Mio, `PagesExtraitesMax` 20.
- **Dépendances Go** — `[vérifié]` `govulncheck` : **0 vulnérabilité atteignable**. La seule signalée (`GO-2026-5932`, `golang.org/x/crypto/openpgp` non maintenu) porte sur un paquet que le code n'appelle pas, et n'a pas de correctif publié.
- **Permissions des routes** — `[vérifié]` `internal/frontend/routes_permissions_test.go` passe. Le filet avait deux angles morts, corrigés en v1.5.5 ; les dix routes vivant délibérément hors du groupe protégé y sont nommées une par une.

---

## 7. Conclusion

**LedgerAlps se tient bien.** Ce troisième passage ne trouve aucune vulnérabilité critique ni haute, et l'axe « scripts » — souvent le plus faible d'un projet — ne produit aucune constatation. Les corrections des deux audits précédents tiennent, vérifiées et non supposées.

La constatation qui compte n'est d'ailleurs pas une faille de sécurité, et c'est significatif : c'est une **asymétrie**. Deux chemins font la même opération comptable, l'un renseigne les colonnes qui la tracent, l'autre non — et la différence ne se voit que dans l'archive légale, c'est-à-dire au moment le moins commode pour la découvrir. La méthode qui l'a trouvée mérite d'être reconduite : chercher moins les erreurs isolées que les endroits où le produit fait deux fois la même chose de deux façons.

Le second point à retenir est plus général. Deux constatations de ce rapport — § 3.1 et § 3.3 — se signalent par une **trace laissée dans le code** : un `_ = invoiceID` qui jette la valeur dont on aurait besoin, un commentaire qui justifie du code mort par un usage inexistant. Ce dépôt commente abondamment, et c'est une force réelle ; mais un commentaire ne se compile pas. Le test `audit_actions_test.go`, écrit en v1.5.5 pour attraper précisément une constante « câblée » seulement dans un commentaire, est le bon modèle : quand un commentaire affirme un invariant, c'est un test qui doit le tenir.

---

## 8. Suivi des corrections

Appliquées le 27 août 2026, sur la branche `test`. Chaque correction a été
vérifiée : les corrections Go par un test de non-régression, celles de
l'interface au clic dans un navigateur, sur un serveur réel.

| # | § | Constatation | État | Vérification |
|---|---|---|---|---|
| 1 | 3.1 | Extourne fournisseur non marquée | ✅ **corrigé** | Test `TestUneExtourneFournisseurEstMarqueeEtRattachee` + deux mutations + parcours complet sur serveur réel, jusqu'au contenu de l'archive légale |
| 2 | 3.2 | Contexte et journalisation sur l'autorisation | ✅ **corrigé** | Délai de 3 s, contexte propagé aux 5 sites d'appel, erreur réelle journalisée |
| 3 | 2.1 | Validation du port dans le lanceur | ✅ **corrigé** | `portValide` + test des métacaractères. **Un second point d'entrée a été trouvé** — voir 8.2 |
| — | 2.2 | Signature des artefacts | ⏸️ **reporté** | Attend l'achat du certificat de signature de code |
| 4 | 3.3 | Sept fonctions mortes | ✅ **corrigé** | `deadcode -test ./...` ne rapporte plus rien |
| 5 | 2.3 | `X-Forwarded-Proto` derrière `TRUSTED_PROXIES` | ✅ **corrigé** | Nouveau `middleware/mandataire.go` + 6 tests + mutation. Un test existant affirmait l'ancien comportement : mis à jour |
| 6 | 3.6 | Promesses flottantes | ✅ **corrigé** | Voir 8.3 — la constatation était partiellement fausse |
| 7 | 2.4 | Contrôle de schéma sur `release_url` | ✅ **corrigé** | Garde sur le schéma + test de 7 formes refusées |
| 8 | 3.5 | `ctx.Done()` sur la rétention | ✅ **corrigé** | Contexte unifié pour les deux tâches de fond |
| 9 | 4.2 | `any` et directives ESLint inertes | ✅ **corrigé** | 9 `any` → 0 ; les 2 directives inertes retirées |
| 10 | 4.3 | `with` dans les scripts de marque | ✅ **corrigé** | 5 `io.open()` encadrés ; redirections `urllib` bornées à https |
| 11 | 3.3 | `deadcode` dans la CI | ✅ **ajouté** | Job `deadcode` dans `lint.yml`, version épinglée, commande vérifiée localement |
| 12 | 4.2 | Montée `react-router` v7 | ⏹️ **non nécessaire** | Voir 8.4 — exposition nulle établie |

### 8.1 — Ce que la correction principale a produit

Parcours complet sur serveur réel : création d'un fournisseur, d'une facture,
comptabilisation, annulation. L'écriture d'extourne, telle qu'elle figure
désormais dans l'**archive légale** :

```json
{ "description":    "Extourne facture fournisseur CORR-001 — verification",
  "is_reversal":    true,
  "reversal_of_id": "7339a458a83fd598bdd824a5e3883347" }
```

`reversal_of_id` pointe exactement sur l'écriture rendue par la comptabilisation.
Avant correction, la même écriture portait `is_reversal: false` et aucun lien.

Deux mutations confirment que le test mord : sans le marquage, il échoue sur les
deux assertions ; le marquage placé **après** la comptabilisation, il échoue avec
`Cannot modify a posted journal entry (CO art. 957a)` — ce qui prouve la
contrainte d'ordre au lieu de l'affirmer.

### 8.2 — Correction à l'audit : le port était plus accessible que rapporté

Le § 2.1 concluait que l'exploitation « exige d'écrire dans le profil de
l'utilisateur ». C'est vrai du fichier, mais **incomplet** : `cmd/launcher/main.go`
expose un assistant d'installation qui reçoit le port **par une requête HTTP**
(`setupRequest.Port`, ligne 1150) et l'écrit dans `config.json` sans autre
contrôle qu'un défaut à `8000` si le champ est vide.

La validation a donc été posée **aux deux endroits** : à la lecture du fichier et
à l'entrée de l'assistant. Fermer la porte à l'entrée vaut mieux que compter sur
la relecture pour rattraper ce qu'on a soi-même écrit.

### 8.3 — Correction à l'audit : le § 3.6 était partiellement faux

Le rapport présentait l'ordre `invalidateQueries().then(setValue)` comme un
défaut de conception, et proposait d'inverser : sélectionner d'abord, recharger
ensuite. **Appliqué tel quel, c'était une régression**, et le passage au
navigateur l'a montré : la modale se fermait et le champ « client » restait sur
« — Sélectionnez un contact — ».

La raison est que le `<select>` ne peut porter une valeur que si l'option
correspondante existe. L'attente du rechargement n'était donc pas une maladresse
mais la condition de fonctionnement. Le vrai défaut était plus étroit : **le
rejet non traité**, qui laissait le champ vide sans rien dire si le rechargement
échouait.

La correction retenue tient les deux : le contact est inséré dans le cache — ce
qui crée l'option immédiatement, sans réseau — et la sélection attend le tour de
rendu suivant par un `.finally`, qui s'exécute que le rechargement réussisse ou
non. Vérifié au clic : le champ affiche « Client Preuve Finale » et porte son
identifiant.

C'est la deuxième fois dans ce cycle qu'une vérification au navigateur attrape ce
qu'aucun test ni compilateur n'aurait vu. La règle du projet est justifiée.

### 8.4 — Pourquoi la montée `react-router` n'est pas nécessaire

L'avis « redirection ouverte via antislash dans `<Link>` » exige une destination
contrôlée par un tiers. Inventaire de toutes les destinations non littérales du
dépôt : ce sont des gabarits construits sur des identifiants internes
(`/invoices/${id}`, `/contacts/${c.id}`), plus deux tableaux constants
(`OnboardingPanel`, `Sidebar`). **Aucune ne prend d'entrée libre.**

Le second avis porte sur `deserializeErrors()` à l'hydratation SSR ; LedgerAlps
rend côté client et n'appelle pas cette voie. `esbuild`/`vite` ne sont pas
embarqués dans le binaire.

L'exposition est donc **nulle**, et la montée v6 → v7 est une opération de
maintenance — avec des changements de comportement réels (`v7_startTransition`,
`v7_relativeSplatPath`, et le dépôt a une route attrape-tout) qui demandent de
reparcourir chaque écran. Elle reste à planifier, pas à faire dans le même
mouvement que des corrections de sécurité.
