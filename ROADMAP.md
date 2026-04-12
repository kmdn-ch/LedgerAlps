# LedgerAlps — Roadmap

> Les versions sont indicatives. Les fonctionnalités peuvent changer de milestone selon les priorités.

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
- Bulletin de paiement QR : textes créancier/débiteur dans la langue choisie
- PDF factures : langue liée au paramètre société ou par facture
- Wizard premier démarrage : langue détectée depuis la locale Windows
- Sélecteur de langue dans la barre de navigation

**Plan technique**
- `react-i18next` en frontend, fichiers `public/locales/{fr,de,it,en}/translation.json`
- Backend : génération PDF language-aware (en-tête facture, textes QR-bill)
- NSIS : packs DE, FR, EN déjà présents — ajouter IT

---

## v1.5 — Smart Setup Wizard : auto-remplissage depuis les registres suisses

Au premier démarrage, le wizard pré-remplira les champs société depuis les registres publics suisses.

**Sources**
| Registre | Données | URL |
|---|---|---|
| UID/IDE Register (admin.ch) | Raison sociale, forme juridique, CHE, adresse, statut TVA | `https://www.uid.admin.ch/` |
| Registre vaudois (prestations.vd.ch) | Inscription RC, but, date de constitution | `https://prestations.vd.ch/pub/101266/` |

**Flux utilisateur**
1. L'utilisateur saisit son CHE (ex. `CHE-123.456.789`) dans le wizard
2. Le wizard appelle l'API UID → pré-remplit nom, forme juridique, adresse, TVA
3. Si le registre vaudois renvoie des données → pré-remplit les champs cantonaux
4. L'utilisateur vérifie, corrige si besoin, clique « Démarrer »

**Plan technique**
- Endpoint `GET /api/uid-lookup?che=CHE-123.456.789` (proxy vers ZEFIX/UID, évite CORS)
- Frontend : input CHE avec debounce → auto-fill sur match valide

---

## v1.6 — Installer UX : gestion des données à la désinstallation

L'installeur NSIS affichera une boîte de dialogue de confirmation en français lors de la désinstallation :

> **Souhaitez-vous supprimer vos données comptables ?**
> *(base de données, configuration, journaux)*
>
> [ **Supprimer les données** ]   [ **Conserver les données** ]

Si l'utilisateur clique « Supprimer », `%APPDATA%\LedgerAlps\` est effacé.
Si l'utilisateur clique « Conserver » (défaut), seuls les fichiers programme sont supprimés.

**Plan technique**
- Page personnalisée NSIS avec `nsDialogs`
- Français en langue principale ; fallback anglais si détection locale échoue
- Journalisation du choix dans l'event log Windows

---

## v1.7 — Mobile / PWA

- Manifest Progressive Web App (pin sur écran d'accueil)
- Layout responsive pour consultation des factures sur mobile
- Saisie journal en mode hors-ligne avec sync au reconnect

---

## v1.8 — Multi-utilisateurs & Permissions

- Rôles : Admin / Comptable / Lecture seule
- Audit trail par utilisateur
- Invitation par e-mail (onboarding par token)

---

## v1.9 — Rapprochement bancaire UI

- Matching visuel des écritures camt.053 contre le journal
- Workflow « matcher & passer » en un clic
- Écritures non rapprochées mises en évidence

---

## v2.0 — E-facturation (ZUGFeRD / Factur-X)

- Factures hybrides PDF+XML embarqué
- Import de factures fournisseurs → création automatique d'écritures journal
- Conformité pilote eDEF suisse

---

## Complété

| Version | Fonctionnalité | Date |
|---|---|---|
| v0.1.0 | Backend FastAPI, SQLAlchemy, modèles, API REST complète | avr. 2026 |
| v1.0.0 | Réécriture Go — moteur comptable double-entrée, JWT, hash chain SHA-256, CO art. 957 | avr. 2026 |
| v1.1.0 | ISO 20022 pain.001 / camt.053, export légal ZIP, dashboard stats | avr. 2026 |
| v1.1.1 | Lanceur Windows (`-H=windowsgui`), wizard premier démarrage, config JSON, frontend embarqué (go:embed) | avr. 2026 |
| v1.1.5 | Fix 404 frontend après installation — `go:embed dist/` | avr. 2026 |
| v1.1.6 | Fix ERR_TOO_MANY_REDIRECTS après soumission wizard | avr. 2026 |
| v1.2.x | Pipeline release GoReleaser + NSIS, CLI (`migrate`, `bootstrap`, `health`), endpoints reports / payments / audit-logs | avr. 2026 |
| v1.3.x | PDF QR-bill : encodage Latin-1, conformité SPC 0200, layout BillLayout.java, validation IBAN, avertissements UI | avr. 2026 |
