"""
LedgerAlps — QR-facture suisse
Standard : Swiss Payment Standards 2.0 (Six-Group / STUZZA)
https://www.six-group.com/en/products-services/banking-services/payment-standardization/standards/qr-bill.html

Éléments :
  - QR-IBAN (format CH / LI, 26 chiffres avec check digit)
  - Référence QRR (27 chiffres) ou RF (ISO 11649)
  - Payload Swiss QR Code (SPC, version 0200)
  - Arrondi 0.05 CHF obligatoire
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from decimal import Decimal, ROUND_HALF_UP


# ─── Constantes Six-Group ─────────────────────────────────────────────────────

QR_TYPE = "SPC"           # Swiss Payments Code
QR_VERSION = "0200"
QR_CODING = "1"           # UTF-8
SUPPORTED_CURRENCIES = {"CHF", "EUR"}
MAX_AMOUNT = Decimal("999999999.99")
MAX_MESSAGE_LENGTH = 140
MAX_ADDITIONAL_INFO_LENGTH = 140


# ─── Structures de données ────────────────────────────────────────────────────

@dataclass
class QRAddress:
    """Adresse au format Six-Group (type S = structuré, type K = combiné)."""
    name: str                           # max 70 chars
    address_type: str = "S"             # S = structured, K = combined
    street_or_address_line1: str = ""   # max 70 chars
    building_number_or_address_line2: str = ""  # max 16 / 70 chars
    postal_code: str = ""               # max 16 chars (requis si type S)
    city: str = ""                      # max 35 chars (requis si type S)
    country: str = "CH"                 # ISO 3166-1 alpha-2

    def validate(self) -> None:
        if len(self.name) > 70:
            raise QRInvoiceError(f"Nom trop long (max 70): {self.name!r}")
        if self.country not in _VALID_COUNTRIES:
            raise QRInvoiceError(f"Code pays invalide: {self.country!r}")
        if self.address_type == "S" and (not self.postal_code or not self.city):
            raise QRInvoiceError("Adresse structurée : NPA et ville requis.")

    def to_lines(self) -> list[str]:
        return [
            self.address_type,
            self.name,
            self.street_or_address_line1,
            self.building_number_or_address_line2,
            self.postal_code,
            self.city,
            self.country,
        ]


@dataclass
class QRInvoiceData:
    """Données complètes de la QR-facture."""
    creditor_iban: str                  # QR-IBAN ou IBAN
    creditor: QRAddress
    amount: Decimal | None              # None = montant ouvert
    currency: str                       # CHF ou EUR
    debtor: QRAddress | None = None     # Optionnel
    reference_type: str = "NON"         # QRR, SCOR (RF), NON
    reference: str = ""                 # QRR 27 chiffres ou RF
    unstructured_message: str = ""      # max 140 chars
    additional_info: str = ""           # max 140 chars (trailer "EPD")
    alternative_procedures: list[str] = field(default_factory=list)  # max 2 × 100 chars


class QRInvoiceError(Exception):
    pass


# ─── Générateur QR-facture ────────────────────────────────────────────────────

class QRInvoiceGenerator:
    """
    Génère le payload texte du Swiss QR Code et le QR code image.
    Le payload est le contenu exact à encoder dans le QR code.
    """

    @staticmethod
    def generate_payload(data: QRInvoiceData) -> str:
        """
        Génère le payload textuel selon la spec Six-Group §3.3.
        Séparateur : \\n (LF), encodage UTF-8.
        """
        QRInvoiceGenerator._validate(data)

        amount_str = ""
        if data.amount is not None:
            rounded = _round_to_5_rappen(data.amount)
            amount_str = f"{rounded:.2f}"

        # Débiteur vide si non fourni
        empty_address = QRAddress(name="", address_type="S")
        debtor = data.debtor or empty_address

        lines = [
            QR_TYPE,
            QR_VERSION,
            QR_CODING,
            # Compte créancier
            data.creditor_iban,
            # Créancier
            *data.creditor.to_lines(),
            # Créancier final (vide — réservé)
            "", "", "", "", "", "", "",
            # Montant
            amount_str,
            data.currency,
            # Débiteur
            *debtor.to_lines(),
            # Référence
            data.reference_type,
            data.reference,
            # Informations supplémentaires
            data.unstructured_message,
            "EPD",  # End Payment Data — obligatoire
            data.additional_info,
        ]

        # Procédures alternatives (max 2)
        for proc in data.alternative_procedures[:2]:
            lines.append(proc[:100])

        return "\n".join(lines)

    @staticmethod
    def generate_qr_image(payload: str, box_size: int = 3) -> bytes:
        """
        Génère l'image QR code PNG (bytes).
        Intègre la croix suisse au centre selon spec Six-Group §3.5.
        Nécessite : qrcode[pil], Pillow
        """
        try:
            import qrcode
            from qrcode.constants import ERROR_CORRECT_M
            from PIL import Image, ImageDraw
            import io
        except ImportError:
            raise QRInvoiceError(
                "Dépendances manquantes : pip install qrcode[pil] Pillow"
            )

        qr = qrcode.QRCode(
            version=None,
            error_correction=ERROR_CORRECT_M,
            box_size=box_size,
            border=0,
        )
        qr.add_data(payload)
        qr.make(fit=True)
        img = qr.make_image(fill_color="black", back_color="white").convert("RGB")

        # Croix suisse au centre (spec Six-Group : 7×7 mm dans un QR de 46×46 mm)
        w, h = img.size
        cross_w = int(w * 0.15)
        cross_h = int(h * 0.15)
        cx = (w - cross_w) // 2
        cy = (h - cross_h) // 2

        cross = Image.new("RGB", (cross_w, cross_h), "white")
        draw = ImageDraw.Draw(cross)
        arm_w = cross_w // 3
        arm_h = cross_h // 3
        # Barre horizontale
        draw.rectangle([0, arm_h, cross_w, cross_h - arm_h], fill="red")
        # Barre verticale
        draw.rectangle([arm_w, 0, cross_w - arm_w, cross_h], fill="red")

        img.paste(cross, (cx, cy))

        buf = io.BytesIO()
        img.save(buf, format="PNG", dpi=(300, 300))
        return buf.getvalue()

    @staticmethod
    def _validate(data: QRInvoiceData) -> None:
        """Validation complète selon Swiss Payment Standards §3."""
        # IBAN
        iban = data.creditor_iban.replace(" ", "").upper()
        if not _validate_iban(iban):
            raise QRInvoiceError(f"IBAN invalide : {data.creditor_iban!r}")

        # QR-IBAN obligatoire pour référence QRR
        is_qr_iban = _is_qr_iban(iban)
        if data.reference_type == "QRR" and not is_qr_iban:
            raise QRInvoiceError("La référence QRR exige un QR-IBAN (IID 30000–31999).")
        if data.reference_type == "SCOR" and is_qr_iban:
            raise QRInvoiceError("La référence RF (SCOR) n'est pas compatible avec un QR-IBAN.")

        # Référence QRR
        if data.reference_type == "QRR":
            clean_ref = data.reference.replace(" ", "")
            if not re.fullmatch(r"\d{27}", clean_ref):
                raise QRInvoiceError("Référence QRR invalide : doit contenir exactement 27 chiffres.")
            if not _validate_qrr_check_digit(clean_ref):
                raise QRInvoiceError("Chiffre de contrôle QRR invalide.")

        # Référence RF (ISO 11649)
        if data.reference_type == "SCOR":
            if not re.fullmatch(r"RF\d{2}[A-Z0-9]{1,21}", data.reference.replace(" ", "").upper()):
                raise QRInvoiceError("Référence RF invalide.")

        # Montant
        if data.amount is not None:
            if data.amount < Decimal("0.01"):
                raise QRInvoiceError("Le montant minimum est 0.01.")
            if data.amount > MAX_AMOUNT:
                raise QRInvoiceError(f"Montant maximum dépassé : {MAX_AMOUNT}")

        # Devise
        if data.currency not in SUPPORTED_CURRENCIES:
            raise QRInvoiceError(f"Devise non supportée : {data.currency!r}. Utilisez CHF ou EUR.")

        # Messages
        if len(data.unstructured_message) > MAX_MESSAGE_LENGTH:
            raise QRInvoiceError(f"Message trop long (max {MAX_MESSAGE_LENGTH} chars).")

        # Adresses
        data.creditor.validate()
        if data.debtor:
            data.debtor.validate()


# ─── Génération des références ────────────────────────────────────────────────

class QRReferenceGenerator:
    """Génère et valide les références QRR (27 chiffres) et RF (ISO 11649)."""

    @staticmethod
    def generate_qrr(customer_ref: str, participant_id: str = "000000000") -> str:
        """
        Génère une référence QRR à 27 chiffres.
        Format : [participant_id 9d][ref_padded 16d][check 1d] (+ espaces pour affichage)
        """
        # Construire la partie numérique (26 chiffres sans check digit)
        raw = f"{participant_id[:9]:>09}"
        ref_numeric = re.sub(r"[^0-9]", "", customer_ref)
        raw += f"{ref_numeric:>017}"[:17]  # 9 + 17 = 26 chiffres avant le check digit
        raw = raw[:26]

        # Calcul check digit (algorithme modulo 10 récursif)
        check = _mod10_recursive(raw)
        full = raw + str(check)

        # Formatage avec espaces : 2-5-5-5-5-5
        return (f"{full[0:2]} {full[2:7]} {full[7:12]} "
                f"{full[12:17]} {full[17:22]} {full[22:27]}")

    @staticmethod
    def generate_rf(creditor_ref: str) -> str:
        """
        Génère une référence RF selon ISO 11649.
        Format : RF[check_2d][ref_alphanumeric max 21 chars]
        """
        ref = re.sub(r"[^A-Z0-9]", "", creditor_ref.upper())[:21]
        # Calcul checksum ISO 11649
        check = _rf_checksum(ref)
        return f"RF{check:02d}{ref}"

    @staticmethod
    def format_qrr_display(qrr: str) -> str:
        """Formatage lisible pour impression : XX XXXXX XXXXX XXXXX XXXXX XXXXX."""
        digits = re.sub(r"\s", "", qrr)
        if len(digits) != 27:
            return qrr
        return (f"{digits[0:2]} {digits[2:7]} {digits[7:12]} "
                f"{digits[12:17]} {digits[17:22]} {digits[22:27]}")


# ─── Utilitaires IBAN ─────────────────────────────────────────────────────────

def _validate_iban(iban: str) -> bool:
    """Validation IBAN par modulo 97 (ISO 13616)."""
    iban = iban.replace(" ", "").upper()
    if not re.fullmatch(r"[A-Z]{2}[0-9]{2}[A-Z0-9]+", iban):
        return False
    rearranged = iban[4:] + iban[:4]
    numeric = "".join(str(ord(c) - 55) if c.isalpha() else c for c in rearranged)
    return int(numeric) % 97 == 1


def _is_qr_iban(iban: str) -> bool:
    """
    Un QR-IBAN CH/LI a un IID (positions 5–9) entre 30000 et 31999.
    Exemple : CH44 3100 0000 0012 3456 7
    """
    iban = iban.replace(" ", "").upper()
    if not iban.startswith(("CH", "LI")):
        return False
    try:
        iid = int(iban[4:9])
        return 30000 <= iid <= 31999
    except ValueError:
        return False


def _validate_qrr_check_digit(ref27: str) -> bool:
    """Valide le chiffre de contrôle modulo 10 récursif d'une référence QRR."""
    if len(ref27) != 27:
        return False
    return _mod10_recursive(ref27[:26]) == int(ref27[26])


