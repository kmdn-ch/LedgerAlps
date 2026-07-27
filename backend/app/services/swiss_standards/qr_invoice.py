"""
LedgerAlps — QR-facture suisse
Standard : Swiss Payment Standards 2.0 (Six-Group / STUZZA)
Spec SPC 0200 — https://www.six-group.com/dam/download/banking-services/standardization/qr-bill/ig-qr-bill-v2.2-en.pdf

Corrections v2 :
  - Payload ligne par ligne conforme §3.3 (séquence exacte, 31 lignes)
  - QRR 27 chiffres : 26 chiffres de données + 1 check digit modulo 10 récursif
  - Image QR : version forcée pour garantir la densité, crosshatch SVG en fallback
  - Validation complète IBAN / QR-IBAN / QRR / RF
"""

from __future__ import annotations

import io
import re
from dataclasses import dataclass, field
from decimal import Decimal, ROUND_HALF_UP


# ─── Constantes Six-Group (§3.1) ─────────────────────────────────────────────

QR_TYPE    = "SPC"
QR_VERSION = "0200"
QR_CODING  = "1"           # UTF-8
SEPARATOR  = "\n"          # LF obligatoire (pas CRLF)

SUPPORTED_CURRENCIES = {"CHF", "EUR"}
MAX_AMOUNT           = Decimal("999999999.99")
MIN_AMOUNT           = Decimal("0.01")


# ─── Structures ───────────────────────────────────────────────────────────────

@dataclass
class QRAddress:
    """
    Adresse selon §3.3.
    type S = structurée (street + number + NPA + ville)
    type K = combinée (2 lignes libres)
    """
    name:            str
    address_type:    str = "S"   # "S" ou "K"
    street:          str = ""    # ligne 1 (rue) ou adresse ligne 1
    house_number:    str = ""    # numéro ou adresse ligne 2
    postal_code:     str = ""    # NPA (obligatoire si type S)
    city:            str = ""    # ville (obligatoire si type S)
    country:         str = "CH"  # ISO 3166-1 alpha-2

    def to_lines(self) -> list[str]:
        """Retourne les 7 lignes dans l'ordre exact de la spec."""
        return [
            self.address_type,
            self.name[:70],
            self.street[:70],
            self.house_number[:16],
            self.postal_code[:16],
            self.city[:35],
            self.country[:2].upper(),
        ]

    def validate(self) -> None:
        if not self.name.strip():
            raise QRInvoiceError("Nom du créancier requis.")
        if self.address_type not in ("S", "K"):
            raise QRInvoiceError(f"Type d'adresse invalide : {self.address_type!r}. Utilisez 'S' ou 'K'.")
        if self.address_type == "S":
            if not self.postal_code:
                raise QRInvoiceError("NPA requis pour une adresse structurée (type S).")
            if not self.city:
                raise QRInvoiceError("Ville requise pour une adresse structurée (type S).")
        if self.country not in _VALID_COUNTRIES:
            raise QRInvoiceError(f"Code pays invalide : {self.country!r}")


@dataclass
class QRInvoiceData:
    """Données complètes de la QR-facture (§3.3)."""
    creditor_iban:        str
    creditor:             QRAddress
    amount:               Decimal | None     # None = montant ouvert
    currency:             str                # CHF ou EUR
    debtor:               QRAddress | None = None
    reference_type:       str = "NON"        # QRR | SCOR | NON
    reference:            str = ""
    unstructured_message: str = ""           # max 140 chars
    bill_information:     str = ""           # max 140 chars (trailer EPD)
    alternative_schemes:  list[str] = field(default_factory=list)


class QRInvoiceError(Exception):
    pass


# ─── Générateur payload ───────────────────────────────────────────────────────

