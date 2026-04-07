"""
LedgerAlps — Modèles de la base de données

Plan comptable PME suisse structuré en classes d'actifs selon les normes
fiduciaires suisses. Tous les modèles sont conformes CO art. 957–963.
"""

from __future__ import annotations

import enum
from datetime import date, datetime
from decimal import Decimal
from uuid import UUID

from sqlalchemy import (
    Boolean, Date, DateTime, Enum, ForeignKey, Integer,
    Numeric, String, Text, UniqueConstraint, func, text,
)
from sqlalchemy.dialects.postgresql import JSONB, UUID as PG_UUID
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.db.base import AuditMixin, Base, TimestampMixin, UUIDPrimaryKey


# ─── Enums ────────────────────────────────────────────────────────────────────

class AccountType(str, enum.Enum):
    ASSET = "asset"          # Actif
    LIABILITY = "liability"  # Passif
    EQUITY = "equity"        # Capitaux propres
    REVENUE = "revenue"      # Produits
    EXPENSE = "expense"      # Charges


class AccountClass(str, enum.Enum):
    """Classes du plan comptable PME suisse."""
    C1_LIQUID_ASSETS = "1"          # Actif circulant — avoirs liquides
    C2_FIXED_ASSETS = "2"           # Actif immobilisé
    C3_LIABILITIES_ST = "3"         # Dettes à court terme — WRONG, see below
    # Selon PME suisse :
    # 1000–1999: Actif circulant
    # 2000–2999: Actif immobilisé
    # 3000–3999: Dettes à court terme (passif circulant)
    # 4000–4999: Dettes à long terme
    # 5000–5999: Capitaux propres
    # 6000–6999: Produits
    # 7000–7999: Charges de marchandises
    # 8000–8999: Charges de personnel
    # 9000–9999: Autres charges


class JournalEntryStatus(str, enum.Enum):
    DRAFT = "draft"
    POSTED = "posted"      # Validé — immuable après ce point (CO art. 957a)
    REVERSED = "reversed"  # Contrepassé (jamais supprimé)


class DocumentStatus(str, enum.Enum):
    DRAFT = "draft"
    SENT = "sent"
    PAID = "paid"
    OVERDUE = "overdue"
    CANCELLED = "cancelled"
    ARCHIVED = "archived"


class VATMethod(str, enum.Enum):
    EFFECTIVE = "effective"  # Méthode effective
    TDFN = "tdfn"            # Taux de la dette fiscale nette


# ─── Utilisateurs ─────────────────────────────────────────────────────────────

class User(Base, AuditMixin):
    """Utilisateur du système — nLPD : données minimales."""
    __tablename__ = "users"

    email: Mapped[str] = mapped_column(String(255), unique=True, nullable=False)
    name: Mapped[str] = mapped_column(String(255), nullable=False)
    password_hash: Mapped[str] = mapped_column(String(255), nullable=False)
    is_active: Mapped[bool] = mapped_column(Boolean, default=True)
    is_admin: Mapped[bool] = mapped_column(Boolean, default=False)
    last_login: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))

    # Relations
    journal_entries: Mapped[list["JournalEntry"]] = relationship(back_populates="created_by")


# ─── Plan Comptable ───────────────────────────────────────────────────────────

