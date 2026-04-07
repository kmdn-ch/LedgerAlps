"""
LedgerAlps — Générateur ISO 20022 pain.001 (CustomerCreditTransferInitiation)
Utilisé pour les virements groupés vers les banques suisses et européennes.
Spec : pain.001.001.09 (version actuelle pour CH/SEPA)
"""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass, field
from datetime import datetime, timezone
from decimal import Decimal
from typing import Literal
from uuid import uuid4
from xml.etree.ElementTree import Element, SubElement, tostring
from xml.dom.minidom import parseString


# ─── Types ────────────────────────────────────────────────────────────────────

ServiceLevel = Literal["SEPA", "NURG", "URGP"]
LocalInstrument = Literal["CT", "SALA", "DIVI"]


@dataclass
class Pain001Party:
    """Partie (débiteur ou créancier) dans un pain.001."""
    name: str                           # max 140 chars
    iban: str
    bic: str | None = None
    address_line: list[str] = field(default_factory=list)
    country: str = "CH"


@dataclass
class Pain001Transaction:
    """Une transaction individuelle dans un groupe de paiement."""
    end_to_end_id: str                  # Référence bout-en-bout (max 35 chars)
    creditor: Pain001Party
    amount: Decimal
    currency: str = "CHF"
    purpose_code: str | None = None     # e.g. "SALA" pour salaires
    remittance_info: str | None = None  # max 140 chars (non structuré)
    structured_ref: str | None = None   # RF ou QRR (structuré)
    execution_date: str | None = None   # YYYY-MM-DD


@dataclass
class Pain001PaymentGroup:
    """Groupe de paiements (PaymentInformation) — même débiteur, même date."""
    payment_info_id: str
    debtor: Pain001Party
    requested_execution_date: str       # YYYY-MM-DD
    service_level: ServiceLevel = "NURG"
    local_instrument: LocalInstrument | None = None
    transactions: list[Pain001Transaction] = field(default_factory=list)

    @property
    def control_sum(self) -> Decimal:
        return sum(t.amount for t in self.transactions)


# ─── Générateur pain.001 ──────────────────────────────────────────────────────

