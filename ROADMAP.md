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

Priorités arrêtées après audit de conformité (juil. 2026). L'ordre reflète un
principe simple : ce qui fait qu'un indépendant suisse choisit LedgerAlps plutôt
qu'une solution cloud, c'est une comptabilité **complète**, des données qu'il ne
peut **pas perdre**, et des données que **personne d'autre ne détient**.

| Priorité | Fonctionnalité | Description |
|---|---|---|
| 1 | **Factures fournisseurs & charges** | ✅ Backend livré (API + TVA déductible). L'**impôt préalable** alimente désormais le chiffre 400 de la déclaration : auparavant figé à zéro, la TVA due était surévaluée. Reste : **interface de saisie**, écriture au journal à la comptabilisation (charge + TVA déductible / créanciers), notes de frais, pièces jointes (scan de facture). |
| 2 | **Sauvegarde & restauration** | ✅ Livré en v1.3.14 (CLI + snapshot automatique). Reste : **chiffrement des sauvegardes** (voir priorité 4), UI de restauration, sauvegarde vers un dossier externe (NAS / clé USB), test de restauration planifié. |
| 3 | **Limitation des tentatives de connexion** | ✅ Livré en v1.3.14. Reste : journalisation des verrouillages dans l'audit trail. |
| 4 | **Chiffrement au repos — base et sauvegardes** | Deux volets distincts, le second étant le plus urgent.<br>**Sauvegardes chiffrées** : ce sont elles qui *quittent* la machine (NAS, clé USB, disque externe) et échappent donc au contrôle de l'utilisateur. Un instantané égaré expose l'intégralité de la comptabilité et des données clients. Chiffrement au moment de l'écriture (âge/XChaCha20 ou équivalent), avec une phrase de passe distincte du mot de passe de session — sans quoi perdre le poste revient à perdre aussi les sauvegardes.<br>**Base SQLite chiffrée** (SQLCipher ou équivalent) : aujourd'hui les données clients sont en clair sur le disque ; un portable volé est une violation nLPD annonçable. Argument différenciant face au cloud.<br>L'avis `nlpd-art8-data-security` affiché dans l'application recommande déjà un support chiffré *en attendant* — c'est cette entrée qui le remplace. |
| 5 | **Suppression & droit à l'effacement (nLPD)** | Endpoints `DELETE` (seules les routes logo et facture fournisseur brouillon en disposent). Effacement d'un contact avec conservation des pièces comptables exigées par le CO art. 958f — anonymisation plutôt que suppression physique. |
| 6 | Multi-utilisateurs & Permissions | Rôles Admin / Comptable / Lecture seule. Cas d'usage central en Suisse : donner un accès **lecture seule à la fiduciaire** sans partager le compte admin. |
| 7 | Rapprochement bancaire UI | L'import camt.053 existe déjà côté backend mais aucune interface ne permet de l'exploiter. Matching visuel contre le journal, workflow « matcher & passer ». |
| 8 | **eBill** (remplace ZUGFeRD / Factur-X) | Réseau e-facturation suisse opéré par SIX, adopté par la majorité des banques CH. Plus pertinent pour une PME suisse que ZUGFeRD / Factur-X, orientés marché européen. |
| 9 | **Validation QR-facture contre le portail officiel SIX** | SIX exploite un [portail de validation](https://validation.iso-payments.ch/qrrechnung) qui contrôle le *payload* **et** l'image rendue contre les Implementation Guidelines. C'est la seule vérification faisant autorité, et celle qui manque : nos tests vérifient notre lecture de la spécification, pas la conformité réelle du bulletin produit. À faire : soumettre un PDF de référence à chaque évolution du format, et documenter le résultat.<br>**Sur le logo « QR-Ready »** — non retenu en l'état. `qr-ready.ch` est associé à Epsitec SA (éditeur de Crésus, logiciel concurrent), n'est pas un programme SIX, ne documente aucun processus de certification, et la page QR-Ready d'Epsitec redirige aujourd'hui vers une 404. Afficher ce logo reviendrait à adopter la marque d'un concurrent sans validation vérifiable derrière. La validation SIX apporte la substance ; si un label officiel apparaît un jour, la veille de conformité le signalera. |
| 10 | **Veille de conformité automatisée** | ✅ Livré en v1.4.0. Surveillance hebdomadaire des sources faisant autorité (Fedlex SPARQL pour nLPD/OPDo/LTVA/CO, SIX pour la QR-facture, EUR-Lex pour le RGPD) ; une évolution ouvre une issue, un mainteneur rédige l'avis en citant la source. Avis affichés dans l'application via un flux embarqué (fonctionne hors ligne). Boucle fermée par la **vérification de mise à jour** : la veille prévient l'équipe, l'équipe publie, l'application invite l'utilisateur à installer. Le flux distant signé Ed25519 reste implémenté et testé mais **volontairement non branché** — voir le raisonnement dans `compliance/README.md`. Voir [`compliance/README.md`](compliance/README.md). |

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
| v1.4.1 | **Assistant de premier démarrage** : recherche IDE réparée (l'endpoint utilisé répondait 403 à tout le monde ; implémentation unifiée dans `internal/core/zefix`, elle existait en double) et enregistrement des données société corrigé — **sécurité** : jeton de rafraîchissement en cookie HttpOnly, déconnexion révoquée côté serveur, plus aucun appel à Google Fonts — bcrypt sorti du budget base de données (Bootstrap, Login) — bannières de conformité repliées, messages de validation, libellés d'accessibilité | juil. 2026 |
