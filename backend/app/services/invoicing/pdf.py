"""
LedgerAlps — Service de génération PDF
Utilise Jinja2 pour le rendu HTML et WeasyPrint pour la conversion PDF.
Intègre le QR code Swiss QR-Bill dans le document.
"""

from __future__ import annotations

import base64
import hashlib
import os
from datetime import date, datetime
from decimal import Decimal
from pathlib import Path
from typing import Any

from jinja2 import Environment, FileSystemLoader, select_autoescape

from app.core.config import settings
from app.models import Invoice
from app.services.swiss_standards.qr_invoice import (
    QRAddress, QRInvoiceData, QRInvoiceGenerator, QRReferenceGenerator,
)

TEMPLATES_DIR = Path(__file__).parent.parent.parent / "templates"


class PDFGenerationError(Exception):
    pass


# ─── Filtres Jinja2 ──────────────────────────────────────────────────────────

def _filter_currency(value: Decimal | float | None, currency: str = "CHF") -> str:
    if value is None:
        return f"0.00 {currency}"
    return f"{Decimal(str(value)):,.2f}".replace(",", "'") + f" {currency}"


def _filter_date(value: date | datetime | None, fmt: str = "%d.%m.%Y") -> str:
    if value is None:
        return ""
    if isinstance(value, datetime):
        return value.strftime(fmt)
    return value.strftime(fmt)


def _filter_number(value: Decimal | float | None, decimals: int = 2) -> str:
    if value is None:
        return "0"
    v = Decimal(str(value))
    if v == v.to_integral_value():
        return str(int(v))
    return f"{v:.{decimals}f}".rstrip("0").rstrip(".")


def _filter_iban_format(iban: str | None) -> str:
    """Formate un IBAN avec espaces tous les 4 chiffres."""
    if not iban:
        return ""
    clean = iban.replace(" ", "").upper()
    return " ".join(clean[i:i+4] for i in range(0, len(clean), 4))


def _filter_qrr_format(ref: str | None) -> str:
    """Formate une référence QRR : XX XXXXX XXXXX XXXXX XXXXX XXXXX"""
    if not ref:
        return ""
    from app.services.swiss_standards.qr_invoice import QRReferenceGenerator
    return QRReferenceGenerator.format_qrr_display(ref)


# ─── Moteur de rendu ─────────────────────────────────────────────────────────

class InvoicePDFService:

    def __init__(self) -> None:
        self.env = Environment(
            loader=FileSystemLoader(str(TEMPLATES_DIR)),
            autoescape=select_autoescape(["html", "j2"]),
        )
        # Enregistrer les filtres personnalisés
        self.env.filters["currency"] = _filter_currency
        self.env.filters["date"] = _filter_date
        self.env.filters["number"] = _filter_number
        self.env.filters["iban_format"] = _filter_iban_format
        self.env.filters["qrr_format"] = _filter_qrr_format

    def render_html(
        self,
        invoice: Invoice,
        company: dict,
        contact: dict,
        locale: str = "fr_CH",
    ) -> str:
        """Rend le HTML de la facture (utile pour preview web)."""
        context = self._build_context(invoice, company, contact, locale)
        template = self.env.get_template("invoices/invoice.html.j2")
        return template.render(**context)

    def generate_pdf(
        self,
        invoice: Invoice,
        company: dict,
        contact: dict,
        locale: str = "fr_CH",
        output_path: str | None = None,
    ) -> bytes:
        """Génère le PDF de la facture et retourne les bytes."""
        try:
            from weasyprint import HTML, CSS
        except ImportError:
            raise PDFGenerationError(
                "WeasyPrint non installé : pip install weasyprint"
            )

        html_content = self.render_html(invoice, company, contact, locale)
        pdf_bytes = HTML(string=html_content).write_pdf()

        if output_path:
            Path(output_path).parent.mkdir(parents=True, exist_ok=True)
            Path(output_path).write_bytes(pdf_bytes)

        return pdf_bytes

    def compute_pdf_hash(self, pdf_bytes: bytes) -> str:
        """SHA-256 du PDF pour archivage CO art. 958f."""
        return hashlib.sha256(pdf_bytes).hexdigest()

    def _build_context(
        self,
        invoice: Invoice,
        company: dict,
        contact: dict,
        locale: str,
    ) -> dict[str, Any]:
        # Résumé TVA
        vat_summary: dict[Decimal, dict] = {}
        for line in invoice.lines:
            rate = line.vat_rate
            if rate not in vat_summary:
                from app.services.vat.calculator import rate_to_code
                vat_summary[rate] = {
                    "base": Decimal("0"),
                    "vat": Decimal("0"),
                    "code": rate_to_code(rate),
                }
            vat_summary[rate]["base"] += (line.line_total - line.vat_amount)
            vat_summary[rate]["vat"] += line.vat_amount

        # QR code
        qr_payload = None
        qr_image_b64 = None

        if invoice.qr_iban:
            try:
                qr_data = QRInvoiceData(
                    creditor_iban=invoice.qr_iban,
                    creditor=QRAddress(
                        name=company["name"],
                        address_type="S",
                        street_or_address_line1=company.get("address_line1", ""),
                        postal_code=company.get("postal_code", ""),
                        city=company.get("city", ""),
                        country=company.get("country", "CH"),
                    ),
                    amount=invoice.total,
                    currency=invoice.currency,
                    debtor=QRAddress(
                        name=contact["name"],
                        address_type="S",
                        street_or_address_line1=contact.get("address_line1", ""),
                        postal_code=contact.get("postal_code", ""),
                        city=contact.get("city", ""),
                        country=contact.get("country", "CH"),
                    ) if contact.get("city") else None,
                    reference_type="QRR" if invoice.qr_reference else "NON",
                    reference=invoice.qr_reference or "",
                    unstructured_message=invoice.payment_info or f"Facture {invoice.number}",
                )
                qr_payload = QRInvoiceGenerator.generate_payload(qr_data)
                qr_image_bytes = QRInvoiceGenerator.generate_qr_image(qr_payload)
                qr_image_b64 = base64.b64encode(qr_image_bytes).decode()
            except Exception:
                pass  # QR optionnel — ne bloque pas le PDF

        return {
            "invoice": invoice,
            "company": company,
            "contact": contact,
            "locale": locale,
            "vat_summary": vat_summary,
            "qr_payload": qr_payload,
            "qr_image_b64": qr_image_b64,
            "generated_at": datetime.now(),
        }
