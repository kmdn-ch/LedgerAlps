"""
LedgerAlps — Tests unitaires : QR-facture suisse (qr_invoice.py, Corrections v2)

Couverture :
  - Validation / formatage IBAN, détection QR-IBAN
  - QRAddress : construction des lignes, validation
  - QRInvoiceGenerator.generate_payload : structure 31 lignes, erreurs
  - QRReferenceGenerator : QRR (mod10 récursif), RF (ISO 11649), affichage
  - generate_qr_image / generate_qr_svg (ignorés si qrcode/Pillow absents)
"""

from decimal import Decimal

import pytest

from app.services.swiss_standards.qr_invoice import (
    QRAddress,
    QRInvoiceData,
    QRInvoiceError,
    QRInvoiceGenerator,
    QRReferenceGenerator,
    format_iban,
    is_qr_iban,
    validate_iban,
    _mod10_recursive,
    _rf_checksum,
    _round_to_5_rappen,
)


VALID_IBAN = "CH5604835012345678009"       # IBAN standard valide (mod 97 == 1)
VALID_QR_IBAN = "CH4431999123000889012"    # IID 31999 → QR-IBAN (borne haute)


def make_creditor(**overrides) -> QRAddress:
    defaults = dict(
        name="Acme SA",
        address_type="S",
        street="Rue de la Paix",
        house_number="1",
        postal_code="1000",
        city="Lausanne",
        country="CH",
    )
    defaults.update(overrides)
    return QRAddress(**defaults)


def make_data(**overrides) -> QRInvoiceData:
    defaults = dict(
        creditor_iban=VALID_IBAN,
        creditor=make_creditor(),
        amount=Decimal("1234.55"),
        currency="CHF",
        reference_type="NON",
        reference="",
        unstructured_message="Facture FA2025-0001",
    )
    defaults.update(overrides)
    return QRInvoiceData(**defaults)


# ─── IBAN : validation, QR-IBAN, formatage ───────────────────────────────────

class TestValidateIban:

    def test_valid_iban(self):
        assert validate_iban(VALID_IBAN) is True

    def test_invalid_check_digits(self):
        # Mêmes chiffres BBAN, check digits différents (56 → 99)
        assert validate_iban("CH9904835012345678009") is False

    def test_accepts_spaces_and_lowercase(self):
        assert validate_iban("ch56 0483 5012 3456 7800 9") is True

    def test_rejects_malformed_structure(self):
        assert validate_iban("NOTANIBAN") is False

    def test_rejects_empty_string(self):
        assert validate_iban("") is False


class TestIsQrIban:

    def test_detects_qr_iban_upper_boundary(self):
        # IID 31999 = borne haute incluse
        assert is_qr_iban(VALID_QR_IBAN) is True

    def test_regular_iban_is_not_qr(self):
        assert is_qr_iban(VALID_IBAN) is False

    def test_iid_lower_boundary_30000_is_qr(self):
        assert is_qr_iban("CH0030000000000000000") is True

    def test_iid_just_below_boundary_29999_is_not_qr(self):
        assert is_qr_iban("CH0029999000000000000") is False

    def test_iid_just_above_boundary_32000_is_not_qr(self):
        assert is_qr_iban("CH0032000000000000000") is False

    def test_li_prefix_supported(self):
        assert is_qr_iban("LI0031000000000000000") is True

    def test_non_ch_li_prefix_rejected(self):
        assert is_qr_iban("DE0031000000000000000") is False

    def test_too_short_returns_false_not_exception(self):
        assert is_qr_iban("CH1") is False


class TestFormatIban:

    def test_groups_by_four_with_final_remainder(self):
        assert format_iban(VALID_IBAN) == "CH56 0483 5012 3456 7800 9"

    def test_normalizes_case_and_existing_spaces(self):
        assert format_iban("ch56 0483501234567800 9") == "CH56 0483 5012 3456 7800 9"


# ─── QRAddress ────────────────────────────────────────────────────────────────

