"""
LedgerAlps — Tests unitaires : génération PDF facture (pdf.py) × QR-facture

Régression Corrections v2 de qr_invoice.py : pdf.py appelait encore l'ancienne
API (QRAddress.street_or_address_line1, QRReferenceGenerator.format_qrr_display).
Le bug était silencieux — InvoicePDFService._build_context avale toute exception
dans son bloc de génération QR (`except Exception: pass`), donc un PDF était
généré sans QR code, sans aucune erreur visible.
"""

from datetime import date
from decimal import Decimal
from uuid import uuid4

import pytest

from app.models import Invoice
from app.services.invoicing.pdf import InvoicePDFService, _filter_qrr_format
from app.services.swiss_standards.qr_invoice import QRReferenceGenerator


def make_invoice(**overrides) -> Invoice:
    defaults = dict(
        number="FA2025-0001",
        document_type="invoice",
        contact_id=uuid4(),
        issue_date=date(2025, 1, 1),
        currency="CHF",
        total=Decimal("1234.55"),
        qr_iban="CH5604835012345678009",
        qr_reference=None,
        payment_info="Facture FA2025-0001",
    )
    defaults.update(overrides)
    invoice = Invoice(**defaults)
    invoice.lines = []
    return invoice


COMPANY = {
    "name": "Votre Entreprise SA",
    "address_line1": "Rue de la Paix 1",
    "postal_code": "1000",
    "city": "Lausanne",
    "country": "CH",
}

CONTACT = {
    "name": "Client SA",
    "address_line1": "Av. de la Gare 5",
    "postal_code": "1003",
    "city": "Lausanne",
    "country": "CH",
}


class TestFilterQrrFormat:

    def test_formats_27_digit_reference_with_spaces(self):
        ref = QRReferenceGenerator.generate_qrr("FA2025-0001").replace(" ", "")
        formatted = _filter_qrr_format(ref)
        assert formatted.replace(" ", "") == ref
        assert formatted.count(" ") == 5

    def test_empty_or_none_ref_returns_empty_string(self):
        assert _filter_qrr_format(None) == ""
        assert _filter_qrr_format("") == ""


class TestBuildContextQrGeneration:

    def test_qr_payload_generated_when_qr_iban_present(self):
        invoice = make_invoice()
        svc = InvoicePDFService()
        ctx = svc._build_context(invoice, COMPANY, CONTACT, "fr_CH")
        assert ctx["qr_payload"] is not None
        assert ctx["qr_payload"].startswith("SPC\n0200\n1\n")

    def test_qr_payload_uses_creditor_and_debtor_street(self):
        invoice = make_invoice()
        svc = InvoicePDFService()
        ctx = svc._build_context(invoice, COMPANY, CONTACT, "fr_CH")
        lines = ctx["qr_payload"].split("\n")
        assert lines[6] == "Rue de la Paix 1"     # rue créancier
        assert lines[22] == "Av. de la Gare 5"    # rue débiteur

    def test_no_qr_payload_when_qr_iban_absent(self):
        invoice = make_invoice(qr_iban=None)
        svc = InvoicePDFService()
        ctx = svc._build_context(invoice, COMPANY, CONTACT, "fr_CH")
        assert ctx["qr_payload"] is None
        assert ctx["qr_image_b64"] is None

    def test_qr_image_generated_when_qrcode_available(self):
        pytest.importorskip("qrcode")
        pytest.importorskip("PIL")
        invoice = make_invoice()
        svc = InvoicePDFService()
        ctx = svc._build_context(invoice, COMPANY, CONTACT, "fr_CH")
        assert ctx["qr_image_b64"] is not None