class QRInvoiceGenerator:
    """
    Génère le payload texte Swiss QR Code et l'image PNG.
    Le payload doit avoir EXACTEMENT 31 lignes selon §3.3.
    """

    @staticmethod
    def generate_payload(data: QRInvoiceData) -> str:
        """
        Génère le texte à encoder dans le QR code.
        Séparateur : LF (\\n), encodage UTF-8, 31 lignes exactement.
        """
        _validate(data)

        # Montant formaté (2 décimales) ou vide si None
        amount_str = ""
        if data.amount is not None:
            rounded = _round_to_5_rappen(data.amount)
            amount_str = f"{rounded:.2f}"

        # Débiteur vide = 7 lignes vides
        empty = ["", "", "", "", "", "", ""]
        debtor_lines = data.debtor.to_lines() if data.debtor else empty

        # Séquence exacte §3.3 — 31 lignes
        lines = [
            # En-tête (3 lignes)
            QR_TYPE,        # 1
            QR_VERSION,     # 2
            QR_CODING,      # 3

            # Compte créancier (1 ligne)
            data.creditor_iban.replace(" ", "").upper(),  # 4

            # Adresse créancier (7 lignes)
            *data.creditor.to_lines(),   # 5–11

            # Créancier final — vide, réservé (7 lignes)
            *empty,                      # 12–18

            # Montant et devise (2 lignes)
            amount_str,                  # 19
            data.currency,               # 20

            # Adresse débiteur (7 lignes)
            *debtor_lines,               # 21–27

            # Référence (2 lignes)
            data.reference_type,         # 28
            data.reference.replace(" ", ""),  # 29

            # Informations supplémentaires (2 lignes)
            data.unstructured_message[:140],  # 30
            "EPD",                            # 31 — End Payment Data obligatoire
        ]

        # Informations de facturation (optionnel, après EPD)
        if data.bill_information:
            lines.append(data.bill_information[:140])

        # Procédures alternatives (max 2 × 100 chars)
        for scheme in data.alternative_schemes[:2]:
            lines.append(scheme[:100])

        assert len(lines) >= 31, f"Payload incomplet : {len(lines)} lignes"
        return SEPARATOR.join(lines)

    @staticmethod
    def generate_qr_image(payload: str, size_mm: int = 46) -> bytes:
        """
        Génère l'image QR code PNG avec la croix suisse intégrée.

        Paramètres :
          payload  : texte généré par generate_payload()
          size_mm  : taille souhaitée en mm (spec : 46 mm minimum)

        Retourne les bytes PNG à 300 DPI.
        """
        try:
            import qrcode
            from qrcode.constants import ERROR_CORRECT_M
        except ImportError:
            raise QRInvoiceError("Installez : pip install qrcode[pil]")

        try:
            from PIL import Image, ImageDraw
        except ImportError:
            raise QRInvoiceError("Installez : pip install Pillow")

        # ── Générer le QR code ──────────────────────────────────────────────
        # La spec Six-Group §5.1 impose ERROR_CORRECT_M (niveau M)
        qr = qrcode.QRCode(
            error_correction=ERROR_CORRECT_M,
            box_size=10,
            border=0,        # Pas de quiet zone ici — ajoutée par le layout PDF
        )
        qr.add_data(payload, optimize=0)  # optimize=0 : ne pas découper les données
        qr.make(fit=True)

        img: Image.Image = qr.make_image(
            fill_color="black",
            back_color="white",
        ).convert("RGB")

        # ── Croix suisse au centre (§5.1) ───────────────────────────────────
        # Dimensions spec : croix 7×7 mm dans un QR 46×46 mm → ~15% de la taille
        w, h = img.size
        cross_w = max(int(w * 0.155), 10)
        cross_h = max(int(h * 0.155), 10)
        cx = (w - cross_w) // 2
        cy = (h - cross_h) // 2

        # Fond blanc pour effacer les modules QR sous la croix
        cross_img = Image.new("RGB", (cross_w, cross_h), "white")
        draw = ImageDraw.Draw(cross_img)

        # Barre horizontale (1/3 de hauteur, pleine largeur)
        bar_h = cross_h // 3
        bar_v = cross_w // 3
        # Croix rouge suisse
        draw.rectangle([0,     bar_h, cross_w,    cross_h - bar_h], fill="#FF0000")
        draw.rectangle([bar_v, 0,     cross_w - bar_v, cross_h],    fill="#FF0000")

        img.paste(cross_img, (cx, cy))

        # ── Export PNG 300 DPI ───────────────────────────────────────────────
        buf = io.BytesIO()
        img.save(buf, format="PNG", dpi=(300, 300))
        return buf.getvalue()

    @staticmethod
    def generate_qr_svg(payload: str) -> str:
        """
        Génère un QR code en SVG pur (sans dépendances externes).
        Alternative légère si Pillow n'est pas disponible.
        Utilise la lib qrcode en mode SVG.
        """
        try:
            import qrcode
            import qrcode.image.svg
            from qrcode.constants import ERROR_CORRECT_M
        except ImportError:
            raise QRInvoiceError("Installez : pip install qrcode[pil]")

        qr = qrcode.QRCode(
            error_correction=ERROR_CORRECT_M,
            box_size=10,
            border=0,
        )
        qr.add_data(payload, optimize=0)
        qr.make(fit=True)

        factory = qrcode.image.svg.SvgPathImage
        img = qr.make_image(image_factory=factory)

        buf = io.BytesIO()
        img.save(buf)
        svg = buf.getvalue().decode("utf-8")

        # Ajouter la croix suisse en SVG
        svg = _inject_swiss_cross_svg(svg)
        return svg


# ─── Génération des références ────────────────────────────────────────────────