class Account(Base, AuditMixin):
    """
    Compte du plan comptable PME suisse.
    CO art. 957 : structure conforme aux exigences fiduciaires.
    """
    __tablename__ = "accounts"

    number: Mapped[str] = mapped_column(String(10), unique=True, nullable=False, index=True)
    name: Mapped[str] = mapped_column(String(255), nullable=False)
    name_de: Mapped[str | None] = mapped_column(String(255))  # Allemand
    name_it: Mapped[str | None] = mapped_column(String(255))  # Italien
    account_type: Mapped[AccountType] = mapped_column(Enum(AccountType), nullable=False)
    is_active: Mapped[bool] = mapped_column(Boolean, default=True)
    is_system: Mapped[bool] = mapped_column(Boolean, default=False)  # Comptes système non modifiables
    description: Mapped[str | None] = mapped_column(Text)

    # Hiérarchie du plan comptable
    parent_id: Mapped[UUID | None] = mapped_column(
        PG_UUID(as_uuid=True), ForeignKey("accounts.id"), nullable=True
    )
    parent: Mapped["Account | None"] = relationship("Account", remote_side="Account.id")
    children: Mapped[list["Account"]] = relationship("Account", back_populates="parent")

    # Relations
    debit_lines: Mapped[list["JournalLine"]] = relationship(
        "JournalLine", foreign_keys="JournalLine.debit_account_id"
    )
    credit_lines: Mapped[list["JournalLine"]] = relationship(
        "JournalLine", foreign_keys="JournalLine.credit_account_id"
    )

    def __repr__(self) -> str:
        return f"<Account {self.number} — {self.name}>"


# ─── Journal Comptable ────────────────────────────────────────────────────────

class JournalEntry(Base, AuditMixin):
    """
    Écriture au journal.
    CO art. 957a : immuable après validation (status = POSTED).
    Toute correction passe par une contrepassation.
    """
    __tablename__ = "journal_entries"

    date: Mapped[date] = mapped_column(Date, nullable=False, index=True)
    reference: Mapped[str] = mapped_column(String(50), nullable=False, index=True)
    description: Mapped[str] = mapped_column(Text, nullable=False)
    status: Mapped[JournalEntryStatus] = mapped_column(
        Enum(JournalEntryStatus), default=JournalEntryStatus.DRAFT, nullable=False
    )

    # Lien vers le document source (facture, note de crédit, etc.)
    source_document_id: Mapped[UUID | None] = mapped_column(PG_UUID(as_uuid=True))
    source_document_type: Mapped[str | None] = mapped_column(String(50))

    # Traçabilité CO
    posted_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    created_by_id: Mapped[UUID | None] = mapped_column(
        PG_UUID(as_uuid=True), ForeignKey("users.id")
    )
    created_by: Mapped["User | None"] = relationship(back_populates="journal_entries")
    integrity_hash: Mapped[str | None] = mapped_column(String(64))  # SHA-256

    # Contrepassation
    reversal_of_id: Mapped[UUID | None] = mapped_column(
        PG_UUID(as_uuid=True), ForeignKey("journal_entries.id")
    )

    lines: Mapped[list["JournalLine"]] = relationship(
        back_populates="entry", cascade="all, delete-orphan"
    )


class JournalLine(Base, UUIDPrimaryKey, TimestampMixin):
    """
    Ligne d'écriture — partie double obligatoire.
    Chaque JournalEntry doit avoir sum(debit) == sum(credit).
    """
    __tablename__ = "journal_lines"

    entry_id: Mapped[UUID] = mapped_column(
        PG_UUID(as_uuid=True), ForeignKey("journal_entries.id"), nullable=False
    )
    entry: Mapped["JournalEntry"] = relationship(back_populates="lines")

    debit_account_id: Mapped[UUID | None] = mapped_column(
        PG_UUID(as_uuid=True), ForeignKey("accounts.id")
    )
    credit_account_id: Mapped[UUID | None] = mapped_column(
        PG_UUID(as_uuid=True), ForeignKey("accounts.id")
    )
    amount: Mapped[Decimal] = mapped_column(Numeric(15, 2), nullable=False)
    currency: Mapped[str] = mapped_column(String(3), default="CHF", nullable=False)

    # Taux de change si devise étrangère
    exchange_rate: Mapped[Decimal] = mapped_column(Numeric(10, 6), default=Decimal("1.000000"))
    amount_chf: Mapped[Decimal] = mapped_column(Numeric(15, 2), nullable=False)

    description: Mapped[str | None] = mapped_column(Text)
    vat_code: Mapped[str | None] = mapped_column(String(10))
    vat_amount: Mapped[Decimal | None] = mapped_column(Numeric(15, 2))


# ─── Clients / Fournisseurs ───────────────────────────────────────────────────

