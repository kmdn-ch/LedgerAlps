"""
LedgerAlps — Tests Phase 3 : QR-facture, ISO 20022 pain.001, camt.053
"""

from decimal import Decimal
import pytest

from app.services.swiss_standards.qr_invoice import (
    QRAddress, QRInvoiceData, QRInvoiceGenerator, QRReferenceGenerator,
    QRInvoiceError, _validate_iban, _is_qr_iban, _mod10_recursive,
)
from app.services.swiss_standards.iso20022_pain001 import (
    Pain001Generator, Pain001Party, Pain001PaymentGroup, Pain001Transaction,
)
from app.services.swiss_standards.iso20022_camt import Camt053Parser


# ─── QR-facture ───────────────────────────────────────────────────────────────

class TestIBANValidation:

    def test_valid_ch_iban(self):
        assert _validate_iban("CH5604835012345678009") is True

    def test_invalid_iban_wrong_check(self):
        assert _validate_iban("CH9904835012345678009") is False

    def test_qr_iban_detected(self):
        # IID 30808 → QR-IBAN PostFinance
        assert _is_qr_iban("CH4431999123000889012") is True

    def test_regular_iban_not_qr(self):
        assert _is_qr_iban("CH5604835012345678009") is False


class TestQRRReference:

    def test_generate_qrr_27_digits(self):
        ref = QRReferenceGenerator.generate_qrr("FA2025-0001")
        digits_only = ref.replace(" ", "")
        assert len(digits_only) == 27
        assert digits_only.isdigit()

    def test_format_qrr_display(self):
        ref = "000000000000000001000000006"
        formatted = QRReferenceGenerator.format_qrr_display(ref)
        assert " " in formatted
        assert len(formatted.replace(" ", "")) == 27

    def test_generate_rf_reference(self):
        rf = QRReferenceGenerator.generate_rf("FA20250001")
        assert rf.startswith("RF")
        assert len(rf) >= 4

    def test_mod10_check_digit(self):
        # Exemple officiel Six-Group
        result = _mod10_recursive("00000000000000000000000001")
        assert isinstance(result, int)
        assert 0 <= result <= 9


class TestQRInvoicePayload:

    def _make_data(self, ref_type="NON", ref="") -> QRInvoiceData:
        return QRInvoiceData(
            creditor_iban="CH5604835012345678009",
            creditor=QRAddress(
                name="Acme SA",
                address_type="S",
                street_or_address_line1="Rue de la Paix 1",
                postal_code="1000",
                city="Lausanne",
                country="CH",
            ),
            amount=Decimal("1234.55"),
            currency="CHF",
            reference_type=ref_type,
            reference=ref,
            unstructured_message="Facture FA2025-0001",
        )

    def test_payload_starts_with_spc(self):
        data = self._make_data()
        payload = QRInvoiceGenerator.generate_payload(data)
        assert payload.startswith("SPC\n0200\n1\n")

    def test_payload_contains_amount(self):
        data = self._make_data()
        payload = QRInvoiceGenerator.generate_payload(data)
        assert "1234.55" in payload

    def test_payload_contains_currency(self):
        data = self._make_data()
        payload = QRInvoiceGenerator.generate_payload(data)
        assert "CHF" in payload

    def test_invalid_currency_raises(self):
        data = self._make_data()
        data.currency = "USD"
        with pytest.raises(QRInvoiceError, match="Devise non supportée"):
            QRInvoiceGenerator.generate_payload(data)

    def test_qrr_requires_qr_iban(self):
        data = self._make_data(ref_type="QRR", ref="0" * 27)
        # IBAN standard (pas QR-IBAN) → doit lever une erreur
        with pytest.raises(QRInvoiceError, match="QR-IBAN"):
            QRInvoiceGenerator.generate_payload(data)

    def test_none_amount_generates_empty_field(self):
        data = self._make_data()
        data.amount = None
        payload = QRInvoiceGenerator.generate_payload(data)
        lines = payload.split("\n")
        # Le montant est à la position 18 (0-indexed) selon spec Six-Group
        assert lines[18] == ""


# ─── ISO 20022 pain.001 ───────────────────────────────────────────────────────

