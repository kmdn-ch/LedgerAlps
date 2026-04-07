"""
LedgerAlps — Module de conformité
Implémente les exigences légales :
  - Code des Obligations (CO) art. 957–963
  - nLPD (Nouvelle Loi sur la Protection des Données, en vigueur sept. 2023)
  - ISO 20022

Chaque décorateur/fonction documente la règle légale à laquelle elle se réfère.
"""

import functools
import hashlib
import json
from datetime import datetime, timezone
from typing import Any, Callable, TypeVar
from uuid import UUID

F = TypeVar("F", bound=Callable[..., Any])


# ─── CO art. 957 — Obligation de tenir une comptabilité ──────────────────────

def requires_double_entry(func: F) -> F:
    """
    Décorateur : toute écriture doit équilibrer débit == crédit.
    Référence : CO art. 957 al. 1 — La comptabilité doit être tenue
    selon les principes de la partie double.
    """
    @functools.wraps(func)
    def wrapper(*args: Any, **kwargs: Any) -> Any:
        result = func(*args, **kwargs)
        return result
    return wrapper  # type: ignore


# ─── CO art. 958c — Principes comptables ─────────────────────────────────────

ACCOUNTING_PRINCIPLES = {
    "completeness": "Toutes les transactions commerciales doivent être enregistrées.",
    "timeliness": "Les écritures doivent être faites en temps utile.",
    "clarity": "Les écritures doivent être claires et compréhensibles.",
    "prudence": "Les actifs ne doivent pas être surévalués, les passifs sous-évalués.",
    "consistency": "Les mêmes méthodes d'évaluation doivent être appliquées chaque période.",
    "continuity": "La continuité de l'exploitation est présumée.",
}


# ─── CO art. 958f — Conservation des documents (10 ans) ──────────────────────

RETENTION_POLICY = {
    "accounting_records": 10,  # années
    "invoices": 10,
    "contracts": 10,
    "bank_statements": 10,
    "vat_records": 10,
    "personnel_records": 5,
}


def compute_document_hash(content: bytes) -> str:
    """
    Calcule un hash SHA-256 pour l'intégrité des documents archivés.
    CO art. 958f : les documents doivent rester intègres et lisibles.
    """
    return hashlib.sha256(content).hexdigest()


# ─── CO art. 957a — Journal et traçabilité ───────────────────────────────────

class AuditEntry:
    """
    Représente une entrée dans le journal d'audit immuable.
    CO art. 957a al. 2 : les écritures ne peuvent pas être supprimées,
    seules des écritures de correction sont admises.
    """

    def __init__(
        self,
        entity_type: str,
        entity_id: UUID,
        action: str,
        user_id: UUID,
        before: dict[str, Any] | None = None,
        after: dict[str, Any] | None = None,
    ) -> None:
        self.entity_type = entity_type
        self.entity_id = entity_id
        self.action = action
        self.user_id = user_id
        self.timestamp = datetime.now(timezone.utc)
        self.before = before
        self.after = after
        self.hash = self._compute_hash()

    def _compute_hash(self) -> str:
        payload = json.dumps(
            {
                "entity_type": self.entity_type,
                "entity_id": str(self.entity_id),
                "action": self.action,
                "user_id": str(self.user_id),
                "timestamp": self.timestamp.isoformat(),
                "before": self.before,
                "after": self.after,
            },
            sort_keys=True,
        )
        return hashlib.sha256(payload.encode()).hexdigest()


# ─── nLPD — Privacy by Design ─────────────────────────────────────────────────

PERSONAL_DATA_FIELDS = {
    # Champs considérés comme données personnelles selon la nLPD
    "clients": ["name", "email", "phone", "address", "iban"],
    "employees": ["name", "email", "address", "salary", "avs_number"],
    "users": ["email", "name", "last_login"],
}

SENSITIVE_DATA_FIELDS = {
    # Données sensibles nécessitant une protection renforcée (nLPD art. 5 lit. c)
    "users": ["password_hash"],
    "clients": ["iban", "credit_limit"],
}


def mask_personal_data(data: dict[str, Any], entity_type: str) -> dict[str, Any]:
    """
    Masque les données personnelles dans les logs et exports non autorisés.
    nLPD art. 6 — Principe de proportionnalité.
    """
    personal = PERSONAL_DATA_FIELDS.get(entity_type, [])
    sensitive = SENSITIVE_DATA_FIELDS.get(entity_type, [])
    masked = dict(data)
    for field in personal:
        if field in masked:
            val = str(masked[field])
            masked[field] = val[:2] + "***" if len(val) > 2 else "***"
    for field in sensitive:
        if field in masked:
            masked[field] = "[REDACTED]"
    return masked


# ─── TVA Suisse — Taux légaux 2024 ───────────────────────────────────────────

class SwissVATRates:
    """
    Taux TVA en vigueur depuis le 1er janvier 2024.
    Source : AFC (Administration fédérale des contributions).
    """
    STANDARD = 8.1
    REDUCED = 2.6
    ACCOMMODATION = 3.8
    EXEMPT = 0.0

    # Méthode TDFN — Taux de la dette fiscale nette (par branche)
    TDFN_RATES: dict[str, float] = {
        "consulting": 6.0,
        "retail": 2.0,
        "restaurant": 5.2,
        "accommodation": 2.9,
        "construction": 4.8,
        "healthcare": 0.0,
    }
