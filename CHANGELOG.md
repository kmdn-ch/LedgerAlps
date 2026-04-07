# Changelog

Toutes les modifications notables de LedgerAlps sont documentées ici.  
Format : [Keep a Changelog](https://keepachangelog.com/fr/1.0.0/) — Versioning : [SemVer](https://semver.org/lang/fr/).

---

## [Unreleased]

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

[Unreleased]: https://github.com/kmdn-ch/LedgerAlps/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/kmdn-ch/LedgerAlps/releases/tag/v0.1.0