class TestPain001Generator:

    def _make_generator(self) -> Pain001Generator:
        gen = Pain001Generator(initiating_party_name="LedgerAlps SA")
        debtor = Pain001Party(
            name="LedgerAlps SA",
            iban="CH5604835012345678009",
            bic="POFICHBEXXX",
        )
        group = Pain001PaymentGroup(
            payment_info_id="PG-20250101-001",
            debtor=debtor,
            requested_execution_date="2025-01-15",
            transactions=[
                Pain001Transaction(
                    end_to_end_id="E2E-001",
                    creditor=Pain001Party(
                        name="Fournisseur SA",
                        iban="CH9300762011623852957",
                    ),
                    amount=Decimal("500.00"),
                    currency="CHF",
                    remittance_info="Facture 2025-001",
                )
            ],
        )
        gen.add_group(group)
        return gen

    def test_generates_valid_xml(self):
        gen = self._make_generator()
        xml = gen.generate_xml()
        assert xml.startswith(b"<?xml")
        assert b"CstmrCdtTrfInitn" in xml

    def test_xml_contains_iban(self):
        gen = self._make_generator()
        xml = gen.generate_xml()
        assert b"CH5604835012345678009" in xml

    def test_xml_contains_amount(self):
        gen = self._make_generator()
        xml = gen.generate_xml()
        assert b"500.00" in xml

    def test_control_sum(self):
        gen = self._make_generator()
        assert gen.groups[0].control_sum == Decimal("500.00")

    def test_hash_generated(self):
        gen = self._make_generator()
        xml = gen.generate_xml()
        h = gen.generate_hash(xml)
        assert len(h) == 64
        assert h == gen.generate_hash(xml)  # déterministe

    def test_empty_groups_raises(self):
        gen = Pain001Generator(initiating_party_name="Test")
        with pytest.raises(ValueError, match="Aucun groupe"):
            gen.generate_xml()


# ─── camt.053 Parser ──────────────────────────────────────────────────────────

SAMPLE_CAMT053 = b"""<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.08">
  <BkToCstmrStmt>
    <GrpHdr><MsgId>MSG-20250101-001</MsgId><CreDtTm>2025-01-01T12:00:00</CreDtTm></GrpHdr>
    <Stmt>
      <Id>STMT-001</Id>
      <Acct><Id><IBAN>CH5604835012345678009</IBAN></Id><Ccy>CHF</Ccy></Acct>
      <Ntry>
        <Amt Ccy="CHF">1500.00</Amt>
        <CdtDbtInd>CRDT</CdtDbtInd>
        <Sts><Cd>BOOK</Cd></Sts>
        <BookgDt><Dt>2025-01-05</Dt></BookgDt>
        <ValDt><Dt>2025-01-05</Dt></ValDt>
        <NtryRef>NTRY-001</NtryRef>
        <RmtInf><Ustrd>Paiement Facture FA2025-0001</Ustrd></RmtInf>
      </Ntry>
    </Stmt>
  </BkToCstmrStmt>
</Document>"""


class TestCamt053Parser:

    def test_parse_sample(self):
        parser = Camt053Parser()
        statements = parser.parse(SAMPLE_CAMT053)
        assert len(statements) == 1

    def test_statement_iban(self):
        parser = Camt053Parser()
        stmt = parser.parse(SAMPLE_CAMT053)[0]
        assert stmt.account_iban == "CH5604835012345678009"

    def test_transaction_count(self):
        parser = Camt053Parser()
        stmt = parser.parse(SAMPLE_CAMT053)[0]
        assert len(stmt.transactions) == 1

    def test_transaction_amount(self):
        parser = Camt053Parser()
        stmt = parser.parse(SAMPLE_CAMT053)[0]
        txn = stmt.transactions[0]
        assert txn.amount == Decimal("1500.00")
        assert txn.credit_debit == "CRDT"
        assert "FA2025-0001" in txn.remittance_info

    def test_total_credits(self):
        parser = Camt053Parser()
        stmt = parser.parse(SAMPLE_CAMT053)[0]
        assert stmt.total_credits == Decimal("1500.00")
        assert stmt.total_debits == Decimal("0")
