"""
LedgerAlps — Tests unitaires : endpoints QR-facture (app/api/v1/endpoints/swiss.py)

Régression Corrections v2 de qr_invoice.py : ces endpoints appelaient encore
l'ancienne API (QRAddress.street_or_address_line1, format_qrr_display).
Pas de base de données réelle ici (Postgres non disponible en environnement
de test unitaire) — la session DB est simulée pour exercer le code exact des
handlers sans dépendre de tests/integration/test_api.py.
"""

from datetime import date
from decimal import Decimal
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest

from app.api.v1.endpoints.swiss import (
    QRGenerateRequest,
    generate_qr_payload,
    generate_qrr_reference,
)
from app.models import Invoice


def make_invoice(**overrides) -> Invoice:
    defaults = dict(
        number="FA2025-0001",
        document_type="invoice",
        contact_id=uuid4(),
        issue_date=date(2025, 1, 1),
        currency="CHF",
        total=Decimal("1234.55"),
        qr_reference=None,
        payment_info="Facture FA2025-0001",
    )
    defaults.update(overrides)
    return Invoice(**defaults)


def make_db_returning(invoice: Invoice | None) -> AsyncMock:
    """Simule AsyncSession.execute(...).scalar_one_or_none() -> invoice."""
    result = MagicMock()
    result.scalar_one_or_none.return_value = invoice
    db = AsyncMock()
    db.execute.return_value = result
    return db


class TestGenerateQrPayloadEndpoint:

    @pytest.mark.asyncio
    async def test_generates_payload_with_qrr_reference(self):
        invoice = make_invoice()
        db = make_db_returning(invoice)
        payload = QRGenerateRequest(
            invoice_id=uuid4(),
            qr_iban="CH4431999123000889012",  # QR-IBAN (IID 31999)
            reference_type="QRR",
            creditor_name="Acme SA",
            creditor_address="Rue de la Paix 1",
            creditor_postal_code="1000",
            creditor_city="Lausanne",
            creditor_country="CH",
        )

        resp = await generate_qr_payload(payload=payload, db=db, _=None)

        assert resp["payload"].startswith("SPC\n0200\n1\n")
        assert resp["payload"].split("\n")[6] == "Rue de la Paix 1"
        # generate_qrr() renvoie déjà une référence formatée (espaces) ;
        # format_display() ré-appliqué dessus est idempotent.
        assert len(resp["reference"].replace(" ", "")) == 27
        assert resp["reference_formatted"] == resp["reference"]

    @pytest.mark.asyncio
    async def test_missing_invoice_returns_404(self):
        from fastapi import HTTPException

        db = make_db_returning(None)
        payload = QRGenerateRequest(
            invoice_id=uuid4(), qr_iban="CH4431999123000889012", creditor_name="Acme SA",
        )

        with pytest.raises(HTTPException) as exc_info:
            await generate_qr_payload(payload=payload, db=db, _=None)
        assert exc_info.value.status_code == 404


class TestGenerateQrrReferenceEndpoint:

    @pytest.mark.asyncio
    async def test_returns_reference_and_formatted_display(self):
        resp = await generate_qrr_reference(
            customer_ref="FA2025-0001", participant_id="210000000", _=None,
        )
        assert resp["type"] == "QRR"
        assert len(resp["reference"].replace(" ", "")) == 27
        assert resp["reference_formatted"] == resp["reference"]
