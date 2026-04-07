"""
LedgerAlps — Moteur de calcul TVA suisse
Supporte :
  1. Méthode effective (taux réels)
  2. Méthode TDFN (taux de la dette fiscale nette)

Source légale : LTVA (RS 641.20) + ordonnances AFC
"""

from __future__ import annotations

from dataclasses import dataclass, field
from decimal import Decimal, ROUND_HALF_UP
from enum import Enum

from app.core.compliance import SwissVATRates


CHF2 = Decimal("0.01")
CHF4 = Decimal("0.0001")


class VATCalcMethod(str, Enum):
    EFFECTIVE = "effective"
    TDFN = "tdfn"


class VATIncluded(str, Enum):
    INCLUDED = "included"    # Prix TTC — calculer la TVA en dedans
    EXCLUDED = "excluded"    # Prix HT — ajouter la TVA


@dataclass
class VATLine:
    """Résultat du calcul TVA pour une ligne de facture."""
    base_amount: Decimal        # Montant HT
    vat_rate: Decimal           # Taux appliqué (%)
    vat_amount: Decimal         # Montant TVA
    total_amount: Decimal       # Montant TTC
    vat_code: str               # Code TVA (N81, R26, H38, EX00)
    method: VATCalcMethod


@dataclass
class VATSummary:
    """Récapitulatif TVA d'un document — structure pour déclaration AFC."""
    lines: list[VATLine] = field(default_factory=list)

    @property
    def total_base(self) -> Decimal:
        return sum(l.base_amount for l in self.lines).quantize(CHF2)

    @property
    def total_vat(self) -> Decimal:
        return sum(l.vat_amount for l in self.lines).quantize(CHF2)

    @property
    def total_ttc(self) -> Decimal:
        return sum(l.total_amount for l in self.lines).quantize(CHF2)

    def by_rate(self) -> dict[Decimal, dict]:
        """Regroupement par taux — format déclaration AFC."""
        groups: dict[Decimal, dict] = {}
        for line in self.lines:
            r = line.vat_rate
            if r not in groups:
                groups[r] = {"base": Decimal("0"), "vat": Decimal("0"), "code": line.vat_code}
            groups[r]["base"] += line.base_amount
            groups[r]["vat"] += line.vat_amount
        return {k: {
            "base": v["base"].quantize(CHF2),
            "vat": v["vat"].quantize(CHF2),
            "code": v["code"],
        } for k, v in groups.items()}


# ─── Codes TVA suisses ────────────────────────────────────────────────────────

VAT_CODES: dict[str, dict] = {
    "N81": {"rate": Decimal("8.1"),  "name": "Taux normal 8.1%"},
    "R26": {"rate": Decimal("2.6"),  "name": "Taux réduit 2.6%"},
    "H38": {"rate": Decimal("3.8"),  "name": "Hébergement 3.8%"},
    "EX00": {"rate": Decimal("0.0"), "name": "Exonéré / hors champ"},
}


def rate_to_code(rate: Decimal) -> str:
    for code, data in VAT_CODES.items():
        if data["rate"] == rate:
            return code
    return "N81"  # Fallback taux normal


# ─── Calculateur TVA — méthode effective ─────────────────────────────────────

class VATCalculator:

    @staticmethod
    def compute_line(
        amount: Decimal,
        vat_rate: Decimal,
        included: VATIncluded = VATIncluded.EXCLUDED,
        method: VATCalcMethod = VATCalcMethod.EFFECTIVE,
    ) -> VATLine:
        """
        Calcule la TVA pour un montant donné.

        Exemple taux normal 8.1% :
          HT 100.00 → TVA 8.10 → TTC 108.10
          TTC 108.10 → HT 99.9954... → TVA 8.10 (arrondi 5 rappen)
        """
        rate = vat_rate / Decimal("100")

        if included == VATIncluded.EXCLUDED:
            base = amount.quantize(CHF2, rounding=ROUND_HALF_UP)
            vat = (base * rate).quantize(CHF2, rounding=ROUND_HALF_UP)
            total = base + vat
        else:
            # TTC → HT : base = TTC / (1 + rate)
            total = amount.quantize(CHF2, rounding=ROUND_HALF_UP)
            base = (total / (1 + rate)).quantize(CHF2, rounding=ROUND_HALF_UP)
            vat = total - base

        # Arrondi 5 centimes (pratique suisse)
        vat = _round_to_5_rappen(vat)
        total = base + vat

        return VATLine(
            base_amount=base,
            vat_rate=vat_rate,
            vat_amount=vat,
            total_amount=total,
            vat_code=rate_to_code(vat_rate),
            method=method,
        )

    @staticmethod
    def compute_document(
        lines: list[dict],  # [{"amount": Decimal, "vat_rate": Decimal, "included": VATIncluded}]
        method: VATCalcMethod = VATCalcMethod.EFFECTIVE,
    ) -> VATSummary:
        """Calcule la TVA pour l'ensemble des lignes d'un document."""
        summary = VATSummary()
        for line in lines:
            vat_line = VATCalculator.compute_line(
                amount=Decimal(str(line["amount"])),
                vat_rate=Decimal(str(line.get("vat_rate", SwissVATRates.STANDARD))),
                included=line.get("included", VATIncluded.EXCLUDED),
                method=method,
            )
            summary.lines.append(vat_line)
        return summary


# ─── Méthode TDFN ─────────────────────────────────────────────────────────────

class TDFNCalculator:
    """
    Taux de la dette fiscale nette (TDFN).
    Option simplifiée pour PME dont le CA < 5,005,000 CHF.
    Source : AFC, liste des taux TDFN par branche.
    """

    @staticmethod
    def compute(
        turnover_ttc: Decimal,
        tdfn_rate: Decimal,
    ) -> dict:
        """
        TVA due = CA TTC × taux TDFN
        Exemple : CA TTC 100'000 × 6.0% (conseil) = TVA due 6'000 CHF
        """
        vat_due = (turnover_ttc * (tdfn_rate / Decimal("100"))).quantize(CHF2, rounding=ROUND_HALF_UP)
        return {
            "turnover_ttc": turnover_ttc.quantize(CHF2),
            "tdfn_rate": tdfn_rate,
            "vat_due": vat_due,
            "method": VATCalcMethod.TDFN,
        }

    @staticmethod
    def available_sectors() -> dict[str, float]:
        return SwissVATRates.TDFN_RATES


# ─── Utilitaire arrondi 5 rappen ─────────────────────────────────────────────

def _round_to_5_rappen(amount: Decimal) -> Decimal:
    """
    Arrondi commercial suisse au 5 centimes (0.05 CHF).
    Ex: 8.13 → 8.15, 8.12 → 8.10
    """
    return (amount / Decimal("0.05")).quantize(Decimal("1"), rounding=ROUND_HALF_UP) * Decimal("0.05")
