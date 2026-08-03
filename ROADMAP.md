# LedgerAlps — Roadmap

> **Politique de versionnage**
> - `vX.Y.0` — livraison d'un milestone fonctionnel complet
> - `vX.Y.Z` (Z > 0) — correctifs groupés dans le cycle du milestone, taggés une fois stables
> - On ne pose **jamais** un tag par commit isolé

---

## En cours — Interface multilingue (FR / DE / IT / EN)

La Suisse compte quatre langues officielles. LedgerAlps supportera FR, DE, IT et EN.

> Ce milestone visait initialement le numéro v1.4. Celui-ci a été attribué à la
> veille de conformité, livrée en premier ; conformément à la politique de
> versionnage, le numéro de ce milestone sera fixé à sa livraison.

| Langue | Code | Statut |
|---|---|---|
| Français | `fr` | ✅ Défaut actuel |
| Deutsch | `de` | Planifié |
| Italiano | `it` | Planifié |
| English | `en` | Partiel (chaînes UI) |

**Périmètre**
- Traduction complète : menus, formulaires, libellés, messages d'erreur, gabarits de factures
- Bulletin de paiement QR : libellés créancier/débiteur dans la langue choisie
- PDF factures : langue liée au paramètre société ou par facture
- Wizard premier démarrage : langue détectée depuis la locale Windows
- Sélecteur de langue dans la barre de navigation

**Plan technique**
- `react-i18next` en frontend, fichiers `public/locales/{fr,de,it,en}/translation.json`
- Backend : génération PDF language-aware (en-tête facture, libellés QR-bill)
- NSIS : packs DE, FR, EN déjà présents — ajouter IT

---

## Planifié

Priorités arrêtées après audit de conformité (juil. 2026), révisées en août 2026
après l'audit du flux offre de prix → facture. L'ordre reflète un principe
simple : ce qui fait qu'un indépendant suisse choisit LedgerAlps plutôt qu'une
solution cloud, c'est une comptabilité **complète**, des données qu'il ne peut
**pas perdre**, et des données que **personne d'autre ne détient**.

> **Statuts.** ✅ **livré** = testé et validé par l'utilisateur. 🔎 **en
> validation** = codé, CI verte, publié dans une pré-version, en attente de
> validation. Le code écrit ne suffit pas : la recherche IDE répondait 403 à
> tout le monde, l'archive ARM64 n'avait pas de serveur, le compteur « en
> retard » affichait toujours zéro — chaque fois les tests passaient, mais le
> chemin réellement emprunté par l'utilisateur n'avait jamais été parcouru.