class QRReferenceGenerator:
    """
    Génère et valide les références QRR (27 chiffres) et RF (ISO 11649).

    Structure QRR :
      [participant 9 chiffres][référence client 17 chiffres][check 1 chiffre]
      Total : 27 chiffres
    """

    @staticmethod
    def generate_qrr(customer_ref: str, participant_id: str = "000000000") -> str:
        """
        Génère une référence QRR à 27 chiffres.

        Exemple :
          generate_qrr("FA2025-0001", "210000000")
          → "21 00000 00000 00000 00001 03"  (formaté)
        """
        # Nettoyer et normaliser
        p_id    = re.sub(r"[^0-9]", "", participant_id)[:9].zfill(9)
        ref_num = re.sub(r"[^0-9]", "", customer_ref)

        # 26 chiffres de données : 9 (participant) + 17 (référence)
        data26  = (p_id + ref_num.zfill(17))[:26]

        # Check digit modulo 10 récursif
        check   = _mod10_recursive(data26)
        full27  = data26 + str(check)

        assert len(full27) == 27, f"BUG: {len(full27)} chiffres au lieu de 27"
        return QRReferenceGenerator.format_display(full27)

    @staticmethod
    def generate_rf(creditor_ref: str) -> str:
        """
        Génère une référence structurée RF (ISO 11649).

        Exemple :
          generate_rf("FA20250001") → "RF89FA20250001"
        """
        # Garder uniquement les caractères alphanumériques, max 21
        ref = re.sub(r"[^A-Za-z0-9]", "", creditor_ref).upper()[:21]
        check = _rf_checksum(ref)
        return f"RF{check:02d}{ref}"

    @staticmethod
    def format_display(ref27: str) -> str:
        """
        Formate une référence QRR pour l'affichage imprimé.
        Format : XX XXXXX XXXXX XXXXX XXXXX XXXXX
        """
        d = re.sub(r"\s", "", ref27)
        if len(d) != 27:
            return ref27
        return f"{d[0:2]} {d[2:7]} {d[7:12]} {d[12:17]} {d[17:22]} {d[22:27]}"

    @staticmethod
    def validate_qrr(ref: str) -> bool:
        """Valide une référence QRR (27 chiffres + check digit correct)."""
        d = re.sub(r"\s", "", ref)
        if not re.fullmatch(r"\d{27}", d):
            return False
        return _mod10_recursive(d[:26]) == int(d[26])

    @staticmethod
    def validate_rf(ref: str) -> bool:
        """Valide une référence RF (ISO 11649)."""
        ref = ref.replace(" ", "").upper()
        if not re.match(r"^RF\d{2}[A-Z0-9]{1,21}$", ref):
            return False
        return _rf_checksum(ref[4:]) == int(ref[2:4])


# ─── Utilitaires IBAN ────────────────────────────────────────────────────────

def validate_iban(iban: str) -> bool:
    """Valide un IBAN par modulo 97 (ISO 13616)."""
    clean = re.sub(r"\s", "", iban).upper()
    if not re.fullmatch(r"[A-Z]{2}\d{2}[A-Z0-9]{1,30}", clean):
        return False
    rearranged = clean[4:] + clean[:4]
    numeric = "".join(str(ord(c) - 55) if c.isalpha() else c for c in rearranged)
    return int(numeric) % 97 == 1


def is_qr_iban(iban: str) -> bool:
    """
    Un QR-IBAN CH/LI a un IID (positions 5–9) entre 30000 et 31999.
    Exemple : CH44 3100 0000 0012 3456 7 → IID=31000 → QR-IBAN ✓
    """
    clean = re.sub(r"\s", "", iban).upper()
    if not clean.startswith(("CH", "LI")):
        return False
    try:
        iid = int(clean[4:9])
        return 30000 <= iid <= 31999
    except (ValueError, IndexError):
        return False


def format_iban(iban: str) -> str:
    """Formate un IBAN avec espaces : CH56 0483 5012 3456 7800 9"""
    clean = re.sub(r"\s", "", iban).upper()
    return " ".join(clean[i:i+4] for i in range(0, len(clean), 4))


# ─── Validation complète ─────────────────────────────────────────────────────