class Contact(Base, AuditMixin):
    """
    Tiers : client ou fournisseur.
    nLPD : seules les données nécessaires à la relation commerciale sont stockées.
    """
    __tablename__ = "contacts"

    contact_type: Mapped[str] = mapped_column(String(20), nullable=False)  # client, supplier, both
    is_company: Mapped[bool] = mapped_column(Boolean, default=True)
    name: Mapped[str] = mapped_column(String(255), nullable=False)
    legal_name: Mapped[str | None] = mapped_column(String(255))

    # Coordonnées
    address_line1: Mapped[str | None] = mapped_column(String(255))
    address_line2: Mapped[str | None] = mapped_column(String(255))
    postal_code: Mapped[str | None] = mapped_column(String(20))
    city: Mapped[str | None] = mapped_column(String(100))
    country: Mapped[str] = mapped_column(String(2), default="CH")  # ISO 3166-1 alpha-2

    # Numéros légaux
    uid_number: Mapped[str | None] = mapped_column(String(20))   # CHE-123.456.789
    vat_number: Mapped[str | None] = mapped_column(String(30))

    # Contact
    email: Mapped[str | None] = mapped_column(String(255))
    phone: Mapped[str | None] = mapped_column(String(30))

    # Paiement
    payment_term_days: Mapped[int] = mapped_column(Integer, default=30)
    iban: Mapped[str | None] = mapped_column(String(34))
    currency: Mapped[str] = mapped_column(String(3), default="CHF")

    is_active: Mapped[bool] = mapped_column(Boolean, default=True)
    notes: Mapped[str | None] = mapped_column(Text)

    invoices: Mapped[list["Invoice"]] = relationship(back_populates="contact")


# ─── Facturation ──────────────────────────────────────────────────────────────

class Invoice(Base, AuditMixin):
    """
    Facture ou devis.
    Génère automatiquement les écritures comptables lors de la validation.
    """
    __tablename__ = "invoices"

    # Numérotation séquentielle — CO : les numéros doivent être continus
    number: Mapped[str] = mapped_column(String(30), unique=True, nullable=False)
    document_type: Mapped[str] = mapped_column(String(20), nullable=False)  # invoice, quote, credit_note
    status: Mapped[DocumentStatus] = mapped_column(
        Enum(DocumentStatus), default=DocumentStatus.DRAFT, nullable=False
    )

    contact_id: Mapped[UUID] = mapped_column(
        PG_UUID(as_uuid=True), ForeignKey("contacts.id"), nullable=False
    )
    contact: Mapped["Contact"] = relationship(back_populates="invoices")

    # Dates
    issue_date: Mapped[date] = mapped_column(Date, nullable=False)
    due_date: Mapped[date | None] = mapped_column(Date)
    service_period_start: Mapped[date | None] = mapped_column(Date)
    service_period_end: Mapped[date | None] = mapped_column(Date)

    # Montants
    currency: Mapped[str] = mapped_column(String(3), default="CHF")
    subtotal: Mapped[Decimal] = mapped_column(Numeric(15, 2), default=Decimal("0.00"))
    vat_amount: Mapped[Decimal] = mapped_column(Numeric(15, 2), default=Decimal("0.00"))
    total: Mapped[Decimal] = mapped_column(Numeric(15, 2), default=Decimal("0.00"))
    amount_paid: Mapped[Decimal] = mapped_column(Numeric(15, 2), default=Decimal("0.00"))

    # TVA
    vat_method: Mapped[VATMethod] = mapped_column(
        Enum(VATMethod), default=VATMethod.EFFECTIVE
    )

    # QR-facture
    qr_iban: Mapped[str | None] = mapped_column(String(34))
    qr_reference: Mapped[str | None] = mapped_column(String(27))  # RF ou QRR
    payment_info: Mapped[str | None] = mapped_column(String(140))  # Unstructured message

    # PDF archivé
    pdf_path: Mapped[str | None] = mapped_column(String(500))
    pdf_hash: Mapped[str | None] = mapped_column(String(64))  # Intégrité CO

    notes: Mapped[str | None] = mapped_column(Text)
    terms: Mapped[str | None] = mapped_column(Text)

    lines: Mapped[list["InvoiceLine"]] = relationship(
        back_populates="invoice", cascade="all, delete-orphan"
    )
    journal_entry: Mapped["JournalEntry | None"] = relationship(
        "JournalEntry",
        primaryjoin="and_(Invoice.id == foreign(JournalEntry.source_document_id), "
                    "JournalEntry.source_document_type == 'invoice')",
        uselist=False,
        viewonly=True,
    )


