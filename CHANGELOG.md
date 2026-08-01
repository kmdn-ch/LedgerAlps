# Changelog

Toutes les modifications notables de LedgerAlps sont documentées ici.  
Format : [Keep a Changelog](https://keepachangelog.com/fr/1.0.0/) — Versioning : [SemVer](https://semver.org/lang/fr/).

---

## [Unreleased]

### Corrigé
- **Une offre de prix était comptée dans la déclaration TVA.** La table `invoices` héberge aussi les offres et les notes de crédit, et la déclaration ne filtrait pas `document_type` : une offre passée au statut « envoyée » — le geste naturel quand on l'adresse à un prospect — entrait au chiffre 200 avec sa TVA. L'entreprise déclarait et payait de la TVA sur un chiffre d'affaires jamais réalisé. Sous LTVA art. 40 al. 1 let. a, la dette d'impôt naît « au moment de la facturation » ; une offre n'est pas une facture
- **Une offre de prix produisait un PDF intitulé « FACTURE » avec bulletin QR.** Le générateur ignorait `document_type` : le prospect recevait un document payable, portant le taux et le montant de TVA ainsi que le numéro IDE, indiscernable d'une facture. Il pouvait le payer et en déduire l'impôt préalable — or LTVA art. 27 al. 2 rend redevable celui qui fait figurer l'impôt sans en avoir le droit. Le titre suit désormais le type de document et le bulletin QR n'est imprimé que sur une facture
- **Une note de crédit augmentait la TVA due** au lieu de la réduire : les montants sont stockés sans signe. Elle est exclue de la déclaration en attendant un traitement correct (voir Limitations connues)
- **Les offres étaient comptées comme créances client** dans la balance âgée et sur le tableau de bord — un devis envoyé à un prospect n'est dû par personne

### Limitations connues
- **Notes de crédit et TVA** : désormais exclues de la déclaration. C'est conservateur — la TVA est surévaluée plutôt que sous-évaluée — mais incorrect : une note de crédit doit réduire la dette d'impôt (LTVA art. 41). Un traitement juste demande des montants signés
- **Aucune conversion offre → facture** : il faut ressaisir la facture à la main

### Corrigé (installeur)
- **Page de licence de l'installeur** : « Copyright (c) 2024–2026 » s'affichait « 2024â€"2026 ». Le fichier `LICENSE` contenait un tiret demi-cadratin en UTF-8, or NSIS ne lit un fichier de licence en UTF-8 que s'il porte un BOM — sans quoi il retombe sur la codepage ANSI. Le caractère est remplacé par un tiret ASCII plutôt que d'ajouter un BOM, qui perturberait les détecteurs de licence (GitHub licensee, scanners SPDX) et partirait aussi dans les paquets `.deb` et `.rpm`

### Modifié
- Le contrôle d'encodage de l'installeur vérifie désormais **`LICENSE` en plus de `installer.nsi`**, et s'exécute **avant** la compilation. Il ne vivait que dans le workflow de répétition : le vrai workflow de publication n'en avait aucun

---

## [1.4.2] — 2026-08-01

### Modifié
- **Plateformes publiées réduites à Windows x86-64 et Linux x86-64.** macOS et toutes les variantes ARM sont abandonnées : ces binaires étaient produits parce que la compilation croisée Go ne coûte rien, pas parce qu'ils étaient testés — il n'existe aucune machine macOS ni ARM dans la CI. Sur un PC Windows ARM, l'installeur x86-64 fonctionne par émulation ; pour macOS ou ARM, le projet reste compilable depuis les sources
- `scripts/install.sh` refuse macOS et ARM en indiquant la raison, au lieu de tenter un téléchargement qui répondrait 404

### Corrigé
- **L'archive `windows_arm64` ne contenait que `ledgeralps-cli.exe`** — ni serveur ni lanceur. Quiconque téléchargeait le fichier portant le nom de son architecture obtenait un outil en ligne de commande sans rien à piloter. Présent depuis la v1.3.15 au moins, passé inaperçu faute de test. Résolu par la suppression de la cible
- **Le dossier d'installation survivait à la désinstallation.** Les installations antérieures à la v1.1.1 livraient l'interface en fichiers séparés (`index.html`, `assets\`) ; les mises à jour ne les ont jamais retirés, et le `RMDir` non récursif du désinstalleur — volontairement prudent, pour ne jamais effacer un fichier déposé par l'utilisateur — échouait donc en silence. Ces reliquats sont désormais nettoyés à l'installation comme à la désinstallation

### Documentation
- Nouvelle section « Plateformes prises en charge » dans la roadmap, avec le raisonnement de l'abandon
- Rattrapage des entrées 1.4.0 et 1.4.1, absentes de ce fichier alors qu'il est livré dans chaque archive

---

## [1.4.1] — 2026-08-01

### Corrigé
- **Recherche IDE de l'assistant de premier démarrage** : l'endpoint interrogé répondait 403 à tout le monde, et non à cause d'une saisie erronée. La recherche passe désormais par la route publique du registre. Le code existait en double — serveur et lanceur — si bien que corriger un seul côté laissait l'assistant cassé ; implémentation unifiée dans `internal/core/zefix`
- Le code postal était perdu en silence (champ JSON mal nommé)
- Les données société saisies dans l'assistant sont bien enregistrées et visibles dans Paramètres ; en cas d'échec, l'assistant le signale au lieu d'annoncer un succès
- Une panne du registre n'affiche plus de code HTTP : le message invite à saisir les informations manuellement

### Sécurité
- Jeton de rafraîchissement déplacé dans un cookie **HttpOnly + SameSite=Strict**, inaccessible au JavaScript de la page
- La déconnexion révoque le jeton côté serveur
- Suppression de tout appel à Google Fonts : ouvrir l'application ne signale plus son usage à un tiers
- bcrypt sorti du budget de la transaction base de données (Bootstrap, Login)

### Modifié
- Bannières de conformité repliables (~40 px au lieu de ~180)
- Messages de validation et libellés d'accessibilité

---

## [1.4.0] — 2026-07-28

### Ajouté
- **Veille de conformité automatisée** — surveillance hebdomadaire de Fedlex (SPARQL) pour nLPD, OPDo, LTVA et CO, de SIX pour les Implementation Guidelines QR-facture, et d'EUR-Lex pour le RGPD. Une évolution ouvre une issue ; l'avis est **rédigé par un humain avec citation de la source**, jamais généré automatiquement
- **Avis de conformité dans l'application**, servis depuis un flux embarqué dans le binaire — fonctionne hors ligne
- **Vérification de mise à jour** — seul appel réseau sortant du produit, sans identifiant ni télémétrie, résultat mis en cache 24 h, échec silencieux, désactivable par `UPDATE_CHECK=false`

### Documentation
- Suppression du code Python/FastAPI, de `docker-compose.yml` et du script Inno Setup orphelin
- Réécriture d'`ARCHITECTURE.md` et `PRODUCTION.md` ; README recentré sur l'utilisateur

---

## [1.3.16-rc1] — 2026-07-27

> Pré-version publiée depuis la branche `test` pour validation.

### Ajouté
- **Veille de conformité automatisée** — surveillance hebdomadaire des sources faisant autorité : Fedlex (SPARQL) pour nLPD RS 235.1, OPDo 235.11, LTVA 641.20 et CO 220 ; SIX pour les Implementation Guidelines QR-facture ; EUR-Lex pour le RGPD. Une évolution ouvre une issue ; l'avis est ensuite **rédigé par un humain avec citation de la source** — jamais généré automatiquement
- **Avis de conformité dans l'application** — bannière affichant les évolutions qui concernent l'utilisateur, servie depuis un flux embarqué dans le binaire (fonctionne hors ligne). `GET /api/v1/compliance/advisories`
- **Répétition de release sans publication** — workflow « Release (dry run) » produisant tous les artefacts, y compris l'installeur NSIS, sans créer de release ni de tag

### Modifié
- La CI et le lint s'exécutent désormais sur la branche `test` avec les mêmes contrôles que `main`
- `release.yml` refuse un tag **final** dont le commit n'est pas atteignable depuis `main` ; les pré-versions en sont exemptées

### Documentation
- Suppression du code Python/FastAPI (`backend/`), de `docker-compose.yml` et du script Inno Setup orphelin : remplacés par la réécriture Go depuis la v1.0.0, ni construits ni livrés
- Réécriture de `ARCHITECTURE.md` et `PRODUCTION.md`, qui décrivaient encore la pile Python abandonnée
- README recentré sur l'utilisateur ; contenu technique déplacé vers `docs/`
- Nouveaux documents : `docs/DEVELOPMENT.md`, `docs/API.md`, `docs/BRANCHING.md`

---

## [1.3.15] — 2026-07-27

### Ajouté
- **Factures fournisseurs** — l'impôt préalable alimente enfin le chiffre 400 de la déclaration TVA. Il était figé à zéro : la TVA due était systématiquement surévaluée. Lignes multi-taux, garde anti-doublon `UNIQUE(fournisseur, référence)`
- Journalisation des verrouillages de connexion (`security_events`, endpoint réservé aux administrateurs)

### Corrigé
- **Installeur Windows** — le désinstalleur affichait « donnÃ©es ». Le script était en UTF-8 mais sans BOM, et NSIS retombait alors sur la codepage ANSI du système. Les messages sont désormais localisés EN/FR
- **Tableau de bord** — la carte « année fiscale » interrogeait les colonnes `label`/`status`, inexistantes : la requête échouait à chaque appel et la carte restait vide

---

## [1.3.14] — 2026-07-27

### Corrigé
- **Conformité QR-facture (IG v2.4)** — l'appariement référence/compte n'était pas vérifié, première cause de rejet par les banques. QRR exige désormais un QR-IBAN, SCOR et NON sont refusés sur un QR-IBAN, la référence QRR est restreinte au CHF (nouveauté v2.4), les références SCOR sont validées selon ISO 11649
- Un QR-IBAN ne peut plus retomber silencieusement sur une référence NON lors de la génération du PDF

### Ajouté
- **Sauvegarde et restauration** — instantanés `VACUUM INTO` cohérents sans interruption de service, vérification d'intégrité, instantané automatique au démarrage, commandes CLI `backup` / `backups` / `restore`
- **Limitation des tentatives de connexion** — verrouillage par IP après 5 échecs en 15 minutes

---

## [1.2.0] – [1.3.13] — avril 2026

Pipeline de release (GoReleaser + NSIS), CLI d'administration, endpoints
rapports / paiements / journal d'audit, logo d'entreprise, édition des factures
et devis, auto-remplissage IDE/ZEFIX, et une longue série de corrections de la
QR-facture (encodage Latin-1, layout du bulletin, suppression de Swico S1,
validation IBAN, passage à l'adresse structurée de type S).

Le détail commit par commit est disponible sur la page
[Releases](https://github.com/kmdn-ch/LedgerAlps/releases), générée
automatiquement à chaque publication.

---

## [1.1.1] — 2026-04-09

### Ajouté
- **Lanceur Windows** (`cmd/launcher` → `ledgeralps.exe`, `-H=windowsgui`) — assistant de configuration au premier démarrage : génère le JWT_SECRET via `crypto/rand`, collecte email/nom/mot de passe admin dans le navigateur, écrit `%APPDATA%\LedgerAlps\config.json`, démarre le serveur, bootstrap l'admin, ouvre l'application
- **Config JSON** — `internal/config` lit `%APPDATA%\LedgerAlps\config.json` (Windows) ou `~/.ledgeralps/config.json` en priorité sur les variables d'environnement
- **Frontend statique embarqué** — `ledgeralps-server.exe` sert `dist/` depuis le répertoire d'installation avec fallback SPA (`NoRoute → index.html`)
- **Goreleaser** — build `ledgeralps-launcher` (Windows amd64, `-H=windowsgui`) ajouté au pipeline de release
- **NSIS installer** — réécriture complète : installe le lanceur + frontend `dist\`, supprime l'enregistrement de service Windows, raccourcis pointent sur `ledgeralps.exe`

### Modifié
- `Makefile` — cibles `build-launcher`, `build-windows`, `build-frontend`, `build-installer`, `release`
- `README.md` — section installation Windows, assistant premier démarrage, liste complète des 35 endpoints

### Corrigé
- CI : `noctx` — `http.Get` / `http.Post` remplacés par `http.NewRequestWithContext` + `http.DefaultClient.Do` dans le lanceur
- CI : `build-check` — cross-compilation `cmd/launcher` ajoutée pour détecter les régressions à chaque push

---

## [1.0.0] — 2026-04-08

### Réécriture complète — Backend Go (branche go-rewrite, Sprints 1–7)

#### Ajouté
- **Backend Go** (`gin-gonic/gin`) remplace FastAPI — binaire unique, zéro-config
- **SQLite WAL** embarqué (`modernc.org/sqlite`) + **PostgreSQL** (`pgx/v5`) en production
- **Migrations embed.FS** auto au démarrage — aucun outil externe requis
- **Plan comptable PME suisse** — 88 comptes (CO art. 957) seedés en migration
- **JWT refresh tokens** — `POST /auth/refresh`, `POST /auth/logout` (révocation jti), `POST /auth/register`, `POST /auth/bootstrap` (premier admin one-shot)
- **Hash chain SHA-256** (CO art. 957a) sur toutes les écritures postées — immuabilité garantie
- **PDF factures A4** avec QR payment slip Swiss intégré (`fpdf` + `go-qrcode`)
- **QR-facture SPC 0200** — référence QRR MOD-10 récursif, FormatQRRReference, Swico S1
- **ISO 20022 pain.001.001.09** — export virements (`POST /payments/export`)
- **ISO 20022 camt.053.001.08** — import relevés bancaires (`POST /bank-statements/import`)
- **Clôture exercice fiscal** — FiscalYearService.CloseYear() (CO art. 958)
- **Déclaration TVA** — méthode effective + TDFN (AFC 318/100), taux 2024
- **Export ZIP légal** — `GET /exports/legal-archive` (CO art. 958f, 10 ans) + manifest SHA-256, IBAN masqué nLPD
- **Dashboard stats** — `GET /stats` (créances, journal, comptes actifs, contacts, exercice ouvert)
- **26 endpoints** API v1 documentés
- **44 tests** : 34 unitaires (compliance, security, db) + 10 intégration end-to-end (httptest + SQLite temp)
- **Frontend aligné** — json tags snake_case, intercepteur 401 + refresh queue, `vite build` propre

#### Modifié
- `internal/models/models.go` — json tags snake_case sur tous les champs (breaking change API)
- `frontend/src/api/client.ts` — réécriture complète : silent refresh, endpoints Go
- `frontend/src/types/index.ts` — types alignés backend Go (currency, total_amount, invoice_number)

#### Supprimé
- Backend Python/FastAPI (remplacé par Go)
- Dépendances Alembic, SQLAlchemy, Pydantic

#### Conformité
- CO art. 957–963 : partie double, immuabilité, conservation 10 ans
- nLPD : IBAN masqué dans export légal, données minimales
- TVA CH 2024 : 8.1% / 2.6% / 3.8%, arrondi 0.05 CHF
- QR-facture SPC 0200 (Six-Group)
- ISO 20022 pain.001 / camt.053

---

## [0.1.0] — 2026-04-07

### Ajouté
- **Backend FastAPI** avec SQLAlchemy async et PostgreSQL 16
- **Modèles** : `Account`, `JournalEntry` / `JournalLine`, `Invoice` / `InvoiceLine`, `Contact`, `AuditLog`, `FiscalYear`, `User`
- **Migration Alembic initiale** (`0001_initial`) — toutes les tables et enums PostgreSQL
- **API REST complète** : auth JWT, comptes, journal, factures, contacts, TVA, QR-facture, ISO 20022, exports
- **`GET /api/v1/journal`** — pagination (`page`, `page_size`) + filtres (`date_from`, `date_to`, `status`, `reference`)
- **`GET /api/v1/contacts/{id}`** et **`PATCH /api/v1/contacts/{id}`** — mise à jour partielle
- **Moteur comptable** : partie double, contrepassation, hash SHA-256 chaîné (CO art. 957a)
- **Service de facturation** : cycle draft → sent → paid → archived, écritures auto au journal
- **Calcul TVA** suisse : taux 8.1% / 2.6% / 3.8%, arrondi 0.05 CHF, méthode effective et TDFN
- **QR-facture** : génération payload SPC 0200, référence QRR/RF (Six-Group / STUZZA)
- **ISO 20022** : export pain.001.001.09 (virements), import camt.053.001.08 (relevés)
- **Middleware** : rate limiting, security headers, audit log
- **Frontend React/TypeScript/Tailwind** : Dashboard, Factures, Journal, Contacts, Comptes, Rapports, Paramètres
- **`InvoiceDetailPage`** : détail facture, transitions de statut, aperçu PDF inline
- **Composant `PDFPreview`** : affichage inline avec `<iframe>` + objectURL
- **Tests unitaires** : TVA, arrondi 5 rappen
- **Tests d'intégration** : auth, contacts, TVA, factures (cycle complet), journal (pagination + filtres), PATCH contacts
- **Docker Compose** : PostgreSQL + backend + frontend + Nginx (profil production)
- **`.env.example`** avec toutes les variables documentées
- **README** complet : installation, configuration, commandes `make`, conformité légale

### Conformité légale
- CO art. 957–963 : comptabilité en partie double, immuabilité des écritures postées
- nLPD : local-first, données minimales, Privacy by Design
- TVA CH 2024 : taux 8.1% / 2.6% / 3.8%
- QR-facture Six-Group SPC 0200
- ISO 20022 pain.001 / camt.053–054

---

[Unreleased]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.2...HEAD
[1.4.2]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.1...v1.4.2
[1.4.1]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.0...v1.4.1
[1.4.0]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.3.15...v1.4.0
[1.0.0]: https://github.com/kmdn-ch/LedgerAlps/compare/v0.1.0...v1.0.0
[0.1.0]: https://github.com/kmdn-ch/LedgerAlps/releases/tag/v0.1.0