class TestQRAddress:

    def test_to_lines_order_and_content(self):
        addr = make_creditor()
        assert addr.to_lines() == ["S", "Acme SA", "Rue de la Paix", "1", "1000", "Lausanne", "CH"]

    def test_to_lines_truncates_oversized_fields(self):
        addr = make_creditor(
            name="N" * 100,
            street="R" * 100,
            house_number="H" * 30,
            postal_code="P" * 30,
            city="C" * 60,
            country="chf",  # 3 lettres, doit être tronqué à 2 et mis en majuscules
        )
        lines = addr.to_lines()
        assert len(lines[1]) == 70   # name
        assert len(lines[2]) == 70   # street
        assert len(lines[3]) == 16   # house_number
        assert len(lines[4]) == 16   # postal_code
        assert len(lines[5]) == 35   # city
        assert lines[6] == "CH"      # country tronqué + upper

    def test_validate_requires_name(self):
        addr = make_creditor(name="   ")
        with pytest.raises(QRInvoiceError, match="Nom du créancier"):
            addr.validate()

    def test_validate_rejects_invalid_address_type(self):
        addr = make_creditor(address_type="X")
        with pytest.raises(QRInvoiceError, match="Type d'adresse invalide"):
            addr.validate()

    def test_structured_address_requires_postal_code(self):
        addr = make_creditor(postal_code="")
        with pytest.raises(QRInvoiceError, match="NPA requis"):
            addr.validate()

    def test_structured_address_requires_city(self):
        addr = make_creditor(city="")
        with pytest.raises(QRInvoiceError, match="Ville requise"):
            addr.validate()

    def test_combined_address_does_not_require_postal_code_or_city(self):
        addr = make_creditor(address_type="K", postal_code="", city="")
        addr.validate()  # ne doit pas lever

    def test_validate_rejects_invalid_country(self):
        addr = make_creditor(country="ZZ")
        with pytest.raises(QRInvoiceError, match="Code pays invalide"):
            addr.validate()


# ─── QRInvoiceGenerator.generate_payload ─────────────────────────────────────

