# LedgerAlps — Roadmap

> Versions are indicative. Features move between milestones based on priority.

---

## v1.2 — Smart Setup Wizard & Installer UX (next)

### Auto-fill company info at first launch
On first launch, the setup wizard will auto-populate company fields from public
Swiss registries based on the CHE/IDE number entered by the user.

**Sources**
| Registry | Data | URL |
|---|---|---|
| UID/IDE Register (admin.ch) | Official company name, legal form, CHE number, address, VAT status | `https://www.uid.admin.ch/` (SOAP/REST API) |
| Registre vaudois (prestations.vd.ch) | Cantonal entry, RC number, purpose, date of constitution | `https://prestations.vd.ch/pub/101266/` |

**User flow**
1. User types CHE number (e.g. `CHE-123.456.789`) in the wizard
2. Wizard calls UID API → pre-fills company name, legal form, address, VAT
3. If vaudois registry returns extra detail → pre-fills cantonal fields
4. User reviews, corrects if needed, clicks "Start"

**Technical plan**
- New server endpoint `GET /api/uid-lookup?che=CHE-123.456.789` (proxies to ZEFIX/UID)
- Frontend: debounced input on CHE field → auto-fill on valid match
- All lookups proxied through the Go server to avoid CORS

### Detect and reuse existing data on install
When reinstalling or upgrading, if `%APPDATA%\LedgerAlps\config.json` already
exists, the installer skips the setup wizard and reuses the existing database
and configuration. The user sees a notification: *"Configuration existante
détectée — vos données ont été conservées."*

**Technical plan**
- Launcher checks for `config.json` at startup (already implemented)
- Installer page: detect existing install and display a "mise à niveau" message
- No wizard bypass needed — the current flow already handles this, but the UX
  should make it explicit with a clear "upgrade vs fresh install" message

### Ask to delete data on uninstall
The NSIS uninstaller currently preserves `%APPDATA%\LedgerAlps\` silently.
A confirmation dialog (in French, the primary language) will let the user choose:

> **Souhaitez-vous supprimer vos données comptables ?**
> *(base de données, configuration, journaux)*
>
> [ **Supprimer les données** ]   [ **Conserver les données** ]

If the user clicks "Supprimer", `%APPDATA%\LedgerAlps\` is removed entirely.
If they click "Conserver" (default), only the program files are removed and
the database survives a reinstall.

**Technical plan**
- Add a custom NSIS page with `MessageBox` or custom `nsDialogs` page
- French as primary language; fallback to English if locale detection fails
- Log the user's choice to the Windows event log

---

## v1.3 — Multilingual Interface (DE / IT / FR / EN)

Switzerland has four official languages. LedgerAlps will support all three
main ones plus English.

| Language | Code | Status |
|---|---|---|
| Français | `fr` | ✅ Current default |
| Deutsch | `de` | Planned |
| Italiano | `it` | Planned |
| English | `en` | Partial (UI strings) |

**Scope**
- Full UI translation: menus, forms, labels, error messages, invoice templates
- QR-bill payment slip: creditor/debtor text in the correct language
- PDF invoices: language follows company setting or per-invoice override
- Setup wizard: language auto-detected from Windows locale (`LANG` / `USERPROFILE`)
- Language switcher in the top navigation bar (flag icons)

**Technical plan**
- i18n library in React frontend (e.g. `react-i18next`)
- Translation files: `public/locales/de/translation.json`, etc.
- Backend: language-aware PDF generation (invoice header, QR-bill text)
- NSIS installer: already includes DE, FR, EN language packs — add IT

---

## v1.4 — Mobile / PWA

- Progressive Web App manifest so users can pin to home screen
- Responsive layout for invoice consultation on mobile
- Offline-capable journal entry (sync on reconnect)

---

## v1.5 — Multi-user & Permissions

- Role-based access: Admin / Accountant / Read-only
- Per-user audit trail attribution
- Invite by email (token-based onboarding)

---

## v1.6 — Bank Reconciliation UI

- Visual matching of camt.053 bank entries against journal entries
- One-click "match & post" workflow
- Unmatched entries highlighted for review

---

## v1.7 — E-invoicing (ZUGFeRD / Factur-X)

- Generate hybrid PDF/XML invoices (PDF + embedded XML)
- Import supplier invoices for automatic journal creation
- Swiss eDEF pilot compliance

---

## v2.0 — SaaS / Multi-tenant

- Cloud-hosted option alongside self-hosted
- Per-company data isolation at DB level
- Subscription billing via Stripe

---

## Completed

| Version | Feature |
|---|---|
| v1.0 | Core accounting engine, double-entry, CO compliance |
| v1.1 | Windows installer, setup wizard, QR-bill PDF, ISO 20022 |
| v1.1.5 | Embedded frontend (//go:embed) — eliminates install-dir 404 |
| v1.1.6 | Fix ERR_TOO_MANY_REDIRECTS after setup wizard submit |