class Pain001Generator:
    """
    Génère un fichier XML pain.001.001.09 valide.
    Compatible avec PostFinance, UBS, Credit Suisse, Raiffeisen et banques SEPA.
    """

    NAMESPACE = "urn:iso:std:iso:20022:tech:xsd:pain.001.001.09"
    SCHEMA_LOCATION = (
        "urn:iso:std:iso:20022:tech:xsd:pain.001.001.09 "
        "pain.001.001.09.xsd"
    )

    def __init__(self, initiating_party_name: str, message_id: str | None = None) -> None:
        self.initiating_party_name = initiating_party_name[:140]
        self.message_id = message_id or f"LEDGERALPS-{uuid4().hex[:20].upper()}"
        self.groups: list[Pain001PaymentGroup] = []

    def add_group(self, group: Pain001PaymentGroup) -> None:
        self.groups.append(group)

    def generate_xml(self) -> bytes:
        """Génère le XML pain.001 complet, prêt pour upload bancaire."""
        if not self.groups:
            raise ValueError("Aucun groupe de paiement — le fichier serait vide.")

        root = Element("Document")
        root.set("xmlns", self.NAMESPACE)
        root.set("xmlns:xsi", "http://www.w3.org/2001/XMLSchema-instance")
        root.set("xsi:schemaLocation", self.SCHEMA_LOCATION)

        cstmr_cdt_trf = SubElement(root, "CstmrCdtTrfInitn")

        # ─── En-tête (GrpHdr) ─────────────────────────────────────────────────
        grp_hdr = SubElement(cstmr_cdt_trf, "GrpHdr")
        SubElement(grp_hdr, "MsgId").text = self.message_id[:35]
        SubElement(grp_hdr, "CreDtTm").text = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S")
        SubElement(grp_hdr, "NbOfTxs").text = str(
            sum(len(g.transactions) for g in self.groups)
        )
        SubElement(grp_hdr, "CtrlSum").text = str(
            sum(g.control_sum for g in self.groups).quantize(Decimal("0.01"))
        )
        initg_pty = SubElement(grp_hdr, "InitgPty")
        SubElement(initg_pty, "Nm").text = self.initiating_party_name

        # ─── Groupes de paiement ──────────────────────────────────────────────
        for group in self.groups:
            pmt_inf = SubElement(cstmr_cdt_trf, "PmtInf")
            SubElement(pmt_inf, "PmtInfId").text = group.payment_info_id[:35]
            SubElement(pmt_inf, "PmtMtd").text = "TRF"
            SubElement(pmt_inf, "NbOfTxs").text = str(len(group.transactions))
            SubElement(pmt_inf, "CtrlSum").text = str(
                group.control_sum.quantize(Decimal("0.01"))
            )

            # Méthode de paiement
            pmt_tp_inf = SubElement(pmt_inf, "PmtTpInf")
            svc_lvl = SubElement(pmt_tp_inf, "SvcLvl")
            SubElement(svc_lvl, "Cd").text = group.service_level
            if group.local_instrument:
                lcl_instr = SubElement(pmt_tp_inf, "LclInstrm")
                SubElement(lcl_instr, "Cd").text = group.local_instrument

            SubElement(pmt_inf, "ReqdExctnDt").text = group.requested_execution_date

            # Débiteur
            dbtr = SubElement(pmt_inf, "Dbtr")
            SubElement(dbtr, "Nm").text = group.debtor.name[:140]
            self._add_postal_address(dbtr, group.debtor)

            dbtr_acct = SubElement(pmt_inf, "DbtrAcct")
            dbtr_id = SubElement(dbtr_acct, "Id")
            SubElement(dbtr_id, "IBAN").text = group.debtor.iban.replace(" ", "")

            if group.debtor.bic:
                dbtr_agt = SubElement(pmt_inf, "DbtrAgt")
                fin_instn = SubElement(dbtr_agt, "FinInstnId")
                SubElement(fin_instn, "BICFI").text = group.debtor.bic

            # Transactions
            for txn in group.transactions:
                self._add_transaction(pmt_inf, txn)

        xml_bytes = tostring(root, encoding="unicode", xml_declaration=False)
        xml_str = '<?xml version="1.0" encoding="UTF-8"?>\n' + xml_bytes
        return xml_str.encode("utf-8")

    def _add_transaction(self, pmt_inf: Element, txn: Pain001Transaction) -> None:
        cdt_trf_tx_inf = SubElement(pmt_inf, "CdtTrfTxInf")

        # Référence bout-en-bout
        pmt_id = SubElement(cdt_trf_tx_inf, "PmtId")
        SubElement(pmt_id, "EndToEndId").text = txn.end_to_end_id[:35]

        # Montant
        amt = SubElement(cdt_trf_tx_inf, "Amt")
        SubElement(amt, "InstdAmt", Ccy=txn.currency).text = str(
            txn.amount.quantize(Decimal("0.01"))
        )

        # Créancier
        if txn.creditor.bic:
            cdtr_agt = SubElement(cdt_trf_tx_inf, "CdtrAgt")
            fin_instn = SubElement(cdtr_agt, "FinInstnId")
            SubElement(fin_instn, "BICFI").text = txn.creditor.bic

        cdtr = SubElement(cdt_trf_tx_inf, "Cdtr")
        SubElement(cdtr, "Nm").text = txn.creditor.name[:140]
        self._add_postal_address(cdtr, txn.creditor)

        cdtr_acct = SubElement(cdt_trf_tx_inf, "CdtrAcct")
        cdtr_id = SubElement(cdtr_acct, "Id")
        SubElement(cdtr_id, "IBAN").text = txn.creditor.iban.replace(" ", "")

        # Informations de paiement
        if txn.structured_ref:
            rmt_inf = SubElement(cdt_trf_tx_inf, "RmtInf")
            strd = SubElement(rmt_inf, "Strd")
            cdtr_ref_inf = SubElement(strd, "CdtrRefInf")
            tp = SubElement(cdtr_ref_inf, "Tp")
            cd_or_prtry = SubElement(tp, "CdOrPrtry")
            ref_type = "SCOR" if txn.structured_ref.upper().startswith("RF") else "QRR"
            SubElement(cd_or_prtry, "Cd").text = ref_type
            SubElement(cdtr_ref_inf, "Ref").text = txn.structured_ref.replace(" ", "")
        elif txn.remittance_info:
            rmt_inf = SubElement(cdt_trf_tx_inf, "RmtInf")
            SubElement(rmt_inf, "Ustrd").text = txn.remittance_info[:140]

    def _add_postal_address(self, parent: Element, party: Pain001Party) -> None:
        if party.address_line or party.country:
            pstl_adr = SubElement(parent, "PstlAdr")
            SubElement(pstl_adr, "Ctry").text = party.country
            for line in party.address_line[:2]:
                SubElement(pstl_adr, "AdrLine").text = line[:70]

    def generate_hash(self, xml_bytes: bytes) -> str:
        """Hash SHA-256 du fichier pour transmission sécurisée à la banque."""
        return hashlib.sha256(xml_bytes).hexdigest()

    @staticmethod
    def pretty_print(xml_bytes: bytes) -> str:
        return parseString(xml_bytes).toprettyxml(indent="  ")