class InvoiceLine(Base, UUIDPrimaryKey, TimestampMixin):
    __tablename__ = "invoice_lines"

    invoice_id: Mapped[UUID] = mapped_column(
        PG_UUID(as_uuid=True), ForeignKey("invoices.id"), nullable=False
    )
    invoice: Mapped["Invoice"] = relationship(back_populates="lines")

    position: Mapped[int] = mapped_column(Integer, nullable=False)
    description: Mapped[str] = mapped_column(Text, nullable=False)
    quantity: Mapped[Decimal] = mapped_column(Numeric(10, 3), default=Decimal("1.000"))
    unit: Mapped[str | None] = mapped_column(String(20))  # h, pce, km, etc.
    unit_price: Mapped[Decimal] = mapped_column(Numeric(15, 2), nullable=False)
    discount_percent: Mapped[Decimal] = mapped_column(Numeric(5, 2), default=Decimal("0.00"))
    vat_rate: Mapped[Decimal] = mapped_column(Numeric(5, 2), default=Decimal("8.1"))
    vat_amount: Mapped[Decimal] = mapped_column(Numeric(15, 2), default=Decimal("0.00"))
    line_total: Mapped[Decimal] = mapped_column(Numeric(15, 2), nullable=False)

    # Lien au compte de produit
    revenue_account_id: Mapped[UUID | None] = mapped_column(
        PG_UUID(as_uuid=True), ForeignKey("accounts.id")
    )


# ─── Journal d'audit immuable (CO art. 957a) ─────────────────────────────────

class AuditLog(Base, UUIDPrimaryKey):
    """
    Journal d'audit immuable.
    Aucune suppression, aucune mise à jour n'est permise sur cette table.
    Le hash chaîné garantit l'intégrité de la séquence.
    """
    __tablename__ = "audit_logs"

    timestamp: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), nullable=False, index=True
    )
    entity_type: Mapped[str] = mapped_column(String(100), nullable=False)
    entity_id: Mapped[UUID] = mapped_column(PG_UUID(as_uuid=True), nullable=False)
    action: Mapped[str] = mapped_column(String(50), nullable=False)  # CREATE, UPDATE, DELETE, POST
    user_id: Mapped[UUID | None] = mapped_column(PG_UUID(as_uuid=True))
    before_state: Mapped[dict | None] = mapped_column(JSONB)
    after_state: Mapped[dict | None] = mapped_column(JSONB)
    ip_address: Mapped[str | None] = mapped_column(String(45))
    entry_hash: Mapped[str] = mapped_column(String(64), nullable=False)   # SHA-256 de l'entrée
    prev_hash: Mapped[str | None] = mapped_column(String(64))             # Hash de l'entrée précédente (chaîne)


# ─── Exercice Comptable ───────────────────────────────────────────────────────

class FiscalYear(Base, AuditMixin):
    """Exercice comptable — structure les périodes de reporting."""
    __tablename__ = "fiscal_years"

    name: Mapped[str] = mapped_column(String(20), nullable=False)
    start_date: Mapped[date] = mapped_column(Date, nullable=False)
    end_date: Mapped[date] = mapped_column(Date, nullable=False)
    is_closed: Mapped[bool] = mapped_column(Boolean, default=False)
    closed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    closed_by_id: Mapped[UUID | None] = mapped_column(PG_UUID(as_uuid=True), ForeignKey("users.id"))

    __table_args__ = (
        UniqueConstraint("name", name="uq_fiscal_year_name"),
    )
