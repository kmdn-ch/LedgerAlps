"""
LedgerAlps — Moteur de comptabilité en partie double
CO art. 957 : sum(débit) == sum(crédit) pour chaque écriture.
"""

from __future__ import annotations

import hashlib
import json
from datetime import date, datetime, timezone
from decimal import Decimal, ROUND_HALF_UP
from uuid import UUID

from sqlalchemy import select, func, and_
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.compliance import requires_double_entry
from app.models import (
    Account, AccountType, JournalEntry, JournalEntryStatus,
    JournalLine, FiscalYear, AuditLog,
)


CHF = Decimal("0.01")


class AccountingError(Exception):
    pass


class ImbalancedEntryError(AccountingError):
    """Levée si débit ≠ crédit — CO art. 957."""
    pass


class ClosedPeriodError(AccountingError):
    """Levée si l'exercice est clôturé."""
    pass


class PostedEntryError(AccountingError):
    """Levée si on tente de modifier une écriture validée."""
    pass


# ─── Service Journal ──────────────────────────────────────────────────────────

class JournalService:
    """
    Gère les écritures au journal.
    Règle fondamentale : une écriture ne peut jamais être supprimée
    après validation (CO art. 957a). Seule la contrepassation est admise.
    """

    def __init__(self, db: AsyncSession) -> None:
        self.db = db

    @requires_double_entry
    async def create_entry(
        self,
        *,
        entry_date: date,
        description: str,
        lines: list[dict],  # [{"debit_account": "1100", "credit_account": "6100", "amount": 1000.00, ...}]
        reference: str | None = None,
        user_id: UUID,
        source_document_id: UUID | None = None,
        source_document_type: str | None = None,
    ) -> JournalEntry:
        """
        Crée une écriture brouillon au journal.
        La validation (posting) se fait séparément via post_entry().
        """
        # Vérifier que la période n'est pas clôturée
        await self._check_period_open(entry_date)

        # Construire les lignes et vérifier l'équilibre
        journal_lines = await self._build_lines(lines)
        self._assert_balanced(journal_lines)

        ref = reference or await self._next_reference(entry_date)

        entry = JournalEntry(
            date=entry_date,
            reference=ref,
            description=description,
            status=JournalEntryStatus.DRAFT,
            created_by_id=user_id,
            source_document_id=source_document_id,
            source_document_type=source_document_type,
            lines=journal_lines,
        )
        self.db.add(entry)
        await self.db.flush()
        return entry

    async def post_entry(self, entry_id: UUID, user_id: UUID) -> JournalEntry:
        """
        Valide une écriture brouillon → status POSTED.
        Après cette opération : IMMUABLE (CO art. 957a).
        """
        entry = await self._get_entry(entry_id)

        if entry.status != JournalEntryStatus.DRAFT:
            raise PostedEntryError(f"L'écriture {entry.reference} est déjà validée.")

        # Double vérification de l'équilibre
        self._assert_balanced(entry.lines)

        entry.status = JournalEntryStatus.POSTED
        entry.posted_at = datetime.now(timezone.utc)
        entry.integrity_hash = self._compute_entry_hash(entry)

        await self._write_audit(entry, user_id, "POST")
        await self.db.flush()
        return entry

    async def reverse_entry(
        self,
        entry_id: UUID,
        reversal_date: date,
        user_id: UUID,
        description: str | None = None,
    ) -> JournalEntry:
        """
        Contrepasse une écriture validée.
        CO art. 957a : jamais de suppression, toujours une contrepassation.
        """
        original = await self._get_entry(entry_id)

        if original.status != JournalEntryStatus.POSTED:
            raise AccountingError("Seules les écritures validées peuvent être contrepassées.")

        # Inverser les lignes débit/crédit
        reversed_lines = []
        for line in original.lines:
            reversed_lines.append({
                "debit_account_id": line.credit_account_id,
                "credit_account_id": line.debit_account_id,
                "amount": line.amount,
                "currency": line.currency,
                "exchange_rate": line.exchange_rate,
                "amount_chf": line.amount_chf,
                "description": f"Contrepassation: {line.description or ''}",
            })

        reversal = JournalEntry(
            date=reversal_date,
            reference=await self._next_reference(reversal_date),
            description=description or f"Contrepassation de {original.reference}",
            status=JournalEntryStatus.DRAFT,
            created_by_id=user_id,
            reversal_of_id=original.id,
            lines=[JournalLine(**l) for l in reversed_lines],
        )
        original.status = JournalEntryStatus.REVERSED
        self.db.add(reversal)
        await self.db.flush()
        return reversal

    # ─── Grand Livre ──────────────────────────────────────────────────────────

    async def get_account_balance(
        self,
        account_number: str,
        as_of: date | None = None,
        fiscal_year_id: UUID | None = None,
    ) -> dict:
        """Calcule le solde d'un compte à une date donnée."""
        account = await self._get_account_by_number(account_number)

        q = (
            select(
                func.coalesce(func.sum(JournalLine.amount_chf).filter(
                    JournalLine.debit_account_id == account.id
                ), Decimal("0")).label("total_debit"),
                func.coalesce(func.sum(JournalLine.amount_chf).filter(
                    JournalLine.credit_account_id == account.id
                ), Decimal("0")).label("total_credit"),
            )
            .join(JournalEntry, JournalLine.entry_id == JournalEntry.id)
            .where(JournalEntry.status == JournalEntryStatus.POSTED)
        )

        if as_of:
            q = q.where(JournalEntry.date <= as_of)

        result = (await self.db.execute(q)).one()
        debit = result.total_debit or Decimal("0")
        credit = result.total_credit or Decimal("0")

        # Sens du solde selon le type de compte
        if account.account_type in (AccountType.ASSET, AccountType.EXPENSE):
            balance = debit - credit
        else:
            balance = credit - debit

        return {
            "account_number": account.number,
            "account_name": account.name,
            "account_type": account.account_type,
            "debit": debit.quantize(CHF),
            "credit": credit.quantize(CHF),
            "balance": balance.quantize(CHF),
            "as_of": as_of,
        }

    async def get_trial_balance(self, as_of: date | None = None) -> list[dict]:
        """Balance de vérification — tous les comptes actifs."""
        accounts = (await self.db.execute(
            select(Account).where(Account.is_active == True).order_by(Account.number)
        )).scalars().all()

        rows = []
        total_debit = Decimal("0")
        total_credit = Decimal("0")

        for account in accounts:
            bal = await self.get_account_balance(account.number, as_of=as_of)
            if bal["debit"] != Decimal("0") or bal["credit"] != Decimal("0"):
                rows.append(bal)
                total_debit += bal["debit"]
                total_credit += bal["credit"]

        rows.append({
            "account_number": "TOTAL",
            "account_name": "Total",
            "debit": total_debit.quantize(CHF),
            "credit": total_credit.quantize(CHF),
            "balance": (total_debit - total_credit).quantize(CHF),
        })
        return rows

    # ─── Helpers privés ───────────────────────────────────────────────────────

    def _assert_balanced(self, lines: list[JournalLine]) -> None:
        """CO art. 957 : débit == crédit, sinon exception."""
        total_debit = sum(
            l.amount_chf for l in lines if l.debit_account_id is not None
        )
        total_credit = sum(
            l.amount_chf for l in lines if l.credit_account_id is not None
        )
        if total_debit.quantize(CHF) != total_credit.quantize(CHF):
            raise ImbalancedEntryError(
                f"Écriture déséquilibrée : débit={total_debit} ≠ crédit={total_credit}"
            )

    async def _build_lines(self, lines_data: list[dict]) -> list[JournalLine]:
        result = []
        for ld in lines_data:
            debit_acc = await self._get_account_by_number(ld.get("debit_account", "")) if ld.get("debit_account") else None
            credit_acc = await self._get_account_by_number(ld.get("credit_account", "")) if ld.get("credit_account") else None
            amount = Decimal(str(ld["amount"])).quantize(CHF, rounding=ROUND_HALF_UP)
            exchange_rate = Decimal(str(ld.get("exchange_rate", "1.000000")))
            amount_chf = (amount * exchange_rate).quantize(CHF, rounding=ROUND_HALF_UP)

            result.append(JournalLine(
                debit_account_id=debit_acc.id if debit_acc else None,
                credit_account_id=credit_acc.id if credit_acc else None,
                amount=amount,
                currency=ld.get("currency", "CHF"),
                exchange_rate=exchange_rate,
                amount_chf=amount_chf,
                description=ld.get("description"),
                vat_code=ld.get("vat_code"),
                vat_amount=Decimal(str(ld["vat_amount"])).quantize(CHF) if ld.get("vat_amount") else None,
            ))
        return result

    async def _get_entry(self, entry_id: UUID) -> JournalEntry:
        result = await self.db.execute(
            select(JournalEntry).where(JournalEntry.id == entry_id)
        )
        entry = result.scalar_one_or_none()
        if not entry:
            raise AccountingError(f"Écriture introuvable : {entry_id}")
        return entry

    async def _get_account_by_number(self, number: str) -> Account:
        result = await self.db.execute(
            select(Account).where(Account.number == number, Account.is_active == True)
        )
        account = result.scalar_one_or_none()
        if not account:
            raise AccountingError(f"Compte introuvable ou inactif : {number}")
        return account

    async def _check_period_open(self, entry_date: date) -> None:
        result = await self.db.execute(
            select(FiscalYear).where(
                and_(
                    FiscalYear.start_date <= entry_date,
                    FiscalYear.end_date >= entry_date,
                    FiscalYear.is_closed == True,
                )
            )
        )
        if result.scalar_one_or_none():
            raise ClosedPeriodError(f"La période du {entry_date} est clôturée.")

    async def _next_reference(self, entry_date: date) -> str:
        year = entry_date.year
        count = await self.db.execute(
            select(func.count(JournalEntry.id)).where(
                func.extract("year", JournalEntry.date) == year
            )
        )
        n = (count.scalar() or 0) + 1
        return f"JE{year}-{n:05d}"

    def _compute_entry_hash(self, entry: JournalEntry) -> str:
        payload = json.dumps({
            "id": str(entry.id),
            "date": str(entry.date),
            "reference": entry.reference,
            "description": entry.description,
            "lines": [
                {
                    "debit": str(l.debit_account_id),
                    "credit": str(l.credit_account_id),
                    "amount_chf": str(l.amount_chf),
                }
                for l in entry.lines
            ],
        }, sort_keys=True)
        return hashlib.sha256(payload.encode()).hexdigest()

    async def _write_audit(self, entry: JournalEntry, user_id: UUID, action: str) -> None:
        self.db.add(AuditLog(
            entity_type="journal_entry",
            entity_id=entry.id,
            action=action,
            user_id=user_id,
            after_state={"status": entry.status, "reference": entry.reference},
            entry_hash=entry.integrity_hash or "",
        ))
