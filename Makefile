.PHONY: help up down logs shell-backend shell-db migrate seed test lint

help:
	@echo "LedgerAlps — Commandes développeur"
	@echo ""
	@echo "  make up          Démarrer tous les services"
	@echo "  make down        Arrêter tous les services"
	@echo "  make logs        Suivre les logs"
	@echo "  make migrate     Appliquer les migrations Alembic"
	@echo "  make seed        Charger le plan comptable initial"
	@echo "  make test        Lancer la suite de tests"
	@echo "  make lint        Vérifier le code (ruff + mypy)"
	@echo "  make shell-db    Shell psql"

up:
	cp -n .env.example .env 2>/dev/null || true
	docker compose up -d --build
	@echo "Backend:  http://localhost:8000"
	@echo "Frontend: http://localhost:5173"
	@echo "API docs: http://localhost:8000/api/docs"

down:
	docker compose down

logs:
	docker compose logs -f

migrate:
	docker compose exec backend alembic upgrade head

seed:
	docker compose exec backend python -m app.db.seeds.chart_of_accounts

test:
	docker compose exec backend pytest -v --cov=app --cov-report=term-missing

lint:
	docker compose exec backend ruff check app/
	docker compose exec backend mypy app/

shell-backend:
	docker compose exec backend bash

shell-db:
	docker compose exec db psql -U ledgeralps
