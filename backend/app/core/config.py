"""
LedgerAlps — Configuration centrale
Chargée depuis les variables d'environnement ou un fichier .env
"""

from pydantic import Field, SecretStr, field_validator
from pydantic_settings import BaseSettings, SettingsConfigDict
from typing import Literal


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=False,
    )

    # ─── Application ──────────────────────────────────────────────────────────
    app_name: str = "LedgerAlps"
    app_version: str = "0.1.0"
    debug: bool = False
    log_level: Literal["DEBUG", "INFO", "WARNING", "ERROR"] = "INFO"
    api_v1_prefix: str = "/api/v1"

    # ─── Base de données ──────────────────────────────────────────────────────
    database_url: str = Field(
        default="postgresql+asyncpg://ledgeralps:changeme@localhost:5432/ledgeralps"
    )
    db_pool_size: int = 10
    db_max_overflow: int = 20
    db_echo: bool = False  # True en dev pour voir les requêtes SQL

    # ─── Sécurité (nLPD) ──────────────────────────────────────────────────────
    secret_key: SecretStr = Field(default="CHANGE_ME_IN_PRODUCTION_32_CHARS_MIN")
    algorithm: str = "HS256"
    access_token_expire_minutes: int = 60
    refresh_token_expire_days: int = 30

    # ─── Données suisses ──────────────────────────────────────────────────────
    default_currency: str = "CHF"
    default_locale: str = "fr_CH"
    default_timezone: str = "Europe/Zurich"
    vat_standard_rate: float = 8.1    # Taux normal TVA CH 2024
    vat_reduced_rate: float = 2.6     # Taux réduit TVA CH 2024
    vat_accommodation_rate: float = 3.8  # Taux hébergement TVA CH 2024

    # ─── Archivage légal (CO art. 962) ────────────────────────────────────────
    document_retention_years: int = 10
    data_dir: str = "./data"
    export_dir: str = "./data/exports"
    archive_dir: str = "./data/archives"

    @field_validator("secret_key")
    @classmethod
    def validate_secret_key(cls, v: SecretStr) -> SecretStr:
        if len(v.get_secret_value()) < 32:
            raise ValueError("SECRET_KEY doit contenir au moins 32 caractères")
        return v


settings = Settings()