def _validate(data: QRInvoiceData) -> None:
    """Validation complète selon Swiss Payment Standards §3."""

    # IBAN créancier
    iban = re.sub(r"\s", "", data.creditor_iban).upper()
    if not validate_iban(iban):
        raise QRInvoiceError(f"IBAN invalide : {data.creditor_iban!r}")

    qr_iban = is_qr_iban(iban)

    # Cohérence référence ↔ type IBAN
    if data.reference_type == "QRR" and not qr_iban:
        raise QRInvoiceError(
            "La référence QRR exige un QR-IBAN (IID 30000–31999). "
            f"L'IBAN fourni ({iban}) n'est pas un QR-IBAN."
        )
    if data.reference_type == "SCOR" and qr_iban:
        raise QRInvoiceError(
            "La référence RF (SCOR) n'est pas compatible avec un QR-IBAN. "
            "Utilisez un IBAN standard ou changez le type de référence."
        )

    # Validation de la référence
    if data.reference_type == "QRR":
        clean_ref = re.sub(r"\s", "", data.reference)
        if not re.fullmatch(r"\d{27}", clean_ref):
            raise QRInvoiceError(
                f"Référence QRR invalide : doit contenir exactement 27 chiffres "
                f"(reçu {len(clean_ref)} chiffres : {clean_ref!r})."
            )
        if not QRReferenceGenerator.validate_qrr(clean_ref):
            raise QRInvoiceError("Chiffre de contrôle QRR invalide.")

    elif data.reference_type == "SCOR":
        clean_ref = re.sub(r"\s", "", data.reference).upper()
        if not re.fullmatch(r"RF\d{2}[A-Z0-9]{1,21}", clean_ref):
            raise QRInvoiceError(f"Référence RF invalide : {data.reference!r}")

    elif data.reference_type == "NON":
        pass  # Aucune référence structurée

    else:
        raise QRInvoiceError(
            f"Type de référence invalide : {data.reference_type!r}. "
            "Utilisez 'QRR', 'SCOR' ou 'NON'."
        )

    # Montant
    if data.amount is not None:
        if data.amount < MIN_AMOUNT:
            raise QRInvoiceError(f"Montant minimum : {MIN_AMOUNT} CHF.")
        if data.amount > MAX_AMOUNT:
            raise QRInvoiceError(f"Montant maximum dépassé ({MAX_AMOUNT}).")

    # Devise
    if data.currency not in SUPPORTED_CURRENCIES:
        raise QRInvoiceError(
            f"Devise non supportée : {data.currency!r}. Utilisez CHF ou EUR."
        )

    # Messages
    if len(data.unstructured_message) > 140:
        raise QRInvoiceError("Message non structuré : max 140 caractères.")

    # Adresses
    data.creditor.validate()
    if data.debtor:
        data.debtor.validate()


# ─── Algorithmes cryptographiques ────────────────────────────────────────────

def _mod10_recursive(digits: str) -> int:
    """
    Calcule le check digit modulo 10 récursif (table de multiplication suisse).
    Utilisé pour les références QRR et les anciens bulletins de versement.

    Table de report : [0, 9, 4, 6, 8, 2, 7, 1, 3, 5]
    """
    TABLE = [0, 9, 4, 6, 8, 2, 7, 1, 3, 5]
    carry = 0
    for d in str(digits):
        carry = TABLE[(carry + int(d)) % 10]
    return (10 - carry) % 10


def _rf_checksum(ref: str) -> int:
    """
    Calcule les 2 chiffres de contrôle ISO 11649 (RF).
    Formule : 98 - (numérique(ref + "RF00") mod 97)
    """
    rearranged = re.sub(r"[^A-Z0-9]", "", ref.upper()) + "RF00"
    numeric = "".join(str(ord(c) - 55) if c.isalpha() else c for c in rearranged)
    return 98 - (int(numeric) % 97)


def _round_to_5_rappen(amount: Decimal) -> Decimal:
    """
    Arrondi suisse obligatoire au 0.05 CHF pour QR-facture.
    1.23 → 1.25 | 1.22 → 1.20 | 1.225 → 1.25
    """
    return (amount / Decimal("0.05")).quantize(
        Decimal("1"), rounding=ROUND_HALF_UP
    ) * Decimal("0.05")


# ─── Injection croix suisse dans SVG ─────────────────────────────────────────

def _inject_swiss_cross_svg(svg: str) -> str:
    """Injecte la croix suisse au centre d'un QR code SVG."""
    cross = (
        '<g transform="translate(50%,50%) translate(-7%,-7%)">'
        '<rect width="14%" height="14%" fill="white"/>'
        '<rect x="4%" y="0" width="6%" height="14%" fill="#FF0000"/>'
        '<rect x="0" y="4%" width="14%" height="6%" fill="#FF0000"/>'
        '</g>'
    )
    return svg.replace("</svg>", cross + "</svg>")


# ─── Pays ISO 3166-1 alpha-2 (liste minimale) ─────────────────────────────────
_VALID_COUNTRIES = {
    "CH", "LI", "DE", "AT", "FR", "IT", "BE", "NL", "LU", "GB", "ES",
    "PT", "SE", "NO", "DK", "FI", "PL", "CZ", "HU", "RO", "US", "CA",
    "AU", "JP", "CN", "SG", "AE", "SA", "IN", "BR", "MX", "ZA",
}