class TestGeneratePayload:

    def test_payload_has_exactly_31_lines_minimal(self):
        payload = QRInvoiceGenerator.generate_payload(make_data())
        assert len(payload.split("\n")) == 31

    def test_payload_starts_with_header(self):
        payload = QRInvoiceGenerator.generate_payload(make_data())
        assert payload.startswith("SPC\n0200\n1\n")

    def test_payload_contains_cleaned_iban(self):
        data = make_data(creditor_iban="ch56 0483 5012 3456 7800 9")
        payload = QRInvoiceGenerator.generate_payload(data)
        lines = payload.split("\n")
        assert lines[3] == VALID_IBAN

    def test_payload_contains_rounded_amount(self):
        data = make_data(amount=Decimal("1234.53"))  # arrondi 5 centimes → 1234.55
        payload = QRInvoiceGenerator.generate_payload(data)
        lines = payload.split("\n")
        assert lines[18] == "1234.55"

    def test_none_amount_gives_empty_amount_line(self):
        data = make_data(amount=None)
        payload = QRInvoiceGenerator.generate_payload(data)
        lines = payload.split("\n")
        assert lines[18] == ""

    def test_payload_contains_currency(self):
        payload = QRInvoiceGenerator.generate_payload(make_data())
        lines = payload.split("\n")
        assert lines[19] == "CHF"

    def test_debtor_lines_empty_when_no_debtor(self):
        payload = QRInvoiceGenerator.generate_payload(make_data())
        lines = payload.split("\n")
        assert lines[20:27] == [""] * 7

    def test_debtor_lines_populated_when_present(self):
        debtor = make_creditor(name="Client SA", city="Genève")
        data = make_data(debtor=debtor)
        payload = QRInvoiceGenerator.generate_payload(data)
        lines = payload.split("\n")
        assert lines[20:27] == debtor.to_lines()

    def test_reference_and_message_lines(self):
        data = make_data(reference_type="NON", reference="", unstructured_message="Facture FA2025-0001")
        payload = QRInvoiceGenerator.generate_payload(data)
        lines = payload.split("\n")
        assert lines[27] == "NON"
        assert lines[28] == ""
        assert lines[29] == "Facture FA2025-0001"
        assert lines[30] == "EPD"

    def test_reference_spaces_stripped_in_payload(self):
        qrr = QRReferenceGenerator.generate_qrr("1", "210000000")  # formaté avec espaces
        data = make_data(
            creditor_iban=VALID_QR_IBAN,
            reference_type="QRR",
            reference=qrr,
        )
        payload = QRInvoiceGenerator.generate_payload(data)
        lines = payload.split("\n")
        assert " " not in lines[28]
        assert len(lines[28]) == 27

    def test_bill_information_appended_after_epd(self):
        data = make_data(bill_information="//S1/10/1234")
        payload = QRInvoiceGenerator.generate_payload(data)
        lines = payload.split("\n")
        assert lines[30] == "EPD"
        assert lines[31] == "//S1/10/1234"

    def test_alternative_schemes_limited_to_two(self):
        data = make_data(alternative_schemes=["scheme1", "scheme2", "scheme3"])
        payload = QRInvoiceGenerator.generate_payload(data)
        lines = payload.split("\n")
        assert lines[-2:] == ["scheme1", "scheme2"]

    def test_invalid_currency_raises(self):
        data = make_data(currency="USD")
        with pytest.raises(QRInvoiceError, match="Devise non supportée"):
            QRInvoiceGenerator.generate_payload(data)

    def test_invalid_creditor_iban_raises(self):
        data = make_data(creditor_iban="CH0000000000000000000")
        with pytest.raises(QRInvoiceError, match="IBAN invalide"):
            QRInvoiceGenerator.generate_payload(data)

    def test_qrr_reference_requires_qr_iban(self):
        data = make_data(
            creditor_iban=VALID_IBAN,  # IBAN standard, pas QR-IBAN
            reference_type="QRR",
            reference="1" * 27,
        )
        with pytest.raises(QRInvoiceError, match="QR-IBAN"):
            QRInvoiceGenerator.generate_payload(data)

    def test_scor_reference_rejects_qr_iban(self):
        rf = QRReferenceGenerator.generate_rf("FA20250001")
        data = make_data(
            creditor_iban=VALID_QR_IBAN,
            reference_type="SCOR",
            reference=rf,
        )
        with pytest.raises(QRInvoiceError, match="SCOR"):
            QRInvoiceGenerator.generate_payload(data)

    def test_qrr_with_bad_check_digit_raises(self):
        qrr = QRReferenceGenerator.generate_qrr("1", "210000000").replace(" ", "")
        tampered = qrr[:-1] + str((int(qrr[-1]) + 1) % 10)
        data = make_data(
            creditor_iban=VALID_QR_IBAN,
            reference_type="QRR",
            reference=tampered,
        )
        with pytest.raises(QRInvoiceError, match="contrôle QRR"):
            QRInvoiceGenerator.generate_payload(data)

    def test_qrr_wrong_length_raises(self):
        data = make_data(
            creditor_iban=VALID_QR_IBAN,
            reference_type="QRR",
            reference="123",
        )
        with pytest.raises(QRInvoiceError, match="27 chiffres"):
            QRInvoiceGenerator.generate_payload(data)

    def test_invalid_reference_type_raises(self):
        data = make_data(reference_type="BOGUS")
        with pytest.raises(QRInvoiceError, match="Type de référence invalide"):
            QRInvoiceGenerator.generate_payload(data)

    def test_amount_below_minimum_raises(self):
        data = make_data(amount=Decimal("0.00"))
        with pytest.raises(QRInvoiceError, match="Montant minimum"):
            QRInvoiceGenerator.generate_payload(data)

    def test_amount_above_maximum_raises(self):
        data = make_data(amount=Decimal("1000000000.00"))
        with pytest.raises(QRInvoiceError, match="Montant maximum"):
            QRInvoiceGenerator.generate_payload(data)

    def test_message_over_140_chars_raises(self):
        data = make_data(unstructured_message="x" * 141)
        with pytest.raises(QRInvoiceError, match="140 caractères"):
            QRInvoiceGenerator.generate_payload(data)

    def test_message_of_exactly_140_chars_is_accepted(self):
        data = make_data(unstructured_message="x" * 140)
        payload = QRInvoiceGenerator.generate_payload(data)
        assert payload.split("\n")[29] == "x" * 140

    def test_invalid_creditor_address_surfaces_through_payload(self):
        data = make_data(creditor=make_creditor(name=""))
        with pytest.raises(QRInvoiceError, match="Nom du créancier"):
            QRInvoiceGenerator.generate_payload(data)

    def test_invalid_debtor_address_surfaces_through_payload(self):
        data = make_data(debtor=make_creditor(postal_code=""))
        with pytest.raises(QRInvoiceError, match="NPA requis"):
            QRInvoiceGenerator.generate_payload(data)