| Priorité | Fonctionnalité | Description |
|---|---|---|
| 1 | **Factures fournisseurs & charges** | ✅ Backend livré (API + TVA déductible). L'**impôt préalable** alimente désormais le chiffre 400 de la déclaration : auparavant figé à zéro, la TVA due était surévaluée. Reste : **interface de saisie**, écriture au journal à la comptabilisation (charge + TVA déductible / créanciers), notes de frais, pièces jointes (scan de facture). |
| 2 | **Sauvegarde & restauration** | ✅ CLI et instantané automatique (v1.3.14, validé).<br>✅ **Interface (v1.4.4, validé)** : onglet Sauvegardes dans Paramètres — créer un instantané, saisir la phrase de passe de chiffrement, lister les copies et leur état. Restauration avec avertissement explicite : elle est *préparée* puis appliquée au démarrage suivant, un serveur ne pouvant pas échanger sous lui le fichier qu'il a ouvert. Bouton **« Redémarrer LedgerAlps maintenant »** pour l'appliquer sans fermer la fenêtre à la main. Annulable. La comptabilité remplacée est sauvegardée **à la préparation**, avec la même phrase de passe — donc chiffrée si vos sauvegardes le sont.<br>Reste : sauvegarde vers un dossier externe (NAS / clé USB), test de restauration planifié.
| 3 | **Limitation des tentatives de connexion** | ✅ Livré en v1.3.14. Reste : journalisation des verrouillages dans l'audit trail. |
| 4 | **Chiffrement au repos — sauvegardes ✅, base bloquée** | ✅ **Sauvegardes chiffrées (v1.4.4, validé).** Argon2id + XChaCha20-Poly1305, en Go pur. `BACKUP_PASSPHRASE` ou `--passphrase`. Ce sont elles qui *quittent* la machine (NAS, clé USB) et échappent au contrôle de l'utilisateur ; un instantané égaré exposait l'intégralité de la comptabilité et des données clients (nLPD art. 8, OPDo art. 1-6). La copie en clair n'est effacée qu'après relecture, déchiffrement et contrôle d'intégrité SQLite. Le chiffrement est authentifié : altération, troncature et réordonnancement sont refusés. La phrase de passe doit être distincte du mot de passe de session, sinon perdre le poste revient à perdre aussi les sauvegardes.<br>**Base SQLite chiffrée : bloqué par l'architecture.** SQLCipher est une bibliothèque **C**. Le projet compile avec `CGO_ENABLED=0` sur un pilote SQLite en Go pur (`modernc.org/sqlite`), ce qui donne la compilation croisée et le binaire unique sans dépendance système. L'adopter imposerait CGO, donc la fin des deux. Options réelles, à arbitrer : (a) accepter CGO et un toolchain de compilation croisée par plateforme ; (b) chiffrer au niveau applicatif les seules colonnes sensibles, au prix de la recherche et des tris sur ces colonnes ; (c) s'en remettre au chiffrement de disque (BitLocker, LUKS) et le documenter comme la mesure attendue. En l'état c'est (c) qui s'applique, faute de décision. L'avis `nlpd-art8-data-security` affiché dans l'application reste donc pertinent pour la base, mais plus pour les sauvegardes. |
| 5 | **HTTPS natif pour l'accès réseau** | 🔎 **Livré en v1.4.5 — accès local validé, accès réseau en attente de validation.** Le serveur écoute par défaut sur `127.0.0.1` : jusqu'ici il écoutait sur **toutes** les interfaces, si bien qu'un portable sur un réseau public servait sa comptabilité en clair sans que personne l'ait choisi.<br>Rendre LedgerAlps joignable depuis un autre poste (`HOST`) impose désormais TLS : certificat fourni via `TLS_CERT`/`TLS_KEY`, sinon **auto-signé généré** dans `<données applicatives>/tls` — couvrant `localhost`, le nom de la machine et ses adresses IP, valable dix ans et réutilisé d'un démarrage à l'autre pour que l'exception accordée au navigateur tienne.<br>`ALLOW_INSECURE_HTTP=true` reste possible pour le seul cas légitime — un reverse proxy terminant TLS sur la même machine — et le journalise bruyamment, parce que c'est aussi le drapeau vers lequel on se tourne pour faire taire un avertissement.<br>Sans cela, mot de passe de connexion, jeton de session et phrase de passe de sauvegarde traversaient le réseau en clair (LPD art. 8, OPDo art. 3 al. 1 let. c).<br>Une option servant aussi en HTTPS sur `localhost` a été proposée en pré-version puis **retirée** : aucune sécurité réelle gagnée — le trafic ne quitte pas la machine — pour un avertissement de certificat à chaque nouveau profil de navigateur. Dépenser la confiance dans les avertissements sans rien protéger est un mauvais échange ; le raisonnement est dans [`docs/PRODUCTION.md`](docs/PRODUCTION.md#pourquoi-http-sur-localhost).<br>Reste : renouvellement automatique du certificat auto-signé à l'approche de l'échéance, et HSTS une fois qu'un certificat de confiance est la norme. |
| 6 | **Rotation du secret de signature** | 🔎 **Livré, en attente de validation.** Bouton **« Régénérer la clé de signature »** dans Paramètres → Maintenance → Sécurité, réservé aux administrateurs. La confirmation énonce la portée exacte plutôt qu'un avertissement vague : « êtes-vous sûr ? » n'aide personne à décider.<br>**Portée, vérifiée dans le code** : le secret ne sert qu'à signer et relire les jetons. Le régénérer déconnecte toutes les sessions, **et rien d'autre** — mots de passe (bcrypt) intacts, aucune donnée touchée, sauvegardes toujours utilisables (elles ne contiennent pas `config.json`, et leur chiffrement dérive d'une phrase de passe indépendante). Les jetons de rafraîchissement en base sont révoqués au passage, pour que la table cesse de décrire des sessions qui n'existent plus.<br>À utiliser en cas de suspicion de fuite du fichier de configuration : partagé dans un ticket de support, copié sur une clé, poussé par erreur dans un dépôt. Qui le détient forge un jeton valide pour n'importe quel compte, administrateur compris, sans connaître aucun mot de passe.<br>**Ne remplace pas le chiffrement du disque** : qui lit `config.json` lit aussi `ledgeralps.db`, posé dans le même dossier — il n'a alors nul besoin de forger un jeton.
| 7 | **Paramètres → Maintenance & Système** | ✅ **Contrôle de cohérence et état du système (v1.4.5, validés).**<br>🔎 **Piste d'audit, conformité & clôture, réversibilité, sécurité — livrés, en attente de validation.**<br><br>**Piste d'audit** — l'écran expose la chaîne d'empreintes du CO art. 957a et permet de **vérifier la chaîne entière**. Vérifier une entrée isolée ne détecte que la modification de son contenu ; supprimer une ligne laisse l'empreinte de toutes les autres valide. Le parcours contrôle aussi le chaînage et la continuité des numéros, et nomme la rupture. Limite énoncée : une troncature en **fin** de chaîne est indétectable par construction, c'est la sauvegarde qui répond.<br>⚠️ Ce travail a révélé que **l'empreinte ne couvrait pas les valeurs enregistrées** : aucune écriture ne pouvait se revérifier depuis l'origine. Corrigé ; les entrées antérieures sont signalées non vérifiables, jamais altérées.<br><br>**Conformité & clôture** — exercices comptables (déclaration d'un exercice décalé, clôture avec ses conséquences énoncées) et **verrouillage de période** (CO art. 958f, Olico art. 3) : un exercice clos refuse création **et** comptabilisation, y compris pour un brouillon créé avant la clôture.<br>⚠️ Trois défauts enchaînés trouvés ici, vérifiés sur un serveur réel : `fiscal_year_id` n'était renseigné nulle part ; la clôture, qui filtre dessus, marquait l'exercice clos **sans produire d'écriture de clôture** en répondant `200 closed` ; et cette écriture, insérée directement en `posted`, échappait à la chaîne d'intégrité. Corrigés, avec rattrapage des bases existantes au démarrage.<br><br>**Archivage Olico art. 9** — attestation d'intégrité exportable : état de la chaîne, empreinte de tête, périmètre couvert, et **ses limites** — l'horodatage vient de l'horloge du poste, pas d'une autorité tierce (RFC 3161), ce qui établit l'ordre des enregistrements mais pas une date opposable.<br><br>**Export de réversibilité** — l'archive légale contient désormais un dossier `csv/` en plus du JSON : séparateur point-virgule et BOM UTF-8 pour Excel en Suisse, lignes imbriquées extraites dans leurs propres fichiers avec clé étrangère, et un LISEZ-MOI documentant les relations. Le JSON était exact mais supposait d'écrire du code ; sans format ouvrable dans un tableur, « vos données vous appartiennent » restait sans moyen de l'exercer.<br><br>**QR-facture** — la validation SIX IG v2.4 existait sans écran. Elle est désormais exercée par le contrôle de cohérence : IBAN de la société absent ou invalide, adresse structurée incomplète, IBAN de contacts invalides. Silencieux quand il n'y a rien à dire.<br><br>**Sécurité** — voir le point 6.<br><br>Reste à livrer :<br><br>**1. Mode bac à sable** — duplication vers un environnement de test. ⚠️ Exige un marquage permanent et impossible à ignorer : le risque n'est pas technique mais humain — facturer un vrai client depuis le bac à sable, ou l'inverse. Non commencé.<br><br>**2. Console de rejeu ISO 20022 / eBill** — remappage `camt.053/.054`, `pain.001`. L'import et l'export existent côté backend ; la console de rejeu n'est pas commencée.<br><br>**3. Console de logs** — l'état du système est livré (moteur, volumétrie, sauvegardes, réseau, chiffrement du disque, capacités). La consultation des erreurs API demande une capture de journal qui n'existe pas encore ; non commencée.<br><br>**4. TVA AFC** — taux 8.1 / 2.6 / 3.8 % et méthodes effective / TDFN sont en place ; la transmission **e-TVA** ne l'est pas.<br><br>**5. Purge et nLPD** — anonymisation des contacts et règles de rétention : traité au point « Suppression & droit à l'effacement ».<br><br>⚠️ **Le recalcul des soldes reste volontairement absent.** Cet écran montre, il ne répare pas : une comptabilité incohérente se corrige par une écriture, pas par un bouton — réparer en silence effacerait la trace (CO art. 957a al. 2 ch. 5). La réindexation ne touchera jamais aux numéros de documents émis.
| 8 | **Notes de crédit — trace comptable et lien** | ✅ **Lien vers la facture corrigée (v1.4.4, validé)** : la note référence la facture, son PDF porte « Annule la facture : FA-… » (LTVA art. 27 al. 4, CO art. 957a al. 2 ch. 5), et le **montant est borné** — la somme des notes rattachées ne peut plus dépasser la facture, les crédits partiels s'additionnant contre le même plafond.<br>**Reste : l'écriture au journal — mais elle ne peut pas commencer ici.** Aucune facture ne passe d'écriture aujourd'hui : les ventes se saisissent manuellement au journal, seuls les paiements sont automatisés (banque / débiteurs, code 1020 / 1100). Contrepasser automatiquement le produit d'une note de crédit créerait donc un produit négatif sans contrepartie, et doublerait la correction si l'utilisateur l'a déjà passée lui-même. L'automatisation doit être prise dans l'ordre : d'abord la facture à l'émission (débiteurs / produits + TVA due, comptes 1100 / 3000-3200 / 2261), la note de crédit suivant alors le même mécanisme en sens inverse. À traiter avec l'écriture au journal des factures fournisseurs (voir « Factures fournisseurs & charges »). |
| 9 | **Suppression & droit à l'effacement (nLPD)** | Endpoints `DELETE` (seules les routes logo et facture fournisseur brouillon en disposent). Effacement d'un contact avec conservation des pièces comptables exigées par le CO art. 958f — anonymisation plutôt que suppression physique. |
| 10 | Multi-utilisateurs & Permissions | Rôles Admin / Comptable / Lecture seule. Cas d'usage central en Suisse : donner un accès **lecture seule à la fiduciaire** sans partager le compte admin. |
| 11 | Rapprochement bancaire UI | L'import camt.053 existe déjà côté backend mais aucune interface ne permet de l'exploiter. Matching visuel contre le journal, workflow « matcher & passer ». |
| 12 | **eBill** (remplace ZUGFeRD / Factur-X) | Réseau e-facturation suisse opéré par SIX, adopté par la majorité des banques CH. Plus pertinent pour une PME suisse que ZUGFeRD / Factur-X, orientés marché européen. |
| 13 | **Validation QR-facture contre le portail officiel SIX** | SIX exploite un [portail de validation](https://validation.iso-payments.ch/qrrechnung) qui contrôle le *payload* **et** l'image rendue contre les Implementation Guidelines. C'est la seule vérification faisant autorité, et celle qui manque : nos tests vérifient notre lecture de la spécification, pas la conformité réelle du bulletin produit. À faire : soumettre un PDF de référence à chaque évolution du format, et documenter le résultat.<br>**Sur le logo « QR-Ready »** — non retenu en l'état. `qr-ready.ch` est associé à Epsitec SA (éditeur de Crésus, logiciel concurrent), n'est pas un programme SIX, ne documente aucun processus de certification, et la page QR-Ready d'Epsitec redirige aujourd'hui vers une 404. Afficher ce logo reviendrait à adopter la marque d'un concurrent sans validation vérifiable derrière. La validation SIX apporte la substance ; si un label officiel apparaît un jour, la veille de conformité le signalera. |
| 14 | **Veille de conformité automatisée** | ✅ Livré en v1.4.0. Surveillance hebdomadaire des sources faisant autorité (Fedlex SPARQL pour nLPD/OPDo/LTVA/CO, SIX pour la QR-facture, EUR-Lex pour le RGPD) ; une évolution ouvre une issue, un mainteneur rédige l'avis en citant la source. Avis affichés dans l'application via un flux embarqué (fonctionne hors ligne). Boucle fermée par la **vérification de mise à jour** : la veille prévient l'équipe, l'équipe publie, l'application invite l'utilisateur à installer. Le flux distant signé Ed25519 reste implémenté et testé mais **volontairement non branché** — voir le raisonnement dans `compliance/README.md`. Voir [`compliance/README.md`](compliance/README.md). |

**Écarté** — *Mobile / PWA* : incompatible avec l'architecture. LedgerAlps est un
binaire local écouté sur `localhost` ; une « saisie hors-ligne avec sync »
suppose un serveur central que le produit n'a pas, et ne veut pas avoir.

> Les numéros de version des milestones planifiés seront attribués à la livraison, pas à l'avance.

---

## Plateformes prises en charge

**Windows x86-64** et **Linux x86-64**. Rien d'autre.

| Plateforme | Livrable | Statut |
|---|---|---|
| Windows x86-64 | `LedgerAlps_Setup_*.exe` (+ archive `.zip`) | ✅ Pris en charge |
| Linux x86-64 | archive `.tar.gz` | ✅ Pris en charge (paquets `.deb`/`.rpm` abandonnés en v1.4.5) |
| macOS (Intel et Apple Silicon) | — | ❌ Abandonné après v1.4.1 |
| Windows / Linux ARM64 | — | ❌ Abandonné après v1.4.1 |

**Pourquoi cet abandon.** Ces binaires étaient publiés parce que la compilation
croisée en Go ne coûte rien, pas parce qu'ils étaient testés : il n'existe ni
machine macOS ni machine ARM dans la CI ni dans le projet. Ils étaient donc
livrés sans qu'aucune vérification n'ait été possible.

L'archive `windows_arm64` a rendu le coût concret : elle ne contenait que
`ledgeralps-cli.exe`, sans serveur ni lanceur — soit un outil en ligne de
commande sans rien à piloter. Quiconque téléchargeait le fichier portant le nom
de son architecture obtenait un paquet inutilisable, et ce depuis la v1.3.15 au
moins. Personne ne s'en est aperçu, précisément parce que personne ne testait.

Publier un binaire, c'est promettre qu'il fonctionne. Deux plateformes tenues
valent mieux que six supposées.

**Paquets Linux abandonnés en v1.4.5**, pour la même raison à plus petite
échelle : l'empaquetage `.deb`/`.rpm` suppose de suivre les conventions de
plusieurs distributions, et personne ne le vérifiait sur une vraie machine.
L'archive `.tar.gz` reste publiée. **Linux reste une plateforme de test** — la
CI tourne sur Ubuntu, c'est là que `go test -race` s'exécute (impossible en
local, faute de compilateur C) et là que les assertions de permissions de
fichiers ont un sens, Windows ignorant les bits Unix. Abandonner l'empaquetage
ne coûte aucune couverture ; abandonner Linux en coûterait beaucoup.

**Conséquences pratiques**
- **PC Windows ARM** (Surface, portables Snapdragon) : utiliser l'installeur
  x86-64, que Windows exécute par émulation.
- **macOS, Linux ARM, Raspberry Pi** : compiler depuis les sources
  (`make build`, Go 1.26+). Le code reste portable ; c'est la *publication*
  d'artefacts non testés qui s'arrête, pas la compatibilité.
- `scripts/install.sh` refuse désormais ces plateformes avec un message
  explicite, au lieu de tenter un téléchargement qui répondrait 404.

Une plateforme sera réintégrée le jour où elle sera testée en CI, pas avant.

---

## Complété

| Version | Fonctionnalité | Date |
|---|---|---|
| v0.1.0 | Backend FastAPI, SQLAlchemy, modèles, API REST complète | avr. 2026 |
| v1.0.0 | Réécriture Go — moteur comptable double-entrée, JWT, hash chain SHA-256, CO art. 957 | avr. 2026 |
| v1.1.0 | ISO 20022 pain.001 / camt.053, export légal ZIP, dashboard stats | avr. 2026 |
| v1.1.1 | Lanceur Windows (`-H=windowsgui`), wizard premier démarrage, config JSON, frontend embarqué (`go:embed`) | avr. 2026 |
| v1.2.0 | Pipeline release GoReleaser + NSIS, CLI (`migrate`, `bootstrap`, `health`), endpoints reports / payments / audit-logs | avr. 2026 |
| v1.3.0 | Logo société — sidebar, PDF, upload settings | avr. 2026 |
| v1.3.1–v1.3.11 | PDF QR-bill : encodage Latin-1, conformité SPC 0200, layout BillLayout.java, suppression Swico S1, validation IBAN, avertissements UI | avr. 2026 |
| v1.3.12 | CHE auto-fill ZEFIX, notification réinstallation, dialogue NSIS suppression données | avr. 2026 |
| v1.3.13 | **Fix QR-bill SPC 0200 v2.3** : remplacement type adresse K→S (type K retiré en v2.3), croix suisse restaurée (`image/draw`), séparation NPA/localité pour adresse structurée | avr. 2026 |
| v1.3.14 | **Conformité QR-facture IG v2.4** : appariement référence/compte imposé (QRR ⇄ QR-IBAN, SCOR/NON ⇄ IBAN standard), QRR restreinte au CHF, validation SCOR ISO 11649 (mod 97-10), IBAN CH/LI 21 car. — **sauvegarde & restauration** (snapshot `VACUUM INTO`, rétention, CLI `backup`/`backups`/`restore`, snapshot auto au démarrage) — **limitation des tentatives de connexion** (verrouillage par IP) | juil. 2026 |
| v1.3.15 | **Factures fournisseurs** : API + lignes multi-taux, garde anti-doublon `UNIQUE(fournisseur, référence)` — l'**impôt préalable** alimente enfin le chiffre 400 de la déclaration TVA (figé à zéro auparavant : la TVA due était surévaluée) — **fix installeur** : BOM UTF-8 manquant (NSIS lisait le script en codepage ANSI → « donnÃ©es »), chaînes désormais localisées EN/FR — **fix tableau de bord** : la carte année fiscale interrogeait des colonnes inexistantes (`label`/`status`) et échouait à chaque appel — journalisation des verrouillages de connexion (`security_events`) | juil. 2026 |
| v1.4.0 | **Veille de conformité** : surveillance hebdomadaire de Fedlex (SPARQL), SIX et EUR-Lex ; avis affichés dans l'application depuis un flux embarqué (hors ligne) ; **vérification de mise à jour** désactivable, sans identifiant ni télémétrie — suppression du code Python hérité et refonte complète de la documentation | juil. 2026 |
| v1.4.3 | **Flux offre de prix → facture** : une offre entrait dans la déclaration TVA et sortait en PDF titré « FACTURE » avec bulletin QR (LTVA art. 27 al. 2, art. 40) — corrigé ; conversion offre → facture conservant les deux documents et les reliant (CO art. 957a al. 2 ch. 5, art. 958f al. 3) ; une offre ne peut plus être « payée » ; notes de crédit signées dans la déclaration (LTVA art. 41) | août 2026 |
| v1.4.2 | **Plateformes publiées réduites à Windows x86-64 et Linux x86-64** — macOS et ARM abandonnés faute de machine de test (voir [Plateformes prises en charge](#plateformes-prises-en-charge)) — **fix** : l'archive `windows_arm64` ne contenait que le CLI, sans serveur ni lanceur, depuis la v1.3.15 — **fix** : le dossier d'installation survivait à la désinstallation, des reliquats d'avant la v1.1.1 faisant échouer le `RMDir` en silence — rattrapage des entrées 1.4.0 et 1.4.1 du CHANGELOG, livré dans chaque archive | août 2026 |
| v1.4.1 | **Assistant de premier démarrage** : recherche IDE réparée (l'endpoint utilisé répondait 403 à tout le monde ; implémentation unifiée dans `internal/core/zefix`, elle existait en double) et enregistrement des données société corrigé — **sécurité** : jeton de rafraîchissement en cookie HttpOnly, déconnexion révoquée côté serveur, plus aucun appel à Google Fonts — bcrypt sorti du budget base de données (Bootstrap, Login) — bannières de conformité repliées, messages de validation, libellés d'accessibilité | juil. 2026 |
