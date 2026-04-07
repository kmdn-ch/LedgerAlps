"""
LedgerAlps — Service d'export et d'archivage
CO art. 958f : conservation 10 ans, intégrité garantie par hash.

Formats produits :
  - Grand Livre CSV (par compte, par période)
  - Balance de vérification CSV
  - Journal CSV complet
  - Manifest d'archive JSON avec hash de chaque fichier
"""

from __future__ import annotations

import csv
import hashlib
import io
import json
import zipfile
from datetime import date, datetime, timezone
from decimal import Decimal
from pathlib import Path
from typing import Any
from uuid import UUID

from sqlalchemy import and_, select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.models import Account, FiscalYear, JournalEntry, JournalEntryStatus, JournalLine
from app.services.accounting.journal import JournalService


class ExportService:
    """
    Génère les exports comptables.
    Chaque archive inclut un manifest JSON avec le hash SHA-256
    de chaque fichier pour garantir l'intégrité (CO art. 958f).
    """

    def __init__(self, db: AsyncSession) -> None:
        self.db = db
        self.journal_svc = JournalService(db)

    # ─── Grand Livre (General Ledger) ─────────────────────────────────────────

    async def export_general_ledger_csv(
        self,
        start_date: date,
        end_date: date,
        account_number: str | None = None,
    ) -> bytes:
        """
        Grand Livre : toutes les écritures par compte, avec soldes cumulés.
        Format CSV UTF-8 avec BOM (compatibilité Excel suisse).
        """
        q = (
            select(JournalLine)
            .join(JournalEntry, JournalLine.entry_id == JournalEntry.id)
            .join(Account, (JournalLine.debit_account_id == Account.id) |
                           (JournalLine.credit_account_id == Account.id))
            .where(
                and_(
                    JournalEntry.status == JournalEntryStatus.POSTED,
                    JournalEntry.date >= start_date,
                    JournalEntry.date <= end_date,
                )
            )
            .options(
                selectinload(JournalLine.entry),
            )
            .order_by(Account.number, JournalEntry.date, JournalEntry.reference)
        )

        if account_number:
            account = (await self.db.execute(
                select(Account).where(Account.number == account_number)
            )).scalar_one_or_none()
            if account:
                q = q.where(
                    (JournalLine.debit_account_id == account.id) |
                    (JournalLine.credit_account_id == account.id)
                )

        lines = (await self.db.execute(q)).scalars().all()

        buf = io.StringIO()
        buf.write("\ufeff")  # BOM UTF-8
        writer = csv.writer(buf, delimiter=";", quoting=csv.QUOTE_MINIMAL)

        writer.writerow([
            "Compte", "Désignation", "Date", "Référence", "Description",
            "Débit CHF", "Crédit CHF", "Solde cumulé CHF",
        ])

        current_account = None
        running_balance = Decimal("0")

        for line in lines:
            entry = line.entry
            # Déterminer le compte concerné pour cette ligne
            for acc_id, side in [
                (line.debit_account_id, "debit"),
                (line.credit_account_id, "credit"),
            ]:
                if acc_id is None:
                    continue
                acc = (await self.db.execute(
                    select(Account).where(Account.id == acc_id)
                )).scalar_one_or_none()
                if not acc:
                    continue

                if current_account != acc.number:
                    if current_account is not None:
                        writer.writerow([])  # séparateur entre comptes
                    current_account = acc.number
                    running_balance = Decimal("0")

                debit = line.amount_chf if side == "debit" else Decimal("0")
                credit = line.amount_chf if side == "credit" else Decimal("0")

                if acc.account_type in ("asset", "expense"):
                    running_balance += debit - credit
                else:
                    running_balance += credit - debit

                writer.writerow([
                    acc.number,
                    acc.name,
                    entry.date.strftime("%d.%m.%Y"),
                    entry.reference,
                    entry.description,
                    _fmt_chf(debit) if debit else "",
                    _fmt_chf(credit) if credit else "",
                    _fmt_chf(running_balance),
                ])

        return buf.getvalue().encode("utf-8")

    # ─── Balance de vérification ───────────────────────────────────────────────

    async def export_trial_balance_csv(self, as_of: date | None = None) -> bytes:
        """Balance de vérification CSV — contrôle sum(débit) == sum(crédit)."""
        rows = await self.journal_svc.get_trial_balance(as_of=as_of)

        buf = io.StringIO()
        buf.write("\ufeff")
        writer = csv.writer(buf, delimiter=";")
        writer.writerow(["Compte", "Désignation", "Débit CHF", "Crédit CHF", "Solde CHF"])

        for row in rows:
            writer.writerow([
                row["account_number"],
                row.get("account_name", ""),
                _fmt_chf(row["debit"]),
                _fmt_chf(row["credit"]),
                _fmt_chf(row["balance"]),
            ])

        return buf.getvalue().encode("utf-8")

    # ─── Journal complet ───────────────────────────────────────────────────────

    async def export_journal_csv(self, start_date: date, end_date: date) -> bytes:
        """Export du journal général — toutes les écritures validées."""
        entries = (await self.db.execute(
            select(JournalEntry)
            .options(selectinload(JournalEntry.lines))
            .where(
                and_(
                    JournalEntry.status == JournalEntryStatus.POSTED,
                    JournalEntry.date >= start_date,
                    JournalEntry.date <= end_date,
                )
            )
            .order_by(JournalEntry.date, JournalEntry.reference)
        )).scalars().all()

        buf = io.StringIO()
        buf.write("\ufeff")
        writer = csv.writer(buf, delimiter=";")
        writer.writerow([
            "Date", "Référence", "Description",
            "Cpt débit", "Cpt crédit", "Montant CHF",
            "Devise orig.", "Taux change",
            "Code TVA", "Hash intégrité",
        ])

        for entry in entries:
            for line in entry.lines:
                debit_acc = (await self.db.execute(
                    select(Account).where(Account.id == line.debit_account_id)
                )).scalar_one_or_none() if line.debit_account_id else None

                credit_acc = (await self.db.execute(
                    select(Account).where(Account.id == line.credit_account_id)
                )).scalar_one_or_none() if line.credit_account_id else None

                writer.writerow([
                    entry.date.strftime("%d.%m.%Y"),
                    entry.reference,
                    entry.description,
                    debit_acc.number if debit_acc else "",
                    credit_acc.number if credit_acc else "",
                    _fmt_chf(line.amount_chf),
                    line.currency,
                    str(line.exchange_rate),
                    line.vat_code or "",
                    (entry.integrity_hash or "")[:16] + "...",
                ])

        return buf.getvalue().encode("utf-8")

    # ─── Archive légale CO (10 ans) ───────────────────────────────────────────

    async def create_legal_archive(
        self,
        fiscal_year_id: UUID,
        output_dir: str | None = None,
    ) -> tuple[bytes, dict]:
        """
        Crée une archive ZIP conforme CO art. 958f :
          - journal.csv
          - grand_livre.csv
          - balance_verification.csv
          - manifest.json (hash SHA-256 de chaque fichier)

        La clé d'intégrité du manifest garantit que les fichiers
        n'ont pas été modifiés après l'archivage.
        """
        fy = (await self.db.execute(
            select(FiscalYear).where(FiscalYear.id == fiscal_year_id)
        )).scalar_one_or_none()

        if not fy:
            raise ValueError(f"Exercice introuvable : {fiscal_year_id}")

        start = fy.start_date
        end = fy.end_date
        year_label = f"{start.year}"

        # Générer les fichiers
        journal_csv = await self.export_journal_csv(start, end)
        ledger_csv = await self.export_general_ledger_csv(start, end)
        balance_csv = await self.export_trial_balance_csv(as_of=end)

        files = {
            f"journal_{year_label}.csv": journal_csv,
            f"grand_livre_{year_label}.csv": ledger_csv,
            f"balance_verification_{year_label}.csv": balance_csv,
        }

        # Manifest d'intégrité
        manifest: dict[str, Any] = {
            "ledgeralps_version": "0.1.0",
            "fiscal_year": year_label,
            "fiscal_year_start": str(start),
            "fiscal_year_end": str(end),
            "archived_at": datetime.now(timezone.utc).isoformat(),
            "co_retention_until": str(end.replace(year=end.year + 10)),
            "files": {},
        }

        for filename, content in files.items():
            sha256 = hashlib.sha256(content).hexdigest()
            manifest["files"][filename] = {
                "sha256": sha256,
                "size_bytes": len(content),
            }

        manifest_bytes = json.dumps(manifest, indent=2, ensure_ascii=False).encode("utf-8")
        manifest["manifest_hash"] = hashlib.sha256(manifest_bytes).hexdigest()
        manifest_bytes = json.dumps(manifest, indent=2, ensure_ascii=False).encode("utf-8")
        files["manifest.json"] = manifest_bytes

        # Créer le ZIP
        zip_buf = io.BytesIO()
        with zipfile.ZipFile(zip_buf, "w", zipfile.ZIP_DEFLATED) as zf:
            for filename, content in files.items():
                zf.writestr(filename, content)

        zip_bytes = zip_buf.getvalue()

        if output_dir:
            archive_path = Path(output_dir) / f"archive_{year_label}_{datetime.now().strftime('%Y%m%d_%H%M%S')}.zip"
            archive_path.parent.mkdir(parents=True, exist_ok=True)
            archive_path.write_bytes(zip_bytes)

        return zip_bytes, manifest


def _fmt_chf(value: Decimal | None) -> str:
    if value is None:
        return "0.00"
    return f"{value:.2f}".replace(".", ",")
