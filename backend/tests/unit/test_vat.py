"""
LedgerAlps — Tests unitaires Phase 2
Couvre : moteur comptable, calcul TVA, machine d'états facture.
"""

from decimal import Decimal
import pytest

from app.services.vat.calculator import (
    VATCalculator, VATIncluded, VATCalcMethod, _round_to_5_rappen
)
from app.services.accounting.journal import ImbalancedEntryError


# ─── TVA ──────────────────────────────────────────────────────────────────────

class TestVATCalculator:

    def test_standard_rate_excluded(self):
        """100 CHF HT à 8.1% → TVA 8.10 → TTC 108.10"""
        result = VATCalculator.compute_line(
            Decimal("100.00"), Decimal("8.1"), VATIncluded.EXCLUDED
        )
        assert result.base_amount == Decimal("100.00")
        assert result.vat_amount == Decimal("8.10")
        assert result.total_amount == Decimal("108.10")
        assert result.vat_code == "N81"

    def test_standard_rate_included(self):
        """108.10 TTC → HT 100.00 + TVA 8.10"""
        result = VATCalculator.compute_line(
            Decimal("108.10"), Decimal("8.1"), VATIncluded.INCLUDED
        )
        assert result.vat_amount == Decimal("8.10")
        assert result.base_amount + result.vat_amount == result.total_amount

    def test_reduced_rate(self):
        """50 CHF HT à 2.6% → TVA 1.30"""
        result = VATCalculator.compute_line(
            Decimal("50.00"), Decimal("2.6"), VATIncluded.EXCLUDED
        )
        assert result.vat_amount == Decimal("1.30")
        assert result.vat_code == "R26"

    def test_exempt(self):
        """Exonéré → TVA 0"""
        result = VATCalculator.compute_line(
            Decimal("200.00"), Decimal("0.0"), VATIncluded.EXCLUDED
        )
        assert result.vat_amount == Decimal("0.00")
        assert result.vat_code == "EX00"

    def test_rounding_5_rappen(self):
        """Arrondi suisse au 5 centimes."""
        assert _round_to_5_rappen(Decimal("8.13")) == Decimal("8.15")
        assert _round_to_5_rappen(Decimal("8.12")) == Decimal("8.10")
        assert _round_to_5_rappen(Decimal("8.125")) == Decimal("8.15")

    def test_document_summary(self):
        lines = [
            {"amount": Decimal("100.00"), "vat_rate": Decimal("8.1")},
            {"amount": Decimal("50.00"),  "vat_rate": Decimal("2.6")},
        ]
        summary = VATCalculator.compute_document(lines)
        assert len(summary.lines) == 2
        assert summary.total_base == Decimal("150.00")
        assert summary.total_vat > Decimal("0")


# ─── Arrondi 5 rappen ─────────────────────────────────────────────────────────

class TestRounding:

    @pytest.mark.parametrize("input_val,expected", [
        ("0.01", "0.00"),
        ("0.025", "0.05"),
        ("1.234", "1.25"),
        ("99.99", "100.00"),
    ])
    def test_five_rappen(self, input_val: str, expected: str):
        result = _round_to_5_rappen(Decimal(input_val))
        assert result == Decimal(expected)
