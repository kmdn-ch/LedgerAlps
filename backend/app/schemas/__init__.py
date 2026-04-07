"""
LedgerAlps — Schémas Pydantic v2
Validation des entrées/sorties API.
"""

from __future__ import annotations

from datetime import date, datetime
from decimal import Decimal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, EmailStr, Field, field_validator


# ─── Base ─────────────────────────────────────────────────────────────────────

class APIModel(BaseModel):
    model_config = ConfigDict(from_attributes=True, populate_by_name=True)


# ─── Auth ─────────────────────────────────────────────────────────────────────

class LoginRequest(BaseModel):
    email: EmailStr
    password: str = Field(min_length=8)


class TokenResponse(BaseModel):
    access_token: str
    refresh_token: str
    token_type: str = "bearer"


class UserCreate(BaseModel):
    email: EmailStr
    name: str = Field(min_length=1, max_length=255)
    password: str = Field(min_length=8)


class UserResponse(APIModel):
    id: UUID
    email: str
    name: str
    is_active: bool
    is_admin: bool
    created_at: datetime


# ─── Comptes ──────────────────────────────────────────────────────────────────

class AccountResponse(APIModel):
    id: UUID
    number: str
    name: str
    account_type: str
    is_active: bool
    parent_id: UUID | None = None


class AccountBalanceResponse(BaseModel):
    account_number: str
    account_name: str
    account_type: str
    debit: Decimal
    credit: Decimal
    balance: Decimal
    as_of: date | None = None


# ─── Journal ──────────────────────────────────────────────────────────────────

class JournalLineCreate(BaseModel):
    debit_account: str | None = None
    credit_account: str | None = None
    amount: Decimal = Field(gt=0)
    currency: str = Field(default="CHF", max_length=3)
    exchange_rate: Decimal = Field(default=Decimal("1.0"), gt=0)
    description: str | None = None
    vat_code: str | None = None
    vat_amount: Decimal | None = None

    @field_validator("debit_account", "credit_account")
    @classmethod
    def at_least_one_account(cls, v: str | None) -> str | None:
        return v  # validation croisée dans le service


class JournalEntryCreate(BaseModel):
    date: date
    description: str = Field(min_length=1, max_length=500)
    lines: list[JournalLineCreate] = Field(min_length=1)
    reference: str | None = None


class JournalLineResponse(APIModel):
    id: UUID
    debit_account_id: UUID | None
    credit_account_id: UUID | None
    amount: Decimal
    currency: str
    amount_chf: Decimal
    description: str | None


class JournalEntryResponse(APIModel):
    id: UUID
    date: date
    reference: str
    description: str
    status: str
    posted_at: datetime | None
    lines: list[JournalLineResponse]
    created_at: datetime


# ─── Contacts ─────────────────────────────────────────────────────────────────

class ContactCreate(BaseModel):
    contact_type: str = Field(default="client")
    is_company: bool = True
    name: str = Field(min_length=1, max_length=255)
    legal_name: str | None = None
    address_line1: str | None = None
    address_line2: str | None = None
    postal_code: str | None = None
    city: str | None = None
    country: str = Field(default="CH", max_length=2)
    uid_number: str | None = None
    vat_number: str | None = None
    email: EmailStr | None = None
    phone: str | None = None
    payment_term_days: int = Field(default=30, ge=0, le=365)
    iban: str | None = None
    currency: str = Field(default="CHF", max_length=3)
    notes: str | None = None


class ContactResponse(APIModel):
    id: UUID
    contact_type: str
    is_company: bool
    name: str
    city: str | None
    country: str
    email: str | None
    phone: str | None
    payment_term_days: int
    is_active: bool
    created_at: datetime


# ─── Factures ─────────────────────────────────────────────────────────────────

class InvoiceLineCreate(BaseModel):
    description: str = Field(min_length=1)
    quantity: Decimal = Field(default=Decimal("1"), gt=0)
    unit: str | None = None
    unit_price: Decimal = Field(gt=0)
    discount_percent: Decimal = Field(default=Decimal("0"), ge=0, le=100)
    vat_rate: Decimal = Field(default=Decimal("8.1"), ge=0)


class InvoiceCreate(BaseModel):
    contact_id: UUID
    issue_date: date
    due_date: date | None = None
    document_type: str = Field(default="invoice")
    vat_method: str = Field(default="effective")
    qr_iban: str | None = None
    payment_info: str | None = Field(default=None, max_length=140)
    notes: str | None = None
    terms: str | None = None
    lines: list[InvoiceLineCreate] = Field(min_length=1)


class InvoiceLineResponse(APIModel):
    id: UUID
    position: int
    description: str
    quantity: Decimal
    unit: str | None
    unit_price: Decimal
    discount_percent: Decimal
    vat_rate: Decimal
    vat_amount: Decimal
    line_total: Decimal


class InvoiceResponse(APIModel):
    id: UUID
    number: str
    document_type: str
    status: str
    contact_id: UUID
    issue_date: date
    due_date: date | None
    subtotal: Decimal
    vat_amount: Decimal
    total: Decimal
    amount_paid: Decimal
    qr_iban: str | None
    notes: str | None = None
    terms: str | None = None
    lines: list[InvoiceLineResponse]
    created_at: datetime


class InvoiceStatusUpdate(BaseModel):
    status: str
    payment_date: date | None = None
    payment_amount: Decimal | None = None


# ─── TVA ──────────────────────────────────────────────────────────────────────

class VATComputeRequest(BaseModel):
    amount: Decimal = Field(gt=0)
    vat_rate: Decimal = Field(default=Decimal("8.1"), ge=0)
    included: str = Field(default="excluded")


class VATComputeResponse(BaseModel):
    base_amount: Decimal
    vat_rate: Decimal
    vat_amount: Decimal
    total_amount: Decimal
    vat_code: str


# ─── Contacts — mise à jour ───────────────────────────────────────────────────

class ContactUpdate(BaseModel):
    contact_type: str | None = None
    is_company: bool | None = None
    name: str | None = Field(default=None, min_length=1, max_length=255)
    legal_name: str | None = None
    address_line1: str | None = None
    address_line2: str | None = None
    postal_code: str | None = None
    city: str | None = None
    country: str | None = Field(default=None, max_length=2)
    uid_number: str | None = None
    vat_number: str | None = None
    email: EmailStr | None = None
    phone: str | None = None
    payment_term_days: int | None = Field(default=None, ge=0, le=365)
    iban: str | None = None
    currency: str | None = Field(default=None, max_length=3)
    is_active: bool | None = None
    notes: str | None = None


# ─── Pagination ───────────────────────────────────────────────────────────────

class PaginatedResponse(BaseModel):
    items: list
    total: int
    page: int
    page_size: int
    pages: int


class JournalPageResponse(BaseModel):
    items: list[JournalEntryResponse]
    total: int
    page: int
    page_size: int
    pages: int
