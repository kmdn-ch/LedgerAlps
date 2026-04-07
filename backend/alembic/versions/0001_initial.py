"""Migration initiale — tous les modèles LedgerAlps

Revision ID: 0001_initial
Revises:
Create Date: 2026-04-07 00:00:00.000000

Conforme CO art. 957–963 : toutes les tables avec traçabilité UUID + timestamps.
"""

from __future__ import annotations

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

# revision identifiers
revision: str = "0001_initial"
down_revision: str | None = None
branch_labels: str | None = None
depends_on: str | None = None


def upgrade() -> None:
    # ── Enums PostgreSQL ────────────────────────────────────────────────────────
    accounttype = postgresql.ENUM(
        "asset", "liability", "equity", "revenue", "expense",
        name="accounttype", create_type=True,
    )
    journalentrystatus = postgresql.ENUM(
        "draft", "posted", "reversed",
        name="journalentrystatus", create_type=True,
    )
    documentstatus = postgresql.ENUM(
        "draft", "sent", "paid", "overdue", "cancelled", "archived",
        name="documentstatus", create_type=True,
    )
    vatmethod = postgresql.ENUM(
        "effective", "tdfn",
        name="vatmethod", create_type=True,
    )
    accounttype.create(op.get_bind(), checkfirst=True)
    journalentrystatus.create(op.get_bind(), checkfirst=True)
    documentstatus.create(op.get_bind(), checkfirst=True)
    vatmethod.create(op.get_bind(), checkfirst=True)

    # ── users ───────────────────────────────────────────────────────────────────
    op.create_table(
        "users",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True, nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("email", sa.String(255), nullable=False),
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("password_hash", sa.String(255), nullable=False),
        sa.Column("is_active", sa.Boolean(), nullable=False, server_default=sa.text("true")),
        sa.Column("is_admin", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("last_login", sa.DateTime(timezone=True), nullable=True),
        sa.UniqueConstraint("email", name="uq_users_email"),
    )

    # ── accounts ────────────────────────────────────────────────────────────────
    op.create_table(
        "accounts",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True, nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("number", sa.String(10), nullable=False),
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("name_de", sa.String(255), nullable=True),
        sa.Column("name_it", sa.String(255), nullable=True),
        sa.Column("account_type", sa.Enum("asset", "liability", "equity", "revenue", "expense", name="accounttype"), nullable=False),
        sa.Column("is_active", sa.Boolean(), nullable=False, server_default=sa.text("true")),
        sa.Column("is_system", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("description", sa.Text(), nullable=True),
        sa.Column("parent_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("accounts.id"), nullable=True),
        sa.UniqueConstraint("number", name="uq_accounts_number"),
        sa.Index("ix_accounts_number", "number"),
    )

    # ── fiscal_years ────────────────────────────────────────────────────────────
    op.create_table(
        "fiscal_years",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True, nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("name", sa.String(20), nullable=False),
        sa.Column("start_date", sa.Date(), nullable=False),
        sa.Column("end_date", sa.Date(), nullable=False),
        sa.Column("is_closed", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("closed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("closed_by_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("users.id"), nullable=True),
        sa.UniqueConstraint("name", name="uq_fiscal_year_name"),
    )

    # ── journal_entries ─────────────────────────────────────────────────────────
    op.create_table(
        "journal_entries",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True, nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("date", sa.Date(), nullable=False),
        sa.Column("reference", sa.String(50), nullable=False),
        sa.Column("description", sa.Text(), nullable=False),
        sa.Column("status", sa.Enum("draft", "posted", "reversed", name="journalentrystatus"), nullable=False, server_default="draft"),
        sa.Column("source_document_id", postgresql.UUID(as_uuid=True), nullable=True),
        sa.Column("source_document_type", sa.String(50), nullable=True),
        sa.Column("posted_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("created_by_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("users.id"), nullable=True),
        sa.Column("integrity_hash", sa.String(64), nullable=True),
        sa.Column("reversal_of_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("journal_entries.id"), nullable=True),
        sa.Index("ix_journal_entries_date", "date"),
        sa.Index("ix_journal_entries_reference", "reference"),
    )

    # ── journal_lines ───────────────────────────────────────────────────────────
    op.create_table(
        "journal_lines",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True, nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("entry_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("journal_entries.id"), nullable=False),
        sa.Column("debit_account_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("accounts.id"), nullable=True),
        sa.Column("credit_account_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("accounts.id"), nullable=True),
        sa.Column("amount", sa.Numeric(15, 2), nullable=False),
        sa.Column("currency", sa.String(3), nullable=False, server_default="CHF"),
        sa.Column("exchange_rate", sa.Numeric(10, 6), nullable=False, server_default="1.000000"),
        sa.Column("amount_chf", sa.Numeric(15, 2), nullable=False),
        sa.Column("description", sa.Text(), nullable=True),
        sa.Column("vat_code", sa.String(10), nullable=True),
        sa.Column("vat_amount", sa.Numeric(15, 2), nullable=True),
    )

    # ── contacts ────────────────────────────────────────────────────────────────
    op.create_table(
        "contacts",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True, nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("contact_type", sa.String(20), nullable=False),
        sa.Column("is_company", sa.Boolean(), nullable=False, server_default=sa.text("true")),
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("legal_name", sa.String(255), nullable=True),
        sa.Column("address_line1", sa.String(255), nullable=True),
        sa.Column("address_line2", sa.String(255), nullable=True),
        sa.Column("postal_code", sa.String(20), nullable=True),
        sa.Column("city", sa.String(100), nullable=True),
        sa.Column("country", sa.String(2), nullable=False, server_default="CH"),
        sa.Column("uid_number", sa.String(20), nullable=True),
        sa.Column("vat_number", sa.String(30), nullable=True),
        sa.Column("email", sa.String(255), nullable=True),
        sa.Column("phone", sa.String(30), nullable=True),
        sa.Column("payment_term_days", sa.Integer(), nullable=False, server_default="30"),
        sa.Column("iban", sa.String(34), nullable=True),
        sa.Column("currency", sa.String(3), nullable=False, server_default="CHF"),
        sa.Column("is_active", sa.Boolean(), nullable=False, server_default=sa.text("true")),
        sa.Column("notes", sa.Text(), nullable=True),
    )

    # ── invoices ────────────────────────────────────────────────────────────────
    op.create_table(
        "invoices",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True, nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("number", sa.String(30), nullable=False),
        sa.Column("document_type", sa.String(20), nullable=False),
        sa.Column("status", sa.Enum("draft", "sent", "paid", "overdue", "cancelled", "archived", name="documentstatus"), nullable=False, server_default="draft"),
        sa.Column("contact_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("contacts.id"), nullable=False),
        sa.Column("issue_date", sa.Date(), nullable=False),
        sa.Column("due_date", sa.Date(), nullable=True),
        sa.Column("service_period_start", sa.Date(), nullable=True),
        sa.Column("service_period_end", sa.Date(), nullable=True),
        sa.Column("currency", sa.String(3), nullable=False, server_default="CHF"),
        sa.Column("subtotal", sa.Numeric(15, 2), nullable=False, server_default="0.00"),
        sa.Column("vat_amount", sa.Numeric(15, 2), nullable=False, server_default="0.00"),
        sa.Column("total", sa.Numeric(15, 2), nullable=False, server_default="0.00"),
        sa.Column("amount_paid", sa.Numeric(15, 2), nullable=False, server_default="0.00"),
        sa.Column("vat_method", sa.Enum("effective", "tdfn", name="vatmethod"), nullable=False, server_default="effective"),
        sa.Column("qr_iban", sa.String(34), nullable=True),
        sa.Column("qr_reference", sa.String(27), nullable=True),
        sa.Column("payment_info", sa.String(140), nullable=True),
        sa.Column("pdf_path", sa.String(500), nullable=True),
        sa.Column("pdf_hash", sa.String(64), nullable=True),
        sa.Column("notes", sa.Text(), nullable=True),
        sa.Column("terms", sa.Text(), nullable=True),
        sa.UniqueConstraint("number", name="uq_invoices_number"),
    )

    # ── invoice_lines ───────────────────────────────────────────────────────────
    op.create_table(
        "invoice_lines",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True, nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("invoice_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("invoices.id"), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.Column("description", sa.Text(), nullable=False),
        sa.Column("quantity", sa.Numeric(10, 3), nullable=False, server_default="1.000"),
        sa.Column("unit", sa.String(20), nullable=True),
        sa.Column("unit_price", sa.Numeric(15, 2), nullable=False),
        sa.Column("discount_percent", sa.Numeric(5, 2), nullable=False, server_default="0.00"),
        sa.Column("vat_rate", sa.Numeric(5, 2), nullable=False, server_default="8.10"),
        sa.Column("vat_amount", sa.Numeric(15, 2), nullable=False, server_default="0.00"),
        sa.Column("line_total", sa.Numeric(15, 2), nullable=False),
        sa.Column("revenue_account_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("accounts.id"), nullable=True),
    )

    # ── audit_logs ──────────────────────────────────────────────────────────────
    op.create_table(
        "audit_logs",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True, nullable=False),
        sa.Column("timestamp", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("entity_type", sa.String(100), nullable=False),
        sa.Column("entity_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("action", sa.String(50), nullable=False),
        sa.Column("user_id", postgresql.UUID(as_uuid=True), nullable=True),
        sa.Column("before_state", postgresql.JSONB(), nullable=True),
        sa.Column("after_state", postgresql.JSONB(), nullable=True),
        sa.Column("ip_address", sa.String(45), nullable=True),
        sa.Column("entry_hash", sa.String(64), nullable=False),
        sa.Column("prev_hash", sa.String(64), nullable=True),
        sa.Index("ix_audit_logs_timestamp", "timestamp"),
    )


def downgrade() -> None:
    op.drop_table("audit_logs")
    op.drop_table("invoice_lines")
    op.drop_table("invoices")
    op.drop_table("contacts")
    op.drop_table("journal_lines")
    op.drop_table("journal_entries")
    op.drop_table("fiscal_years")
    op.drop_table("accounts")
    op.drop_table("users")

    # Supprimer les enums PostgreSQL
    for name in ("vatmethod", "documentstatus", "journalentrystatus", "accounttype"):
        sa.Enum(name=name).drop(op.get_bind(), checkfirst=True)
