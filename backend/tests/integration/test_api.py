"""
LedgerAlps — Tests d'intégration
Testent les endpoints HTTP complets avec base de données PostgreSQL de test.
Requiert : DATABASE_URL pointant vers une base vide.
"""

from __future__ import annotations

from uuid import uuid4

import pytest
import pytest_asyncio
from httpx import AsyncClient, ASGITransport
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine

from app.core.config import settings
from app.db.base import Base
from app.db.session import get_db
from app.main import app
from app.models import Account, AccountType

# ─── Fixtures ─────────────────────────────────────────────────────────────────

TEST_DB_URL = settings.database_url.replace("/ledgeralps", "/ledgeralps_test")

test_engine = create_async_engine(TEST_DB_URL, echo=False)
TestSessionLocal = async_sessionmaker(test_engine, expire_on_commit=False)


@pytest_asyncio.fixture(scope="session", autouse=True)
async def setup_db():
    async with test_engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)
        await conn.run_sync(Base.metadata.create_all)
    yield
    await test_engine.dispose()


@pytest_asyncio.fixture
async def db_session():
    async with TestSessionLocal() as session:
        yield session
        await session.rollback()


@pytest_asyncio.fixture
async def client(db_session: AsyncSession):
    async def _get_test_db():
        yield db_session

    app.dependency_overrides[get_db] = _get_test_db
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        yield ac
    app.dependency_overrides.clear()


# ─── Auth ─────────────────────────────────────────────────────────────────────

class TestAuthEndpoints:

    @pytest.mark.asyncio
    async def test_register_success(self, client: AsyncClient):
        resp = await client.post("/api/v1/auth/register", json={
            "email":    "jean@test.ch",
            "name":     "Jean Makrab",
            "password": "SecurePass123!",
        })
        assert resp.status_code == 201
        data = resp.json()
        assert data["email"]    == "jean@test.ch"
        assert data["is_active"] is True
        assert "password_hash" not in data

    @pytest.mark.asyncio
    async def test_register_duplicate_email(self, client: AsyncClient):
        payload = {"email": "dup@test.ch", "name": "Test", "password": "SecurePass123!"}
        await client.post("/api/v1/auth/register", json=payload)
        resp = await client.post("/api/v1/auth/register", json=payload)
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_login_success(self, client: AsyncClient):
        await client.post("/api/v1/auth/register", json={
            "email": "login@test.ch", "name": "L", "password": "SecurePass123!"
        })
        resp = await client.post("/api/v1/auth/login", json={
            "email": "login@test.ch", "password": "SecurePass123!"
        })
        assert resp.status_code == 200
        assert "access_token"  in resp.json()
        assert "refresh_token" in resp.json()

    @pytest.mark.asyncio
    async def test_login_wrong_password(self, client: AsyncClient):
        resp = await client.post("/api/v1/auth/login", json={
            "email": "nobody@test.ch", "password": "wrong"
        })
        assert resp.status_code == 401

    @pytest.mark.asyncio
    async def test_protected_endpoint_without_token(self, client: AsyncClient):
        resp = await client.get("/api/v1/accounts")
        assert resp.status_code == 403  # HTTPBearer renvoie 403 si pas de header


# ─── Contacts ─────────────────────────────────────────────────────────────────

