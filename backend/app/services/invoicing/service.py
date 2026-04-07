"""
LedgerAlps — Service de facturation
Gère le cycle de vie complet : devis → facture → paiement → archivage.
Génère automatiquement les écritures comptables à la validation.
"""

from __future__ import annotations

from datetime import date, datetime, timezone
from decimal import Decimal
from uuid import UUID

from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.models import Contact, DocumentStatus, Invoice, InvoiceLine, VATMethod
from app.services.accounting.journal import JournalService
from app.services.vat.calculator import VATCalculator, VATIncluded, VATCalcMethod


class InvoiceError(Exception):
    pass


class InvalidTransitionError(InvoiceError):
    pass


# ─── Machine d'états ─────────────────────────────────────────────────────────

ALLOWED_TRANSITIONS: dict[DocumentStatus, list[DocumentStatus]] = {
    DocumentStatus.DRAFT: [DocumentStatus.SENT, DocumentStatus.CANCELLED],
    DocumentStatus.SENT: [DocumentStatus.PAID, DocumentStatus.OVERDUE, DocumentStatus.CANCELLED],
    DocumentStatus.OVERDUE: [DocumentStatus.PAID, DocumentStatus.CANCELLED],
    DocumentStatus.PAID: [DocumentStatus.ARCHIVED],
    DocumentStatus.CANCELLED: [],
    DocumentStatus.ARCHIVED: [],
}


