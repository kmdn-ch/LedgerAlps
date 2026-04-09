# LedgerAlps — Roadmap

> Versions are indicative. Features move between milestones based on priority.

---

## v1.2 — Smart Setup Wizard (next)

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
- New launcher endpoint `GET /uid-lookup?che=CHE-123.456.789`
- Calls `https://www.uid.admin.ch/app/api/v1/...` (public REST API, no key needed)
- Falls back to scraping `prestations.vd.ch` if UID API is insufficient
- Frontend: debounced input on CHE field → auto-fill on valid match
- All lookups are client-side initiated, no data stored

---

## v1.3 — Mobile / PWA

- Progressive Web App manifest so users can pin to home screen
- Responsive layout for invoice consultation on mobile
- Offline-capable journal entry (sync on reconnect)

---

## v1.4 — Multi-user & Permissions

- Role-based access: Admin / Accountant / Read-only
- Per-user audit trail attribution
- Invite by email (token-based onboarding)

---

## v1.5 — Bank Reconciliation UI

- Visual matching of camt.053 bank entries against journal entries
- One-click "match & post" workflow
- Unmatched entries highlighted for review

---

## v1.6 — E-invoicing (ZUGFeRD / Factur-X)

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
