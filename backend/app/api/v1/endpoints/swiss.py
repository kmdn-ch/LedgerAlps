"""LedgerAlps — Endpoints Phase 3 : QR-facture, ISO 20022, PDF, exports."""

from datetime import date
from io import BytesIO
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Query
from fastapi.responses import Response, StreamingResponse
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.api.deps import get_current_user
from app.db.session import get_db
from app.models import Contact, FiscalYear, Invoice, User
from app.services.exports.accounting import ExportService
from app.services.swiss_standards.iso20022_pain001 import (
    Pain001Generator, Pain001Party, Pain001PaymentGroup, Pain001Transaction,
)
from app.services.swiss_standards.qr_invoice import (
    QRAddress, QRInvoiceData, QRInvoiceGenerator, QRReferenceGenerator,
)

from pydantic import BaseModel
from decimal import Decimal


# ─── QR-facture ───────────────────────────────────────────────────────────────

qr_router = APIRouter(prefix="/qr-invoice", tags=["qr-invoice"])


class QRGenerateRequest(BaseModel):
    invoice_id: UUID
    qr_iban: str
    reference_type: str = "QRR"   # QRR, SCOR, NON
    creditor_name: str
    creditor_address: str = ""
    creditor_postal_code: str = ""
    creditor_city: str = ""
    creditor_country: str = "CH"


