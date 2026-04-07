"""
LedgerAlps — Point d'entrée FastAPI (Phase 5 — Production)
"""

from contextlib import asynccontextmanager
from collections.abc import AsyncGenerator
import logging
import sys

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.core.config import settings
from app.db.session import engine
from app.db.base import Base
from app.middleware.security import (
    RateLimitMiddleware, SecurityHeadersMiddleware, AuditLogMiddleware,
)
from app.api.v1.endpoints.auth import router as auth_router
from app.api.v1.endpoints.main import (
    accounts_router, contacts_router, invoices_router, journal_router, vat_router,
)
from app.api.v1.endpoints.swiss import (
    qr_router, iso_router, pdf_router, export_router,
)

logging.basicConfig(
    level=getattr(logging, settings.log_level),
    format="%(asctime)s %(levelname)-8s %(name)s — %(message)s",
    handlers=[logging.StreamHandler(sys.stdout)],
)
logger = logging.getLogger("ledgeralps")


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncGenerator[None, None]:
    logger.info("LedgerAlps %s démarrage…", settings.app_version)
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    logger.info("Base de données initialisée.")
    yield
    await engine.dispose()
    logger.info("LedgerAlps arrêt.")


app = FastAPI(
    title=settings.app_name,
    version=settings.app_version,
    description="Comptabilité suisse — CO, nLPD, ISO 20022, QR-facture",
    docs_url="/api/docs" if settings.debug else None,
    redoc_url="/api/redoc" if settings.debug else None,
    openapi_url="/api/openapi.json" if settings.debug else None,
    lifespan=lifespan,
)

app.add_middleware(SecurityHeadersMiddleware)
app.add_middleware(AuditLogMiddleware)
app.add_middleware(RateLimitMiddleware)
app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:5173", "http://localhost:3000"] if settings.debug else [],
    allow_credentials=True,
    allow_methods=["GET", "POST", "PUT", "PATCH", "DELETE"],
    allow_headers=["Authorization", "Content-Type"],
)

PREFIX = settings.api_v1_prefix
app.include_router(auth_router,     prefix=PREFIX)
app.include_router(accounts_router, prefix=PREFIX)
app.include_router(contacts_router, prefix=PREFIX)
app.include_router(invoices_router, prefix=PREFIX)
app.include_router(journal_router,  prefix=PREFIX)
app.include_router(vat_router,      prefix=PREFIX)
app.include_router(qr_router,       prefix=PREFIX)
app.include_router(iso_router,      prefix=PREFIX)
app.include_router(pdf_router,      prefix=PREFIX)
app.include_router(export_router,   prefix=PREFIX)


@app.get("/health", tags=["system"])
async def health() -> dict:
    return {"status": "ok", "version": settings.app_version, "debug": settings.debug}