class InvoiceService:

    def __init__(self, db: AsyncSession) -> None:
        self.db = db
        self.journal = JournalService(db)

    async def create_invoice(
        self,
        *,
        contact_id: UUID,
        issue_date: date,
        due_date: date | None = None,
        lines_data: list[dict],
        document_type: str = "invoice",
        vat_method: VATMethod = VATMethod.EFFECTIVE,
        qr_iban: str | None = None,
        payment_info: str | None = None,
        notes: str | None = None,
        terms: str | None = None,
        user_id: UUID,
    ) -> Invoice:
        """Crée une nouvelle facture en brouillon."""
        number = await self._next_number(document_type, issue_date)

        invoice = Invoice(
            number=number,
            document_type=document_type,
            status=DocumentStatus.DRAFT,
            contact_id=contact_id,
            issue_date=issue_date,
            due_date=due_date or self._compute_due_date(issue_date),
            vat_method=vat_method,
            qr_iban=qr_iban,
            payment_info=payment_info,
            notes=notes,
            terms=terms,
        )

        lines = self._build_lines(lines_data)
        invoice.lines = lines
        self._compute_totals(invoice)

        self.db.add(invoice)
        await self.db.flush()
        return invoice

    async def transition(
        self,
        invoice_id: UUID,
        target_status: DocumentStatus,
        user_id: UUID,
        payment_date: date | None = None,
        payment_amount: Decimal | None = None,
    ) -> Invoice:
        """
        Effectue une transition d'état sur la facture.
        Génère les écritures comptables lors du passage en SENT (comptabilisation).
        """
        invoice = await self._get(invoice_id)
        current = invoice.status

        if target_status not in ALLOWED_TRANSITIONS.get(current, []):
            raise InvalidTransitionError(
                f"Transition {current} → {target_status} non autorisée."
            )

        invoice.status = target_status

        # Comptabilisation à l'envoi (méthode de la facturation — CO)
        if target_status == DocumentStatus.SENT:
            await self._post_to_journal(invoice, user_id)

        elif target_status == DocumentStatus.PAID:
            await self._record_payment(invoice, user_id, payment_date, payment_amount)

        await self.db.flush()
        return invoice

    async def add_line(self, invoice_id: UUID, line_data: dict) -> InvoiceLine:
        """Ajoute une ligne à une facture en brouillon."""
        invoice = await self._get(invoice_id)
        if invoice.status != DocumentStatus.DRAFT:
            raise InvoiceError("Seules les factures en brouillon peuvent être modifiées.")

        position = len(invoice.lines) + 1
        line = self._build_line(line_data, position)
        invoice.lines.append(line)
        self._compute_totals(invoice)
        return line

    # ─── Comptabilisation ─────────────────────────────────────────────────────

    async def _post_to_journal(self, invoice: Invoice, user_id: UUID) -> None:
        """
        Génère l'écriture comptable standard d'une facture client :
          Débit  1100 Créances clients  →  TTC
          Crédit 3201 TVA collectée     →  TVA
          Crédit 6100 Prestations       →  HT
        """
        if invoice.total == Decimal("0"):
            return

        lines = []

        # Débit client (TTC)
        lines.append({
            "debit_account": "1100",
            "credit_account": None,
            "amount": invoice.total,
            "description": f"Facture {invoice.number}",
        })

        # Crédit TVA collectée
        if invoice.vat_amount > Decimal("0"):
            lines.append({
                "debit_account": None,
                "credit_account": "3201",
                "amount": invoice.vat_amount,
                "description": f"TVA — Facture {invoice.number}",
                "vat_code": "N81",
                "vat_amount": invoice.vat_amount,
            })

        # Crédit produit HT
        lines.append({
            "debit_account": None,
            "credit_account": "6100",
            "amount": invoice.subtotal,
            "description": f"Prestations — Facture {invoice.number}",
        })

        entry = await self.journal.create_entry(
            entry_date=invoice.issue_date,
            description=f"Facture {invoice.number}",
            lines=lines,
            reference=f"FAC-{invoice.number}",
            user_id=user_id,
            source_document_id=invoice.id,
            source_document_type="invoice",
        )
        await self.journal.post_entry(entry.id, user_id)

    async def _record_payment(
        self,
        invoice: Invoice,
        user_id: UUID,
        payment_date: date | None,
        payment_amount: Decimal | None,
    ) -> None:
        """
        Écriture de paiement reçu :
          Débit  1020 Banque  →  montant reçu
          Crédit 1100 Créances clients
        """
        amount = payment_amount or invoice.total
        invoice.amount_paid += amount
        pdate = payment_date or date.today()

        lines = [
            {"debit_account": "1020", "credit_account": None, "amount": amount,
             "description": f"Paiement facture {invoice.number}"},
            {"debit_account": None, "credit_account": "1100", "amount": amount,
             "description": f"Paiement facture {invoice.number}"},
        ]

        entry = await self.journal.create_entry(
            entry_date=pdate,
            description=f"Paiement facture {invoice.number}",
            lines=lines,
            reference=f"PAY-{invoice.number}",
            user_id=user_id,
            source_document_id=invoice.id,
            source_document_type="payment",
        )
        await self.journal.post_entry(entry.id, user_id)

    # ─── Calculs ──────────────────────────────────────────────────────────────

    def _build_lines(self, lines_data: list[dict]) -> list[InvoiceLine]:
        return [self._build_line(ld, i + 1) for i, ld in enumerate(lines_data)]

    def _build_line(self, ld: dict, position: int) -> InvoiceLine:
        qty = Decimal(str(ld.get("quantity", "1")))
        price = Decimal(str(ld["unit_price"]))
        discount = Decimal(str(ld.get("discount_percent", "0"))) / 100
        vat_rate = Decimal(str(ld.get("vat_rate", "8.1")))

        base = (qty * price * (1 - discount)).quantize(Decimal("0.01"))
        vat_line = VATCalculator.compute_line(base, vat_rate, VATIncluded.EXCLUDED)

        return InvoiceLine(
            position=position,
            description=ld["description"],
            quantity=qty,
            unit=ld.get("unit"),
            unit_price=price,
            discount_percent=Decimal(str(ld.get("discount_percent", "0"))),
            vat_rate=vat_rate,
            vat_amount=vat_line.vat_amount,
            line_total=vat_line.total_amount,
        )

    def _compute_totals(self, invoice: Invoice) -> None:
        invoice.subtotal = sum(l.line_total - l.vat_amount for l in invoice.lines).quantize(Decimal("0.01"))
        invoice.vat_amount = sum(l.vat_amount for l in invoice.lines).quantize(Decimal("0.01"))
        invoice.total = invoice.subtotal + invoice.vat_amount

    def _compute_due_date(self, issue_date: date) -> date:
        from datetime import timedelta
        return issue_date + timedelta(days=30)

    async def _get(self, invoice_id: UUID) -> Invoice:
        result = await self.db.execute(
            select(Invoice)
            .options(selectinload(Invoice.lines))
            .where(Invoice.id == invoice_id)
        )
        inv = result.scalar_one_or_none()
        if not inv:
            raise InvoiceError(f"Facture introuvable : {invoice_id}")
        return inv

    async def _next_number(self, doc_type: str, issue_date: date) -> str:
        """Numérotation séquentielle annuelle — CO : continu, sans trou."""
        prefix = {"invoice": "FA", "quote": "OF", "credit_note": "NC"}.get(doc_type, "FA")
        year = issue_date.year
        count = (await self.db.execute(
            select(func.count(Invoice.id)).where(
                Invoice.document_type == doc_type,
                func.extract("year", Invoice.issue_date) == year,
            )
        )).scalar() or 0
        return f"{prefix}{year}-{count + 1:04d}"
