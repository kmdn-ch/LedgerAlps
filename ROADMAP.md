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

| Priorité | Fonctionnalité | Description |
|---|---|---|
| 1 | Mobile / PWA | Manifest PWA, layout responsive, saisie journal hors-ligne avec sync |
| 2 | Multi-utilisateurs & Permissions | Rôles Admin / Comptable / Lecture seule, audit trail, invitation par e-mail |
| 3 | Rapprochement bancaire UI | Matching visuel camt.053 contre journal, workflow « matcher & passer » |
| 4 | E-facturation ZUGFeRD / Factur-X | Factures hybrides PDF+XML, import fournisseurs → écritures auto, conformité eDEF |

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
