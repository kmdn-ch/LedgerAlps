"""
LedgerAlps — Middlewares de sécurité
  - Rate limiting par IP (SlowAPI / Redis-compatible)
  - Security headers (OWASP recommandations)
  - Request/response logging pour l'audit (nLPD)
  - Blocage des méthodes HTTP non autorisées
"""

from __future__ import annotations

import hashlib
import time
import logging
from collections import defaultdict
from datetime import datetime, timezone
from typing import Callable

from fastapi import Request, Response, status
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware

logger = logging.getLogger("ledgeralps.security")


# ─── Rate Limiter (in-process, adapté au local-first) ────────────────────────

class RateLimiter:
    """
    Rate limiter par IP — fenêtre glissante.
    En production avec Redis, remplacer par slowapi + redis.
    """

    def __init__(self) -> None:
        self._windows: dict[str, list[float]] = defaultdict(list)

    def is_allowed(
        self,
        key: str,
        limit: int,
        window_seconds: int,
    ) -> tuple[bool, int]:
        now   = time.monotonic()
        cutoff = now - window_seconds
        hits   = self._windows[key]

        # Purger les entrées expirées
        hits[:] = [t for t in hits if t > cutoff]

        remaining = max(0, limit - len(hits))
        if len(hits) >= limit:
            return False, remaining

        hits.append(now)
        return True, remaining - 1


_rate_limiter = RateLimiter()

# Limites par endpoint (requests / fenêtre)
RATE_LIMITS: dict[str, tuple[int, int]] = {
    "/api/v1/auth/login":    (10, 60),    # 10 req / 60 s — protection brute force
    "/api/v1/auth/register": (5,  3600),  # 5 req / heure
    "default":               (200, 60),   # 200 req / min pour les autres
}


class RateLimitMiddleware(BaseHTTPMiddleware):

    async def dispatch(self, request: Request, call_next: Callable) -> Response:
        ip  = request.client.host if request.client else "unknown"
        path = request.url.path

        limit_cfg = RATE_LIMITS.get(path, RATE_LIMITS["default"])
        limit, window = limit_cfg
        key = f"{ip}:{path}"

        allowed, remaining = _rate_limiter.is_allowed(key, limit, window)

        if not allowed:
            logger.warning("Rate limit dépassé : ip=%s path=%s", ip, path)
            return JSONResponse(
                status_code=status.HTTP_429_TOO_MANY_REQUESTS,
                content={"detail": "Trop de requêtes. Réessayez dans un moment."},
                headers={
                    "Retry-After":         str(window),
                    "X-RateLimit-Limit":   str(limit),
                    "X-RateLimit-Remaining": "0",
                },
            )

        response = await call_next(request)
        response.headers["X-RateLimit-Limit"]     = str(limit)
        response.headers["X-RateLimit-Remaining"] = str(remaining)
        return response


# ─── Security Headers (OWASP) ─────────────────────────────────────────────────

class SecurityHeadersMiddleware(BaseHTTPMiddleware):
    """
    Ajoute les headers de sécurité recommandés par OWASP.
    nLPD : protège contre XSS, clickjacking, MIME sniffing.
    """

    HEADERS = {
        "X-Content-Type-Options":  "nosniff",
        "X-Frame-Options":         "DENY",
        "X-XSS-Protection":        "1; mode=block",
        "Referrer-Policy":         "strict-origin-when-cross-origin",
        "Permissions-Policy":      "geolocation=(), microphone=(), camera=()",
        "Content-Security-Policy": (
            "default-src 'self'; "
            "script-src 'self' 'unsafe-inline'; "
            "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "
            "font-src 'self' https://fonts.gstatic.com; "
            "img-src 'self' data:; "
            "connect-src 'self';"
        ),
        "Strict-Transport-Security": "max-age=31536000; includeSubDomains",
    }

    async def dispatch(self, request: Request, call_next: Callable) -> Response:
        response = await call_next(request)
        for header, value in self.HEADERS.items():
            response.headers[header] = value
        # Supprimer les headers qui révèlent la stack
        response.headers.pop("Server", None)
        response.headers.pop("X-Powered-By", None)
        return response


# ─── Audit Request Logging (nLPD art. 8 — traçabilité) ───────────────────────

class AuditLogMiddleware(BaseHTTPMiddleware):
    """
    Journalise les requêtes sensibles (authentification, modifications).
    Ne logue jamais le body des requêtes (données personnelles — nLPD).
    """

    SENSITIVE_PATHS = {
        "/api/v1/auth/login",
        "/api/v1/auth/register",
    }

    MUTATION_METHODS = {"POST", "PUT", "PATCH", "DELETE"}

    async def dispatch(self, request: Request, call_next: Callable) -> Response:
        start = time.monotonic()
        response = await call_next(request)
        duration_ms = int((time.monotonic() - start) * 1000)

        method = request.method
        path   = request.url.path
        status_code = response.status_code
        ip     = request.client.host if request.client else "—"

        # Logger les appels sensibles et mutations
        if path in self.SENSITIVE_PATHS or method in self.MUTATION_METHODS:
            # Masquer l'IP pour les logs (nLPD — proportionnalité)
            masked_ip = _mask_ip(ip)
            logger.info(
                "%s %s %d %dms ip=%s",
                method, path, status_code, duration_ms, masked_ip,
            )

        # Alerter sur les échecs d'auth
        if path in self.SENSITIVE_PATHS and status_code in (401, 403):
            logger.warning("Auth failure: %s %s %d ip=%s", method, path, status_code, _mask_ip(ip))

        return response


def _mask_ip(ip: str) -> str:
    """Masque partiellement l'IP pour la conformité nLPD."""
    parts = ip.split(".")
    if len(parts) == 4:
        return f"{parts[0]}.{parts[1]}.x.x"
    return "x:x:x"
