"""LedgerAlps — Routers : factures, comptes, journal, TVA."""

from datetime import date
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Query, status
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.api.deps import get_current_user
from app.db.session import get_db
from app.models import Account, Contact, DocumentStatus, Invoice, JournalEntry, JournalEntryStatus, User, VATMethod
from app.schemas import (
    AccountBalanceResponse, AccountResponse,
    ContactCreate, ContactResponse, ContactUpdate,
    InvoiceCreate, InvoiceResponse, InvoiceStatusUpdate,
    JournalEntryCreate, JournalEntryResponse, JournalPageResponse,
    VATComputeRequest, VATComputeResponse,
)
from app.services.accounting.journal import JournalService
from app.services.invoicing.service import InvoiceService
from app.services.vat.calculator import VATCalculator, VATIncluded


# ─── Comptes ──────────────────────────────────────────────────────────────────

accounts_router = APIRouter(prefix="/accounts", tags=["accounts"])


@accounts_router.get("", response_model=list[AccountResponse])
async def list_accounts(
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> list[Account]:
    result = await db.execute(
        select(Account).where(Account.is_active == True).order_by(Account.number)
    )
    return result.scalars().all()


@accounts_router.get("/{number}/balance", response_model=AccountBalanceResponse)
async def account_balance(
    number: str,
    as_of: date | None = Query(default=None),
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> dict:
    svc = JournalService(db)
    return await svc.get_account_balance(number, as_of=as_of)


@accounts_router.get("/trial-balance", response_model=list[AccountBalanceResponse])
async def trial_balance(
    as_of: date | None = Query(default=None),
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> list[dict]:
    svc = JournalService(db)
    return await svc.get_trial_balance(as_of=as_of)


# ─── Journal ──────────────────────────────────────────────────────────────────

journal_router = APIRouter(prefix="/journal", tags=["journal"])


@journal_router.get("", response_model=JournalPageResponse)
async def list_journal(
    page: int = Query(default=1, ge=1),
    page_size: int = Query(default=20, ge=1, le=100),
    date_from: date | None = Query(default=None),
    date_to: date | None = Query(default=None),
    status: str | None = Query(default=None),
    reference: str | None = Query(default=None),
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> dict:
    q = select(JournalEntry).options(selectinload(JournalEntry.lines))

    if date_from:
        q = q.where(JournalEntry.date >= date_from)
    if date_to:
        q = q.where(JournalEntry.date <= date_to)
    if status:
        q = q.where(JournalEntry.status == JournalEntryStatus(status))
    if reference:
        q = q.where(JournalEntry.reference.ilike(f"%{reference}%"))

    count_q = select(func.count()).select_from(q.subquery())
    total = (await db.execute(count_q)).scalar() or 0

    q = q.order_by(JournalEntry.date.desc(), JournalEntry.reference.desc())
    q = q.offset((page - 1) * page_size).limit(page_size)
    items = (await db.execute(q)).scalars().all()

    pages = (total + page_size - 1) // page_size if total > 0 else 1
    return {"items": items, "total": total, "page": page, "page_size": page_size, "pages": pages}


@journal_router.post("", response_model=JournalEntryResponse, status_code=201)
async def create_entry(
    payload: JournalEntryCreate,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
) -> JournalEntry:
    svc = JournalService(db)
    return await svc.create_entry(
        entry_date=payload.date,
        description=payload.description,
        lines=[l.model_dump() for l in payload.lines],
        reference=payload.reference,
        user_id=current_user.id,
    )


@journal_router.post("/{entry_id}/post", response_model=JournalEntryResponse)
async def post_entry(
    entry_id: UUID,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
) -> JournalEntry:
    svc = JournalService(db)
    return await svc.post_entry(entry_id, current_user.id)


@journal_router.post("/{entry_id}/reverse", response_model=JournalEntryResponse, status_code=201)
async def reverse_entry(
    entry_id: UUID,
    reversal_date: date = Query(...),
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
) -> JournalEntry:
    svc = JournalService(db)
    return await svc.reverse_entry(entry_id, reversal_date, current_user.id)


# ─── Contacts ─────────────────────────────────────────────────────────────────

contacts_router = APIRouter(prefix="/contacts", tags=["contacts"])


@contacts_router.post("", response_model=ContactResponse, status_code=201)
async def create_contact(
    payload: ContactCreate,
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> Contact:
    contact = Contact(**payload.model_dump())
    db.add(contact)
    await db.flush()
    return contact


@contacts_router.get("/{contact_id}", response_model=ContactResponse)
async def get_contact(
    contact_id: UUID,
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> Contact:
    contact = (await db.execute(select(Contact).where(Contact.id == contact_id))).scalar_one_or_none()
    if not contact:
        raise HTTPException(status_code=404, detail="Contact introuvable.")
    return contact


@contacts_router.patch("/{contact_id}", response_model=ContactResponse)
async def update_contact(
    contact_id: UUID,
    payload: ContactUpdate,
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> Contact:
    contact = (await db.execute(select(Contact).where(Contact.id == contact_id))).scalar_one_or_none()
    if not contact:
        raise HTTPException(status_code=404, detail="Contact introuvable.")
    for field, value in payload.model_dump(exclude_unset=True).items():
        setattr(contact, field, value)
    await db.flush()
    return contact


@contacts_router.get("", response_model=list[ContactResponse])
async def list_contacts(
    contact_type: str | None = None,
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> list[Contact]:
    q = select(Contact).where(Contact.is_active == True).order_by(Contact.name)
    if contact_type:
        q = q.where(Contact.contact_type == contact_type)
    return (await db.execute(q)).scalars().all()


# ─── Factures ─────────────────────────────────────────────────────────────────

invoices_router = APIRouter(prefix="/invoices", tags=["invoices"])


@invoices_router.post("", response_model=InvoiceResponse, status_code=201)
async def create_invoice(
    payload: InvoiceCreate,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
) -> Invoice:
    svc = InvoiceService(db)
    return await svc.create_invoice(
        contact_id=payload.contact_id,
        issue_date=payload.issue_date,
        due_date=payload.due_date,
        lines_data=[l.model_dump() for l in payload.lines],
        document_type=payload.document_type,
        vat_method=VATMethod(payload.vat_method),
        qr_iban=payload.qr_iban,
        payment_info=payload.payment_info,
        notes=payload.notes,
        terms=payload.terms,
        user_id=current_user.id,
    )


@invoices_router.get("", response_model=list[InvoiceResponse])
async def list_invoices(
    status: str | None = None,
    document_type: str = "invoice",
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> list[Invoice]:
    q = (
        select(Invoice)
        .options(selectinload(Invoice.lines))
        .where(Invoice.document_type == document_type)
        .order_by(Invoice.issue_date.desc())
    )
    if status:
        q = q.where(Invoice.status == DocumentStatus(status))
    return (await db.execute(q)).scalars().all()


@invoices_router.get("/{invoice_id}", response_model=InvoiceResponse)
async def get_invoice(
    invoice_id: UUID,
    db: AsyncSession = Depends(get_db),
    _: User = Depends(get_current_user),
) -> Invoice:
    result = await db.execute(
        select(Invoice).options(selectinload(Invoice.lines)).where(Invoice.id == invoice_id)
    )
    inv = result.scalar_one_or_none()
    if not inv:
        raise HTTPException(status_code=404, detail="Facture introuvable.")
    return inv


@invoices_router.patch("/{invoice_id}/status", response_model=InvoiceResponse)
async def update_invoice_status(
    invoice_id: UUID,
    payload: InvoiceStatusUpdate,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
) -> Invoice:
    svc = InvoiceService(db)
    return await svc.transition(
        invoice_id, DocumentStatus(payload.status), current_user.id,
        payment_date=payload.payment_date, payment_amount=payload.payment_amount,
    )


# ─── TVA ──────────────────────────────────────────────────────────────────────

vat_router = APIRouter(prefix="/vat", tags=["vat"])


@vat_router.post("/compute", response_model=VATComputeResponse)
async def compute_vat(
    payload: VATComputeRequest,
    _: User = Depends(get_current_user),
) -> dict:
    from app.services.vat.calculator import VATIncluded
    included = VATIncluded(payload.included)
    result = VATCalculator.compute_line(payload.amount, payload.vat_rate, included)
    return {
        "base_amount": result.base_amount,
        "vat_rate": result.vat_rate,
        "vat_amount": result.vat_amount,
        "total_amount": result.total_amount,
        "vat_code": result.vat_code,
    }


@vat_router.get("/rates")
async def vat_rates(_: User = Depends(get_current_user)) -> dict:
    from app.services.vat.calculator import VAT_CODES
    return VAT_CODES