# ─── Arrondi suisse (0.05 CHF) ────────────────────────────────────────────────

class TestRoundToFiveRappen:

    @pytest.mark.parametrize(
        "raw, expected",
        [
            ("1.23", "1.25"),
            ("1.22", "1.20"),
            ("1.225", "1.25"),
            ("0.00", "0.00"),
            ("1234.53", "1234.55"),
        ],
    )
    def test_rounding_examples(self, raw, expected):
        assert _round_to_5_rappen(Decimal(raw)) == Decimal(expected)


# ─── Check digit modulo 10 récursif ──────────────────────────────────────────

class TestMod10Recursive:

    @pytest.mark.parametrize(
        "digit, expected",
        [("0", 0), ("1", 1), ("5", 8), ("9", 5)],
    )
    def test_single_digit_matches_table_complement(self, digit, expected):
        # Un seul chiffre : carry = TABLE[digit], résultat = (10 - carry) % 10
        assert _mod10_recursive(digit) == expected

    def test_returns_single_digit_in_range(self):
        result = _mod10_recursive("00000000000000000000000001")
        assert isinstance(result, int)
        assert 0 <= result <= 9


# ─── QRReferenceGenerator : QRR ───────────────────────────────────────────────

class TestGenerateQrr:

    def test_produces_27_digits(self):
        ref = QRReferenceGenerator.generate_qrr("FA2025-0001")
        digits = ref.replace(" ", "")
        assert len(digits) == 27
        assert digits.isdigit()

    def test_round_trips_through_validate_qrr(self):
        ref = QRReferenceGenerator.generate_qrr("FA2025-0001")
        assert QRReferenceGenerator.validate_qrr(ref) is True

    def test_participant_id_preserved_in_first_9_digits(self):
        ref = QRReferenceGenerator.generate_qrr("1", "210000000")
        digits = ref.replace(" ", "")
        assert digits[:9] == "210000000"
        assert digits[9:26] == "1".zfill(17)

    def test_participant_id_longer_than_9_is_truncated(self):
        ref = QRReferenceGenerator.generate_qrr("1", "1234567890123")
        digits = ref.replace(" ", "")
        assert digits[:9] == "123456789"

    def test_non_numeric_customer_ref_chars_are_stripped(self):
        ref = QRReferenceGenerator.generate_qrr("FA2025-0001", "210000000")
        digits = ref.replace(" ", "")
        # "FA2025-0001" → chiffres seuls "20250001", zfill(17)
        assert digits[9:26] == "20250001".zfill(17)
        assert QRReferenceGenerator.validate_qrr(digits) is True

    def test_is_formatted_with_spaces_for_display(self):
        ref = QRReferenceGenerator.generate_qrr("1", "210000000")
        assert ref.count(" ") == 5


class TestFormatDisplay:

    def test_groups_27_digits_as_2_5_5_5_5_5(self):
        ref = "0" * 26 + "6"  # 27 chiffres
        formatted = QRReferenceGenerator.format_display(ref)
        assert formatted == "00 00000 00000 00000 00000 00006"

    def test_passes_through_when_not_27_digits(self):
        assert QRReferenceGenerator.format_display("12345") == "12345"

    def test_ignores_existing_whitespace_before_formatting(self):
        ref = "0" * 26 + "6"
        spaced = ref[:5] + "  " + ref[5:]
        assert QRReferenceGenerator.format_display(spaced) == "00 00000 00000 00000 00000 00006"