@qr_router.post("/generate-payload")
async def generate_qr_payload(
    payload: QRGenerateRequest,
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> dict:
    """Génère le payload texte du QR code Swiss QR-Bill."""
    invoice = (await db.execute(
        select(Invoice).options(selectinload(Invoice.lines)).where(Invoice.id == payload.invoice_id)
    )).scalar_one_or_none()

    if not invoice:
        raise HTTPException(status_code=404, detail="Facture introuvable.")

    reference = invoice.qr_reference or ""
    if payload.reference_type == "QRR" and not reference:
        reference = QRReferenceGenerator.generate_qrr(invoice.number)

    try:
        qr_data = QRInvoiceData(
            creditor_iban=payload.qr_iban,
            creditor=QRAddress(
                name=payload.creditor_name,
                address_type="S",
                street_or_address_line1=payload.creditor_address,
                postal_code=payload.creditor_postal_code,
                city=payload.creditor_city,
                country=payload.creditor_country,
            ),
            amount=invoice.total,
            currency=invoice.currency,
            reference_type=payload.reference_type,
            reference=reference,
            unstructured_message=invoice.payment_info or f"Facture {invoice.number}",
        )
        qr_txt = QRInvoiceGenerator.generate_payload(qr_data)
        return {
            "payload": qr_txt,
            "reference": reference,
            "reference_formatted": QRReferenceGenerator.format_qrr_display(reference),
            "amount": str(invoice.total),
            "currency": invoice.currency,
        }
    except Exception as e:
        raise HTTPException(status_code=422, detail=str(e))


@qr_router.get("/image/{invoice_id}")
async def get_qr_image(
    invoice_id: UUID,
    qr_iban: str = Query(...),
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> Response:
    """Retourne l'image PNG du QR code (avec croix suisse)."""
    invoice = (await db.execute(
        select(Invoice).where(Invoice.id == invoice_id)
    )).scalar_one_or_none()

    if not invoice:
        raise HTTPException(status_code=404, detail="Facture introuvable.")

    try:
        qr_data = QRInvoiceData(
            creditor_iban=qr_iban,
            creditor=QRAddress(name="Créancier", address_type="S",
                               postal_code="0000", city="Ville"),
            amount=invoice.total,
            currency=invoice.currency,
            reference_type="NON",
        )
        payload = QRInvoiceGenerator.generate_payload(qr_data)
        image_bytes = QRInvoiceGenerator.generate_qr_image(payload)
        return Response(content=image_bytes, media_type="image/png")
    except Exception as e:
        raise HTTPException(status_code=422, detail=str(e))


@qr_router.post("/reference/generate-qrr")
async def generate_qrr_reference(
    customer_ref: str = Query(..., description="Référence client (ex: FA2025-0001)"),
    participant_id: str = Query(default="000000000"),
    _: User = Depends(get_current_user),
) -> dict:
    ref = QRReferenceGenerator.generate_qrr(customer_ref, participant_id)
    return {
        "reference": ref,
        "reference_formatted": QRReferenceGenerator.format_qrr_display(ref),
        "type": "QRR",
    }


# ─── ISO 20022 ────────────────────────────────────────────────────────────────

iso_router = APIRouter(prefix="/iso20022", tags=["iso20022"])


class Pain001Request(BaseModel):
    debtor_name: str
    debtor_iban: str
    debtor_bic: str | None = None
    execution_date: date
    payments: list[dict]  # [{"creditor_name", "iban", "amount", "currency", "reference", ...}]


@iso_router.post("/pain001")
async def generate_pain001(
    payload: Pain001Request,
    _: User = Depends(get_current_user),
) -> Response:
    """Génère un fichier XML pain.001.001.09 pour virement bancaire groupé."""
    try:
        gen = Pain001Generator(initiating_party_name=payload.debtor_name)

        debtor = Pain001Party(name=payload.debtor_name, iban=payload.debtor_iban, bic=payload.debtor_bic)
        transactions = []

        for i, p in enumerate(payload.payments):
            transactions.append(Pain001Transaction(
                end_to_end_id=p.get("end_to_end_id", f"NOTPROVIDED-{i:04d}"),
                creditor=Pain001Party(
                    name=p["creditor_name"],
                    iban=p["iban"],
                    bic=p.get("bic"),
                ),
                amount=Decimal(str(p["amount"])),
                currency=p.get("currency", "CHF"),
                remittance_info=p.get("message"),
                structured_ref=p.get("reference"),
            ))

        group = Pain001PaymentGroup(
            payment_info_id=f"PG-{payload.execution_date.strftime('%Y%m%d')}-001",
            debtor=debtor,
            requested_execution_date=payload.execution_date.strftime("%Y-%m-%d"),
            transactions=transactions,
        )
        gen.add_group(group)
        xml_bytes = gen.generate_xml()
        xml_hash = gen.generate_hash(xml_bytes)

        filename = f"pain001_{payload.execution_date.strftime('%Y%m%d')}.xml"
        return Response(
            content=xml_bytes,
            media_type="application/xml",
            headers={
                "Content-Disposition": f'attachment; filename="{filename}"',
                "X-SHA256": xml_hash,
            },
        )
    except Exception as e:
        raise HTTPException(status_code=422, detail=str(e))


@iso_router.post("/camt053/parse")
async def parse_camt053(
    _: User = Depends(get_current_user),
) -> dict:
    """
    Endpoint pour upload et parsing d'un fichier camt.053.
    Implémentation complète en Phase 4 (avec upload multipart).
    """
    return {"message": "Upload de fichiers camt.053 disponible en Phase 4."}


# ─── PDF ──────────────────────────────────────────────────────────────────────

pdf_router = APIRouter(prefix="/pdf", tags=["pdf"])


@pdf_router.get("/invoice/{invoice_id}")
async def download_invoice_pdf(
    invoice_id: UUID,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
) -> StreamingResponse:
    """Génère et retourne le PDF de la facture."""
    invoice = (await db.execute(
        select(Invoice)
        .options(selectinload(Invoice.lines), selectinload(Invoice.contact))
        .where(Invoice.id == invoice_id)
    )).scalar_one_or_none()

    if not invoice:
        raise HTTPException(status_code=404, detail="Facture introuvable.")

    from app.services.invoicing.pdf import InvoicePDFService

    svc = InvoicePDFService()
    contact = invoice.contact

    # Données company — à remplacer par la config réelle en Phase 4
    company = {
        "name": "Votre Entreprise SA",
        "address_line1": "Rue de la Paix 1",
        "postal_code": "1000",
        "city": "Lausanne",
        "country": "CH",
        "uid_number": "CHE-123.456.789",
        "email": "info@votre-entreprise.ch",
        "iban": invoice.qr_iban or "",
    }

    contact_dict = {
        "name": contact.name,
        "address_line1": contact.address_line1 or "",
        "address_line2": contact.address_line2 or "",
        "postal_code": contact.postal_code or "",
        "city": contact.city or "",
        "country": contact.country,
        "uid_number": contact.vat_number or "",
    }

    pdf_bytes = svc.generate_pdf(invoice, company, contact_dict)

    return StreamingResponse(
        BytesIO(pdf_bytes),
        media_type="application/pdf",
        headers={"Content-Disposition": f'attachment; filename="facture_{invoice.number}.pdf"'},
    )


# ─── Exports comptables ───────────────────────────────────────────────────────

export_router = APIRouter(prefix="/exports", tags=["exports"])


@export_router.get("/trial-balance")
async def export_trial_balance(
    as_of: date | None = Query(default=None),
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> Response:
    svc = ExportService(db)
    csv_bytes = await svc.export_trial_balance_csv(as_of=as_of)
    filename = f"balance_{(as_of or date.today()).strftime('%Y%m%d')}.csv"
    return Response(content=csv_bytes, media_type="text/csv",
                    headers={"Content-Disposition": f'attachment; filename="{filename}"'})


@export_router.get("/general-ledger")
async def export_general_ledger(
    start_date: date = Query(...),
    end_date: date = Query(...),
    account_number: str | None = None,
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> Response:
    svc = ExportService(db)
    csv_bytes = await svc.export_general_ledger_csv(start_date, end_date, account_number)
    filename = f"grand_livre_{start_date.strftime('%Y%m%d')}_{end_date.strftime('%Y%m%d')}.csv"
    return Response(content=csv_bytes, media_type="text/csv",
                    headers={"Content-Disposition": f'attachment; filename="{filename}"'})


@export_router.get("/journal")
async def export_journal(
    start_date: date = Query(...),
    end_date: date = Query(...),
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> Response:
    svc = ExportService(db)
    csv_bytes = await svc.export_journal_csv(start_date, end_date)
    filename = f"journal_{start_date.strftime('%Y%m%d')}_{end_date.strftime('%Y%m%d')}.csv"
    return Response(content=csv_bytes, media_type="text/csv",
                    headers={"Content-Disposition": f'attachment; filename="{filename}"'})


@export_router.post("/legal-archive/{fiscal_year_id}")
async def create_legal_archive(
    fiscal_year_id: UUID,
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> StreamingResponse:
    """Génère l'archive légale CO (ZIP avec manifest d'intégrité)."""
    svc = ExportService(db)
    zip_bytes, manifest = await svc.create_legal_archive(fiscal_year_id)
    return StreamingResponse(
        BytesIO(zip_bytes),
        media_type="application/zip",
        headers={
            "Content-Disposition": f'attachment; filename="archive_co_{fiscal_year_id}.zip"',
            "X-Manifest-Hash": manifest.get("manifest_hash", ""),
        },
    )
