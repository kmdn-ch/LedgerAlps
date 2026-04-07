"""
LedgerAlps — Parser ISO 20022 camt.053 / camt.054
camt.053 : Relevé de compte (Bank To Customer Statement)
camt.054 : Notification de débit/crédit (Bank To Customer Debit Credit Notification)

Utilisé pour l'import automatique des mouvements bancaires
et la réconciliation avec les écritures comptables.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import date, datetime
from decimal import Decimal
from xml.etree import ElementTree as ET


# ─── Namespaces ISO 20022 ─────────────────────────────────────────────────────

CAMT053_NS = {
    "v08": "urn:iso:std:iso:20022:tech:xsd:camt.053.001.08",
    "v06": "urn:iso:std:iso:20022:tech:xsd:camt.053.001.06",
    "v02": "urn:iso:std:iso:20022:tech:xsd:camt.053.001.02",
}

CAMT054_NS = {
    "v08": "urn:iso:std:iso:20022:tech:xsd:camt.054.001.08",
    "v06": "urn:iso:std:iso:20022:tech:xsd:camt.054.001.06",
}


# ─── Structures de données ────────────────────────────────────────────────────

@dataclass
class BankTransaction:
    """Un mouvement bancaire extrait d'un camt.053 ou camt.054."""
    entry_reference: str
    booking_date: date
    value_date: date
    amount: Decimal
    currency: str
    credit_debit: str            # CRDT ou DBIT
    status: str                  # BOOK, PDNG, INFO
    bank_tx_code: str = ""
    end_to_end_id: str = ""
    payment_id: str = ""
    remittance_info: str = ""
    structured_ref: str = ""     # QRR ou RF
    counterparty_name: str = ""
    counterparty_iban: str = ""
    is_reconciled: bool = False
    raw_additional_info: str = ""


@dataclass
class AccountStatement:
    """Relevé de compte complet extrait d'un fichier camt.053."""
    message_id: str
    creation_datetime: datetime
    account_iban: str
    account_currency: str
    statement_id: str
    sequence_number: int = 0
    opening_balance: Decimal = Decimal("0")
    closing_balance: Decimal = Decimal("0")
    opening_date: date | None = None
    closing_date: date | None = None
    transactions: list[BankTransaction] = field(default_factory=list)

    @property
    def total_credits(self) -> Decimal:
        return sum(t.amount for t in self.transactions if t.credit_debit == "CRDT")

    @property
    def total_debits(self) -> Decimal:
        return sum(t.amount for t in self.transactions if t.credit_debit == "DBIT")


# ─── Parser camt.053 ──────────────────────────────────────────────────────────