class TestValidateQrr:

    def test_valid_generated_reference_passes(self):
        ref = QRReferenceGenerator.generate_qrr("FA2025-0001")
        assert QRReferenceGenerator.validate_qrr(ref) is True

    def test_tampered_check_digit_fails(self):
        ref = QRReferenceGenerator.generate_qrr("FA2025-0001").replace(" ", "")
        tampered = ref[:-1] + str((int(ref[-1]) + 1) % 10)
        assert QRReferenceGenerator.validate_qrr(tampered) is False

    def test_wrong_length_fails(self):
        assert QRReferenceGenerator.validate_qrr("123") is False

    def test_non_digit_content_fails(self):
        assert QRReferenceGenerator.validate_qrr("A" * 27) is False


# ─── QRReferenceGenerator : RF (ISO 11649) ───────────────────────────────────

class TestGenerateRf:

    def test_starts_with_rf_and_two_check_digits(self):
        rf = QRReferenceGenerator.generate_rf("FA20250001")
        assert rf.startswith("RF")
        assert rf[2:4].isdigit()

    def test_known_checksum_value(self):
        # 98 - (numérique("AB" + "RF00") mod 97) = 98 - 37 = 61 (calculé à la main
        # via l'algorithme ISO 11649 documenté dans _rf_checksum)
        assert QRReferenceGenerator.generate_rf("ab") == "RF61AB"

    def test_strips_non_alphanumeric_and_uppercases(self):
        rf = QRReferenceGenerator.generate_rf("fa-2025/0001")
        assert rf == QRReferenceGenerator.generate_rf("FA20250001")

    def test_truncates_to_21_chars(self):
        rf = QRReferenceGenerator.generate_rf("X" * 30)
        assert len(rf) == 2 + 2 + 21  # "RF" + 2 chiffres + 21 caractères

    def test_round_trips_through_validate_rf(self):
        rf = QRReferenceGenerator.generate_rf("FA20250001")
        assert QRReferenceGenerator.validate_rf(rf) is True


class TestValidateRf:

    def test_known_good_reference(self):
        assert QRReferenceGenerator.validate_rf("RF61AB") is True

    def test_tampered_check_digits_fail(self):
        assert QRReferenceGenerator.validate_rf("RF62AB") is False

    def test_accepts_lowercase_and_spaces(self):
        assert QRReferenceGenerator.validate_rf("rf61 ab") is True

    def test_missing_rf_prefix_fails(self):
        assert QRReferenceGenerator.validate_rf("XX43AB") is False

    def test_malformed_structure_fails(self):
        assert QRReferenceGenerator.validate_rf("RF") is False


# ─── Générateurs d'image (dépendances optionnelles) ──────────────────────────

class TestGenerateQrImage:

    def test_generates_png_bytes(self):
        qrcode = pytest.importorskip("qrcode")
        pytest.importorskip("PIL")
        payload = QRInvoiceGenerator.generate_payload(make_data())
        png = QRInvoiceGenerator.generate_qr_image(payload)
        assert isinstance(png, bytes)
        assert png.startswith(b"\x89PNG\r\n\x1a\n")

    def test_raises_helpful_error_without_qrcode(self, monkeypatch):
        import builtins
        real_import = builtins.__import__

        def fake_import(name, *args, **kwargs):
            if name == "qrcode":
                raise ImportError("simulated: qrcode not installed")
            return real_import(name, *args, **kwargs)

        monkeypatch.setattr(builtins, "__import__", fake_import)
        with pytest.raises(QRInvoiceError, match="pip install qrcode"):
            QRInvoiceGenerator.generate_qr_image("SPC")


class TestGenerateQrSvg:

    def test_generates_svg_with_swiss_cross(self):
        pytest.importorskip("qrcode")
        payload = QRInvoiceGenerator.generate_payload(make_data())
        svg = QRInvoiceGenerator.generate_qr_svg(payload)
        assert svg.strip().startswith("<")
        assert "#FF0000" in svg  # croix suisse injectée
        assert svg.strip().endswith("</svg>")