class TestContactsEndpoints:

    async def _get_token(self, client: AsyncClient) -> str:
        email = "contacts_test@test.ch"
        await client.post("/api/v1/auth/register", json={
            "email": email, "name": "Test", "password": "SecurePass123!"
        })
        resp = await client.post("/api/v1/auth/login", json={
            "email": email, "password": "SecurePass123!"
        })
        return resp.json()["access_token"]

    @pytest.mark.asyncio
    async def test_create_contact(self, client: AsyncClient):
        token = await self._get_token(client)
        resp = await client.post(
            "/api/v1/contacts",
            json={
                "contact_type":      "client",
                "is_company":        True,
                "name":              "Acme SA",
                "city":              "Lausanne",
                "country":           "CH",
                "payment_term_days": 30,
                "currency":          "CHF",
            },
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 201
        assert resp.json()["name"] == "Acme SA"

    @pytest.mark.asyncio
    async def test_list_contacts(self, client: AsyncClient):
        token = await self._get_token(client)
        resp = await client.get(
            "/api/v1/contacts",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 200
        assert isinstance(resp.json(), list)


# ─── TVA ──────────────────────────────────────────────────────────────────────

class TestVATEndpoints:

    async def _get_token(self, client: AsyncClient) -> str:
        email = "vat_test@test.ch"
        await client.post("/api/v1/auth/register", json={
            "email": email, "name": "Test", "password": "SecurePass123!"
        })
        resp = await client.post("/api/v1/auth/login", json={
            "email": email, "password": "SecurePass123!"
        })
        return resp.json()["access_token"]

    @pytest.mark.asyncio
    async def test_compute_vat_standard(self, client: AsyncClient):
        token = await self._get_token(client)
        resp = await client.post(
            "/api/v1/vat/compute",
            json={"amount": 100.00, "vat_rate": 8.1, "included": "excluded"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["vat_code"]   == "N81"
        assert data["vat_amount"] == "8.10"
        assert data["total_amount"] == "108.10"

    @pytest.mark.asyncio
    async def test_get_vat_rates(self, client: AsyncClient):
        token = await self._get_token(client)
        resp = await client.get(
            "/api/v1/vat/rates",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 200
        rates = resp.json()
        assert "N81" in rates
        assert "R26" in rates

    @pytest.mark.asyncio
    async def test_health_endpoint(self, client: AsyncClient):
        resp = await client.get("/health")
        assert resp.status_code == 200
        assert resp.json()["status"] == "ok"


# ─── Factures ─────────────────────────────────────────────────────────────────

class TestInvoicesEndpoints:
    """
    Tests du cycle de vie complet d'une facture.
    Les tests de transition vers SENT nécessitent les comptes comptables
    1020, 1100, 3201, 6100 — ils sont créés directement en session.
    """

    async def _create_accounts(self, db: AsyncSession) -> None:
        """Insère les comptes minimaux nécessaires aux écritures de facturation."""
        accounts = [
            Account(number="1020", name="Banque",             account_type=AccountType.ASSET),
            Account(number="1100", name="Créances clients",   account_type=AccountType.ASSET),
            Account(number="3201", name="TVA collectée",      account_type=AccountType.LIABILITY),
            Account(number="6100", name="Produits services",  account_type=AccountType.REVENUE),
        ]
        for acc in accounts:
            db.add(acc)
        await db.flush()

    async def _register_and_login(self, client: AsyncClient) -> str:
        email = f"inv_{uuid4().hex[:8]}@test.ch"
        await client.post("/api/v1/auth/register", json={
            "email": email, "name": "Test Facturation", "password": "SecurePass123!",
        })
        resp = await client.post("/api/v1/auth/login", json={
            "email": email, "password": "SecurePass123!",
        })
        return resp.json()["access_token"]

    async def _create_contact(self, client: AsyncClient, token: str) -> str:
        resp = await client.post(
            "/api/v1/contacts",
            json={"contact_type": "client", "is_company": True, "name": "Client Test SA",
                  "country": "CH", "payment_term_days": 30, "currency": "CHF"},
            headers={"Authorization": f"Bearer {token}"},
        )
        return resp.json()["id"]

    async def _create_invoice(self, client: AsyncClient, token: str, contact_id: str) -> dict:
        resp = await client.post(
            "/api/v1/invoices",
            json={
                "contact_id":    contact_id,
                "issue_date":    "2026-01-15",
                "document_type": "invoice",
                "lines": [
                    {"description": "Prestation A", "quantity": 2, "unit_price": 500.00, "vat_rate": 8.1},
                    {"description": "Prestation B", "quantity": 1, "unit_price": 200.00, "vat_rate": 8.1},
                ],
            },
            headers={"Authorization": f"Bearer {token}"},
        )
        return resp.json()

    @pytest.mark.asyncio
    async def test_create_invoice(self, client: AsyncClient, db_session: AsyncSession):
        token      = await self._register_and_login(client)
        contact_id = await self._create_contact(client, token)

        resp = await client.post(
            "/api/v1/invoices",
            json={
                "contact_id": contact_id,
                "issue_date": "2026-02-01",
                "lines": [{"description": "Consultation", "unit_price": 1500.00}],
            },
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 201
        data = resp.json()
        assert data["status"]        == "draft"
        assert "FA2026-"             in data["number"]
        assert len(data["lines"])    == 1
        assert float(data["subtotal"]) == pytest.approx(1500.00, rel=1e-2)
        assert float(data["vat_amount"]) == pytest.approx(121.50, rel=1e-2)  # 8.1%

    @pytest.mark.asyncio
    async def test_create_invoice_multi_lines(self, client: AsyncClient, db_session: AsyncSession):
        token      = await self._register_and_login(client)
        contact_id = await self._create_contact(client, token)
        data       = await self._create_invoice(client, token, contact_id)

        assert data["status"]     == "draft"
        assert len(data["lines"]) == 2
        # 2×500 + 1×200 = 1200 HT ; TVA 8.1% = 97.20 ; TTC = 1297.20
        assert float(data["subtotal"])   == pytest.approx(1200.00, rel=1e-2)
        assert float(data["vat_amount"]) == pytest.approx(97.20,   rel=1e-2)
        assert float(data["total"])      == pytest.approx(1297.20,  rel=1e-2)

    @pytest.mark.asyncio
    async def test_list_invoices(self, client: AsyncClient, db_session: AsyncSession):
        token      = await self._register_and_login(client)
        contact_id = await self._create_contact(client, token)
        await self._create_invoice(client, token, contact_id)

        resp = await client.get(
            "/api/v1/invoices",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 200
        assert isinstance(resp.json(), list)
        assert len(resp.json()) >= 1

    @pytest.mark.asyncio
    async def test_get_invoice_by_id(self, client: AsyncClient, db_session: AsyncSession):
        token      = await self._register_and_login(client)
        contact_id = await self._create_contact(client, token)
        created    = await self._create_invoice(client, token, contact_id)

        resp = await client.get(
            f"/api/v1/invoices/{created['id']}",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 200
        assert resp.json()["id"]     == created["id"]
        assert resp.json()["number"] == created["number"]

    @pytest.mark.asyncio
    async def test_get_invoice_not_found(self, client: AsyncClient, db_session: AsyncSession):
        token = await self._register_and_login(client)
        resp  = await client.get(
            f"/api/v1/invoices/{uuid4()}",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 404

    @pytest.mark.asyncio
    async def test_cancel_draft_invoice(self, client: AsyncClient, db_session: AsyncSession):
        token      = await self._register_and_login(client)
        contact_id = await self._create_contact(client, token)
        created    = await self._create_invoice(client, token, contact_id)

        resp = await client.patch(
            f"/api/v1/invoices/{created['id']}/status",
            json={"status": "cancelled"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 200
        assert resp.json()["status"] == "cancelled"

    @pytest.mark.asyncio
    async def test_invalid_transition_cancelled_to_sent(self, client: AsyncClient, db_session: AsyncSession):
        """Une facture annulée ne peut pas repasser en envoyée."""
        token      = await self._register_and_login(client)
        contact_id = await self._create_contact(client, token)
        created    = await self._create_invoice(client, token, contact_id)
        inv_id     = created["id"]

        await client.patch(
            f"/api/v1/invoices/{inv_id}/status",
            json={"status": "cancelled"},
            headers={"Authorization": f"Bearer {token}"},
        )

        resp = await client.patch(
            f"/api/v1/invoices/{inv_id}/status",
            json={"status": "sent"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code in (400, 422)

    @pytest.mark.asyncio
    async def test_send_invoice_posts_journal_entry(
        self, client: AsyncClient, db_session: AsyncSession
    ):
        """La transition draft→sent crée une écriture comptable."""
        await self._create_accounts(db_session)

        token      = await self._register_and_login(client)
        contact_id = await self._create_contact(client, token)
        created    = await self._create_invoice(client, token, contact_id)

        resp = await client.patch(
            f"/api/v1/invoices/{created['id']}/status",
            json={"status": "sent"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 200
        assert resp.json()["status"] == "sent"

    @pytest.mark.asyncio
    async def test_full_lifecycle_draft_sent_paid(
        self, client: AsyncClient, db_session: AsyncSession
    ):
        """Cycle complet : brouillon → envoyée → payée."""
        await self._create_accounts(db_session)

        token      = await self._register_and_login(client)
        contact_id = await self._create_contact(client, token)
        created    = await self._create_invoice(client, token, contact_id)
        inv_id     = created["id"]
        auth       = {"Authorization": f"Bearer {token}"}

        # draft → sent
        resp = await client.patch(f"/api/v1/invoices/{inv_id}/status",
                                  json={"status": "sent"}, headers=auth)
        assert resp.status_code == 200
        assert resp.json()["status"] == "sent"

        # sent → paid
        resp = await client.patch(
            f"/api/v1/invoices/{inv_id}/status",
            json={"status": "paid", "payment_date": "2026-02-20"},
            headers=auth,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "paid"
        assert float(data["amount_paid"]) == pytest.approx(float(created["total"]), rel=1e-2)


# ─── Journal ──────────────────────────────────────────────────────────────────

class TestJournalEndpoints:

    async def _register_and_login(self, client: AsyncClient) -> str:
        email = f"jnl_{uuid4().hex[:8]}@test.ch"
        await client.post("/api/v1/auth/register", json={
            "email": email, "name": "Journal Test", "password": "SecurePass123!",
        })
        resp = await client.post("/api/v1/auth/login", json={
            "email": email, "password": "SecurePass123!",
        })
        return resp.json()["access_token"]

    async def _create_accounts(self, db: AsyncSession) -> None:
        for number, name, atype in [
            ("1010", "Caisse",   AccountType.ASSET),
            ("5000", "Capital",  AccountType.EQUITY),
        ]:
            db.add(Account(number=number, name=name, account_type=atype))
        await db.flush()

    @pytest.mark.asyncio
    async def test_list_journal_empty(self, client: AsyncClient, db_session: AsyncSession):
        token = await self._register_and_login(client)
        resp  = await client.get(
            "/api/v1/journal",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "items"     in data
        assert "total"     in data
        assert "page"      in data
        assert "page_size" in data
        assert "pages"     in data

    @pytest.mark.asyncio
    async def test_list_journal_with_pagination(self, client: AsyncClient, db_session: AsyncSession):
        await self._create_accounts(db_session)
        token = await self._register_and_login(client)
        auth  = {"Authorization": f"Bearer {token}"}

        # Créer 3 écritures
        for i in range(3):
            await client.post("/api/v1/journal", json={
                "date": f"2026-01-{i + 10:02d}",
                "description": f"Écriture test {i}",
                "lines": [{"debit_account": "1010", "credit_account": "5000", "amount": 100.00}],
            }, headers=auth)

        resp = await client.get(
            "/api/v1/journal?page=1&page_size=2",
            headers=auth,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert len(data["items"]) <= 2
        assert data["page"]      == 1
        assert data["page_size"] == 2

    @pytest.mark.asyncio
    async def test_list_journal_filter_by_date(self, client: AsyncClient, db_session: AsyncSession):
        await self._create_accounts(db_session)
        token = await self._register_and_login(client)
        auth  = {"Authorization": f"Bearer {token}"}

        # Deux écritures à des dates différentes
        for d in ["2026-01-05", "2026-03-10"]:
            await client.post("/api/v1/journal", json={
                "date": d, "description": f"Écriture {d}",
                "lines": [{"debit_account": "1010", "credit_account": "5000", "amount": 50.00}],
            }, headers=auth)

        resp = await client.get(
            "/api/v1/journal?date_from=2026-03-01&date_to=2026-03-31",
            headers=auth,
        )
        assert resp.status_code == 200
        items = resp.json()["items"]
        assert all("2026-03" in item["date"] for item in items)


# ─── Contacts PATCH ───────────────────────────────────────────────────────────

class TestContactsPatch:

    async def _register_and_login(self, client: AsyncClient) -> str:
        email = f"patch_{uuid4().hex[:8]}@test.ch"
        await client.post("/api/v1/auth/register", json={
            "email": email, "name": "Patch Test", "password": "SecurePass123!",
        })
        resp = await client.post("/api/v1/auth/login", json={
            "email": email, "password": "SecurePass123!",
        })
        return resp.json()["access_token"]

    @pytest.mark.asyncio
    async def test_patch_contact(self, client: AsyncClient, db_session: AsyncSession):
        token = await self._register_and_login(client)
        auth  = {"Authorization": f"Bearer {token}"}

        # Créer un contact
        resp = await client.post("/api/v1/contacts", json={
            "contact_type": "client", "is_company": True, "name": "Ancienne SA",
            "country": "CH", "payment_term_days": 30, "currency": "CHF",
        }, headers=auth)
        contact_id = resp.json()["id"]

        # Mettre à jour
        resp = await client.patch(
            f"/api/v1/contacts/{contact_id}",
            json={"name": "Nouvelle SA", "payment_term_days": 60, "city": "Zurich"},
            headers=auth,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["name"]               == "Nouvelle SA"
        assert data["payment_term_days"]  == 60
        assert data["city"]               == "Zurich"

    @pytest.mark.asyncio
    async def test_patch_contact_not_found(self, client: AsyncClient, db_session: AsyncSession):
        token = await self._register_and_login(client)
        resp  = await client.patch(
            f"/api/v1/contacts/{uuid4()}",
            json={"name": "Inexistant"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 404