def _mod10_recursive(digits: str) -> int:
    """Algorithme modulo 10 récursif (table de Luhn suisse)."""
    TABLE = [0, 9, 4, 6, 8, 2, 7, 1, 3, 5]
    carry = 0
    for d in digits:
        carry = TABLE[(carry + int(d)) % 10]
    return (10 - carry) % 10


def _rf_checksum(ref: str) -> int:
    """Calcule les 2 chiffres de contrôle ISO 11649 (RF)."""
    rearranged = ref + "RF00"
    numeric = "".join(str(ord(c) - 55) if c.isalpha() else c for c in rearranged)
    return 98 - (int(numeric) % 97)


def _round_to_5_rappen(amount: Decimal) -> Decimal:
    """Arrondi suisse obligatoire au 0.05 CHF pour QR-facture."""
    return (amount / Decimal("0.05")).quantize(Decimal("1"), rounding=ROUND_HALF_UP) * Decimal("0.05")


# ─── Liste ISO 3166-1 alpha-2 minimale ───────────────────────────────────────
_VALID_COUNTRIES = {
    "CH", "LI", "DE", "AT", "FR", "IT", "BE", "NL", "LU", "GB",
    "ES", "PT", "SE", "NO", "DK", "FI", "PL", "CZ", "HU", "RO",
    "US", "CA", "AU", "JP", "CN", "SG", "AE", "SA",
}