class Camt053Parser:
    """
    Parse un fichier XML camt.053 (relevé mensuel).
    Supporte les versions 02, 06 et 08 de la spec ISO 20022.
    """

    def parse(self, xml_content: bytes) -> list[AccountStatement]:
        """Retourne la liste des relevés contenus dans le fichier."""
        try:
            root = ET.fromstring(xml_content)
        except ET.ParseError as e:
            raise Camt053ParseError(f"XML invalide : {e}") from e

        ns = self._detect_namespace(root)
        if not ns:
            raise Camt053ParseError("Namespace camt.053 non reconnu.")

        statements = []
        for stmt_el in root.findall(f".//{{{ns}}}Stmt"):
            statements.append(self._parse_statement(stmt_el, ns))
        return statements

    def _parse_statement(self, stmt: ET.Element, ns: str) -> AccountStatement:
        def txt(tag: str, parent: ET.Element = stmt) -> str:
            el = parent.find(f".//{{{ns}}}{tag}")
            return el.text.strip() if el is not None and el.text else ""

        def dec(tag: str, parent: ET.Element = stmt) -> Decimal:
            val = txt(tag, parent)
            return Decimal(val) if val else Decimal("0")

        # Compte
        acct = stmt.find(f".//{{{ns}}}Acct")
        iban = txt("IBAN", acct) if acct is not None else ""
        currency = txt("Ccy", acct) if acct is not None else "CHF"

        # Soldes
        opening_bal = Decimal("0")
        closing_bal = Decimal("0")
        opening_date = None
        closing_date = None

        for bal_el in stmt.findall(f"{{{ns}}}Bal"):
            cd = txt("Cd", bal_el)
            amt_el = bal_el.find(f"{{{ns}}}Amt")
            amt = Decimal(amt_el.text) if amt_el is not None and amt_el.text else Decimal("0")
            cdt_dbt = txt("CdtDbtInd", bal_el)
            if cdt_dbt == "DBIT":
                amt = -amt
            dt = txt("Dt", bal_el) or txt("DtTm", bal_el)
            parsed_date = _parse_date(dt) if dt else None

            if cd == "OPBD":
                opening_bal = amt
                opening_date = parsed_date
            elif cd in ("CLBD", "CLAV"):
                closing_bal = amt
                closing_date = parsed_date

        # Transactions
        transactions = []
        for entry_el in stmt.findall(f"{{{ns}}}Ntry"):
            txn = self._parse_entry(entry_el, ns)
            if txn:
                transactions.append(txn)

        return AccountStatement(
            message_id=txt("MsgId"),
            creation_datetime=datetime.now(),
            account_iban=iban,
            account_currency=currency,
            statement_id=txt("Id"),
            sequence_number=int(txt("SeqNb") or "0"),
            opening_balance=opening_bal,
            closing_balance=closing_bal,
            opening_date=opening_date,
            closing_date=closing_date,
            transactions=transactions,
        )

    def _parse_entry(self, entry: ET.Element, ns: str) -> BankTransaction | None:
        def txt(tag: str, parent: ET.Element = entry) -> str:
            el = parent.find(f".//{{{ns}}}{tag}")
            return el.text.strip() if el is not None and el.text else ""

        credit_debit = txt("CdtDbtInd")
        if not credit_debit:
            return None

        amt_el = entry.find(f"{{{ns}}}Amt")
        amount = Decimal(amt_el.text) if amt_el is not None and amt_el.text else Decimal("0")
        currency = amt_el.get("Ccy", "CHF") if amt_el is not None else "CHF"

        booking_dt = txt("BookgDt")
        value_dt = txt("ValDt") or booking_dt

        # Remittance info
        rmt_el = entry.find(f".//{{{ns}}}RmtInf")
        remittance = ""
        structured_ref = ""
        if rmt_el is not None:
            remittance = txt("Ustrd", rmt_el)
            ref_el = rmt_el.find(f".//{{{ns}}}Ref")
            if ref_el is not None and ref_el.text:
                structured_ref = ref_el.text.strip()

        # Contrepartie
        rltd_parties = entry.find(f".//{{{ns}}}RltdPties")
        cp_name = ""
        cp_iban = ""
        if rltd_parties is not None:
            party_tag = "Cdtr" if credit_debit == "CRDT" else "Dbtr"
            party_el = rltd_parties.find(f".//{{{ns}}}{party_tag}")
            if party_el is not None:
                cp_name = txt("Nm", party_el)
            iban_el = rltd_parties.find(f".//{{{ns}}}IBAN")
            if iban_el is not None and iban_el.text:
                cp_iban = iban_el.text.strip()

        return BankTransaction(
            entry_reference=txt("NtryRef") or txt("AcctSvcrRef"),
            booking_date=_parse_date(booking_dt) or date.today(),
            value_date=_parse_date(value_dt) or date.today(),
            amount=amount,
            currency=currency,
            credit_debit=credit_debit,
            status=txt("Sts"),
            bank_tx_code=txt("Cd"),
            end_to_end_id=txt("EndToEndId"),
            payment_id=txt("PmtInfId"),
            remittance_info=remittance,
            structured_ref=structured_ref,
            counterparty_name=cp_name,
            counterparty_iban=cp_iban,
        )

    def _detect_namespace(self, root: ET.Element) -> str | None:
        tag = root.tag
        if "{" in tag:
            ns_uri = tag.split("}")[0][1:]
        else:
            ns_uri = root.get("xmlns", "")

        for ns_map in (CAMT053_NS, CAMT054_NS):
            for version, uri in ns_map.items():
                if ns_uri == uri:
                    return uri
        # Chercher dans les enfants
        for child in root.iter():
            if "camt.053" in child.tag or "camt.054" in child.tag:
                return child.tag.split("}")[0][1:]
        return None


class Camt053ParseError(Exception):
    pass


# ─── Réconciliation automatique ───────────────────────────────────────────────

class BankReconciler:
    """
    Tente de réconcilier les transactions bancaires avec les factures ouvertes.
    Logique :
      1. Correspondance exacte sur la référence QRR/RF
      2. Correspondance sur le montant + contrepartie
      3. Reste non réconcilié → queue pour traitement manuel
    """

    @staticmethod
    def match_transactions(
        transactions: list[BankTransaction],
        open_invoices: list[dict],  # [{"number": str, "total": Decimal, "qr_reference": str}]
    ) -> dict[str, list]:
        matched = []
        unmatched = []

        for txn in transactions:
            if txn.credit_debit != "CRDT":
                unmatched.append(txn)
                continue

            found = False
            # Priorité 1 : référence structurée (QRR / RF)
            if txn.structured_ref:
                clean_ref = txn.structured_ref.replace(" ", "")
                for inv in open_invoices:
                    inv_ref = (inv.get("qr_reference") or "").replace(" ", "")
                    if inv_ref and clean_ref == inv_ref:
                        matched.append({"transaction": txn, "invoice": inv, "match_type": "reference"})
                        found = True
                        break

            # Priorité 2 : montant exact
            if not found:
                for inv in open_invoices:
                    if inv["total"] == txn.amount:
                        matched.append({"transaction": txn, "invoice": inv, "match_type": "amount"})
                        found = True
                        break

            if not found:
                unmatched.append(txn)

        return {"matched": matched, "unmatched": unmatched}


# ─── Utilitaires ─────────────────────────────────────────────────────────────

def _parse_date(dt_str: str) -> date | None:
    if not dt_str:
        return None
    try:
        return date.fromisoformat(dt_str[:10])
    except ValueError:
        return None
