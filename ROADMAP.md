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
| 2 | **Sauvegarde & restauration** | ✅ CLI et instantané automatique (v1.3.14, validé).<br>🔎 **Interface (v1.4.4-rc3)** : onglet Sauvegardes dans Paramètres — créer un instantané, saisir la phrase de passe de chiffrement, lister les copies et leur état. Restauration avec avertissement explicite : elle est *préparée* puis appliquée au démarrage suivant, un serveur ne pouvant pas échanger sous lui le fichier qu'il a ouvert. Bouton **« Redémarrer LedgerAlps maintenant »** pour l'appliquer sans fermer la fenêtre à la main. Annulable. La comptabilité remplacée est sauvegardée **à la préparation**, avec la même phrase de passe — donc chiffrée si vos sauvegardes le sont.<br>Reste : sauvegarde vers un dossier externe (NAS / clé USB), test de restauration planifié.
| 3 | **Limitation des tentatives de connexion** | ✅ Livré en v1.3.14. Reste : journalisation des verrouillages dans l'audit trail. |
| 4 | **Chiffrement au repos — sauvegardes 🔎, base bloquée** | 🔎 **Sauvegardes chiffrées (v1.4.4-rc1, en validation).** Argon2id + XChaCha20-Poly1305, en Go pur. `BACKUP_PASSPHRASE` ou `--passphrase`. Ce sont elles qui *quittent* la machine (NAS, clé USB) et échappent au contrôle de l'utilisateur ; un instantané égaré exposait l'intégralité de la comptabilité et des données clients (nLPD art. 8, OPDo art. 1-6). La copie en clair n'est effacée qu'après relecture, déchiffrement et contrôle d'intégrité SQLite. Le chiffrement est authentifié : altération, troncature et réordonnancement sont refusés. La phrase de passe doit être distincte du mot de passe de session, sinon perdre le poste revient à perdre aussi les sauvegardes.<br>**Base SQLite chiffrée : bloqué par l'architecture.** SQLCipher est une bibliothèque **C**. Le projet compile avec `CGO_ENABLED=0` sur un pilote SQLite en Go pur (`modernc.org/sqlite`), ce qui donne la compilation croisée et le binaire unique sans dépendance système. L'adopter imposerait CGO, donc la fin des deux. Options réelles, à arbitrer : (a) accepter CGO et un toolchain de compilation croisée par plateforme ; (b) chiffrer au niveau applicatif les seules colonnes sensibles, au prix de la recherche et des tris sur ces colonnes ; (c) s'en remettre au chiffrement de disque (BitLocker, LUKS) et le documenter comme la mesure attendue. En l'état c'est (c) qui s'applique, faute de décision. L'avis `nlpd-art8-data-security` affiché dans l'application reste donc pertinent pour la base, mais plus pour les sauvegardes. |
| 5 | **HTTPS natif pour l'accès réseau** | Aujourd'hui LedgerAlps n'écoute qu'en HTTP. **Sur un poste unique, ce n'est pas un problème** : le trafic vers `localhost` ne quitte jamais la machine et n'apparaît sur aucun câble — Wireshark sur le réseau ne voit rien. **Le risque est réel dès qu'on déploie sur un serveur du bureau**, cas documenté dans [`docs/PRODUCTION.md`](docs/PRODUCTION.md) : mot de passe de connexion, jeton JWT et désormais **phrase de passe de chiffrement des sauvegardes** traversent alors le réseau en clair, lisibles par quiconque écoute le segment.<br>La parade documentée est un reverse proxy (nginx, Caddy) qui porte le certificat. Elle fonctionne, mais suppose une compétence système que le produit prétend justement ne pas exiger, et rien n'empêche aujourd'hui de démarrer le serveur en écoute sur `0.0.0.0` sans proxy.<br>À faire : écoute TLS native (`TLS_CERT` / `TLS_KEY`), génération d'un certificat auto-signé au premier démarrage en réseau local, et **refus de servir en clair sur une interface non-loopback** sans opt-in explicite — une erreur de configuration ne doit pas exposer silencieusement des identifiants. |
| 6 | **Paramètres → Maintenance & Système** | Regrouper en cinq sections l'outillage d'exploitation. Une bonne moitié **existe déjà sans être exposée** — il s'agit d'abord de la rendre atteignable.<br><br>**1. Intégrité & données** — contrôle de cohérence (débit/crédit, auxiliaires, doublons) ; recalcul des soldes et A-Nouveaux. ⚠️ **La réindexation ne touchera pas aux numéros de documents émis** : renuméroter une facture déjà envoyée contredit la traçabilité du CO art. 957a al. 2 ch. 5 et laisse le client citant une référence disparue.<br><br>**2. Sauvegardes & environnement** — sauvegardes et restauration ✅ (v1.4.4) ; mode bac à sable (duplication vers un environnement de test) ; export de réversibilité (JSON, SQL, CSV) — l'export ZIP légal existe déjà, à étendre. ⚠️ Le bac à sable exige un marquage permanent et impossible à ignorer : le risque n'est pas technique mais humain — facturer un vrai client depuis le bac à sable, ou l'inverse.<br><br>**3. Conformité & clôture** — verrouillage de période (la clôture d'exercice existe) ; TVA AFC : taux 8.1 / 2.6 / 3.8 %, méthode effective ou TDFN, e-TVA ; **archivage [Olico](https://www.fedlex.admin.ch/eli/cc/2002/216/fr) (RS 221.431, *GeBüV* en allemand)** — son art. 9 autorise les supports **modifiables** à condition que des procédés techniques garantissent l'intégrité et horodatent l'enregistrement. C'est exactement ce que fait déjà la chaîne d'empreintes SHA-256 des écritures (CO art. 957a) : il reste à le **prouver** — scellement, horodatage vérifiable, export d'attestation — et à couvrir la conservation dix ans (CO art. 958f) ; purge et nLPD (anonymisation des contacts, règles de rétention — voir « Suppression & droit à l'effacement »).<br><br>**4. Flux & banques (SIX / ISO 20022)** — contrôle QR-IBAN, QRR et SCOR (la validation existe, à exposer) ; console de rejeu ISO 20022 / eBill et remappage `camt.053/.054`, `pain.001`.<br><br>**5. Diagnostic & traçabilité** — piste d'audit (`audit_logs` existe, l'écran manque) ; console de logs et santé (erreurs API, état de la base, état de la veille).<br><br>À découper en plusieurs livraisons : rendre visible l'existant d'abord, construire le neuf ensuite. |
| 7 | **Notes de crédit — trace comptable et lien** | 🔎 **Lien vers la facture corrigée (v1.4.4-rc1, en validation)** : la note référence la facture, son PDF porte « Annule la facture : FA-… » (LTVA art. 27 al. 4, CO art. 957a al. 2 ch. 5), et le **montant est borné** — la somme des notes rattachées ne peut plus dépasser la facture, les crédits partiels s'additionnant contre le même plafond.<br>**Reste : l'écriture au journal — mais elle ne peut pas commencer ici.** Aucune facture ne passe d'écriture aujourd'hui : les ventes se saisissent manuellement au journal, seuls les paiements sont automatisés (banque / débiteurs, code 1020 / 1100). Contrepasser automatiquement le produit d'une note de crédit créerait donc un produit négatif sans contrepartie, et doublerait la correction si l'utilisateur l'a déjà passée lui-même. L'automatisation doit être prise dans l'ordre : d'abord la facture à l'émission (débiteurs / produits + TVA due, comptes 1100 / 3000-3200 / 2261), la note de crédit suivant alors le même mécanisme en sens inverse. À traiter avec l'écriture au journal des factures fournisseurs (voir « Factures fournisseurs & charges »). |
| 8 | **Suppression & droit à l'effacement (nLPD)** | Endpoints `DELETE` (seules les routes logo et facture fournisseur brouillon en disposent). Effacement d'un contact avec conservation des pièces comptables exigées par le CO art. 958f — anonymisation plutôt que suppression physique. |
| 9 | Multi-utilisateurs & Permissions | Rôles Admin / Comptable / Lecture seule. Cas d'usage central en Suisse : donner un accès **lecture seule à la fiduciaire** sans partager le compte admin. |
| 10 | Rapprochement bancaire UI | L'import camt.053 existe déjà côté backend mais aucune interface ne permet de l'exploiter. Matching visuel contre le journal, workflow « matcher & passer ». |
| 11 | **eBill** (remplace ZUGFeRD / Factur-X) | Réseau e-facturation suisse opéré par SIX, adopté par la majorité des banques CH. Plus pertinent pour une PME suisse que ZUGFeRD / Factur-X, orientés marché européen. |
| 12 | **Validation QR-facture contre le portail officiel SIX** | SIX exploite un [portail de validation](https://validation.iso-payments.ch/qrrechnung) qui contrôle le *payload* **et** l'image rendue contre les Implementation Guidelines. C'est la seule vérification faisant autorité, et celle qui manque : nos tests vérifient notre lecture de la spécification, pas la conformité réelle du bulletin produit. À faire : soumettre un PDF de référence à chaque évolution du format, et documenter le résultat.<br>**Sur le logo « QR-Ready »** — non retenu en l'état. `qr-ready.ch` est associé à Epsitec SA (éditeur de Crésus, logiciel concurrent), n'est pas un programme SIX, ne documente aucun processus de certification, et la page QR-Ready d'Epsitec redirige aujourd'hui vers une 404. Afficher ce logo reviendrait à adopter la marque d'un concurrent sans validation vérifiable derrière. La validation SIX apporte la substance ; si un label officiel apparaît un jour, la veille de conformité le signalera. |
| 13 | **Veille de conformité automatisée** | ✅ Livré en v1.4.0. Surveillance hebdomadaire des sources faisant autorité (Fedlex SPARQL pour nLPD/OPDo/LTVA/CO, SIX pour la QR-facture, EUR-Lex pour le RGPD) ; une évolution ouvre une issue, un mainteneur rédige l'avis en citant la source. Avis affichés dans l'application via un flux embarqué (fonctionne hors ligne). Boucle fermée par la **vérification de mise à jour** : la veille prévient l'équipe, l'équipe publie, l'application invite l'utilisateur à installer. Le flux distant signé Ed25519 reste implémenté et testé mais **volontairement non branché** — voir le raisonnement dans `compliance/README.md`. Voir [`compliance/README.md`](compliance/README.md). |

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
| Linux x86-64 | `.deb`, `.rpm` (+ archive `.tar.gz`) | ✅ Pris en charge |
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
