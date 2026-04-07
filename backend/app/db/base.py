"""
Base SQLAlchemy — Tous les modèles héritent de cette classe.
Inclut les champs d'audit obligatoires (CO art. 957a).
"""

from datetime import datetime, timezone
from typing import Any
from uuid import uuid4

from sqlalchemy import DateTime, String, func
from sqlalchemy.dialects.postgresql import UUID
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column


class Base(DeclarativeBase):
    pass


class TimestampMixin:
    """Champs de traçabilité temporelle — CO art. 957a."""

    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True),
        server_default=func.now(),
        nullable=False,
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True),
        server_default=func.now(),
        onupdate=func.now(),
        nullable=False,
    )


class UUIDPrimaryKey:
    """UUID comme clé primaire — évite les conflits en mode distribué/local."""

    id: Mapped[UUID] = mapped_column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid4,
        nullable=False,
    )


class AuditMixin(TimestampMixin, UUIDPrimaryKey):
    """
    Mixin complet : UUID + timestamps + utilisateur responsable.
    Tous les modèles métier doivent hériter de ce mixin.
    """
    pass
