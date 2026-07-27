# LedgerAlps — Roadmap

> **Politique de versionnage**
> - `vX.Y.0` — livraison d'un milestone fonctionnel complet
> - `vX.Y.Z` (Z > 0) — correctifs groupés dans le cycle du milestone, taggés une fois stables
> - On ne pose **jamais** un tag par commit isolé

---

## En cours — v1.4 : Interface multilingue (FR / DE / IT / EN)

La Suisse compte quatre langues officielles. LedgerAlps supportera FR, DE, IT et EN.

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
| 2 | **Sauvegarde & restauration** | ✅ Livré en v1.3.14 (CLI + snapshot automatique). Reste : UI de restauration, sauvegarde vers un dossier externe (NAS / clé USB), test de restauration planifié. |
| 3 | **Limitation des tentatives de connexion** | ✅ Livré en v1.3.14. Reste : journalisation des verrouillages dans l'audit trail. |
| 4 | **Chiffrement au repos** | Base SQLite chiffrée (SQLCipher ou équivalent). Aujourd'hui les données clients sont en clair sur le disque : un portable volé est une violation nLPD annonçable. Argument différenciant face au cloud. |
| 5 | **Suppression & droit à l'effacement (nLPD)** | Endpoints `DELETE` (seules les routes logo et facture fournisseur brouillon en disposent). Effacement d'un contact avec conservation des pièces comptables exigées par le CO art. 958f — anonymisation plutôt que suppression physique. |
| 6 | Multi-utilisateurs & Permissions | Rôles Admin / Comptable / Lecture seule. Cas d'usage central en Suisse : donner un accès **lecture seule à la fiduciaire** sans partager le compte admin. |
| 7 | Rapprochement bancaire UI | L'import camt.053 existe déjà côté backend mais aucune interface ne permet de l'exploiter. Matching visuel contre le journal, workflow « matcher & passer ». |
| 8 | **eBill** (remplace ZUGFeRD / Factur-X) | Réseau e-facturation suisse opéré par SIX, adopté par la majorité des banques CH. Plus pertinent pour une PME suisse que ZUGFeRD / Factur-X, orientés marché européen. |
| 9 | **Veille de conformité automatisée** | 🧪 En validation sur `test`. Surveillance hebdomadaire des sources faisant autorité (Fedlex SPARQL pour nLPD/OPDo/LTVA/CO, SIX pour la QR-facture, EUR-Lex pour le RGPD) ; une évolution ouvre une issue, un mainteneur rédige l'avis en citant la source. Avis affichés dans l'application via un flux embarqué (fonctionne hors ligne). Reste : **signature Ed25519 du flux distant** (clé de publication + rafraîchissement opt-in) pour atteindre les binaires déjà installés, et sélecteur de langue lié à v1.4. Voir [`compliance/README.md`](compliance/README.md). |

**Écarté** — *Mobile / PWA* : incompatible avec l'architecture. LedgerAlps est un
binaire local écouté sur `localhost` ; une « saisie hors-ligne avec sync »
suppose un serveur central que le produit n'a pas, et ne veut pas avoir.

> Les numéros de version des milestones planifiés seront attribués à la livraison, pas à l'avance.

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
