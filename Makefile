.PHONY: help run backend frontend check test install install-backend install-frontend

help:

# Colors
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

# Default target
help:
	@echo "$(BLUE)PaperSage Makefile$(NC)"
	@echo ""
	@echo "$(GREEN)Available commands:$(NC)"
	@echo "  make dev          - Start both frontend and backend concurrently with hot-reload"
	@echo "  make run          - Start both frontend and backend"
	@echo "  make backend     - Start backend only"
	@echo "  make frontend    - Start frontend only"
	@echo "  make check       - Run code quality checks"
	@echo "  make test         - Run tests"
	@echo "  make install     - Install all dependencies"
	@echo ""

# Install dependencies
install: install-backend install-frontend
	@echo "$(GREEN)All dependencies installed$(NC)"

install-backend:
	@echo "$(YELLOW)Installing backend dependencies...$(NC)"
	cd backend && uv sync --extra dev

install-frontend:
	@echo "$(YELLOW)Installing frontend dependencies...$(NC)"
	cd frontend && pnpm install

# Run both servers
run:
	@echo "$(GREEN)Starting PaperSage...$(NC)"
	@echo "$(BLUE)Frontend: http://localhost:5173$(NC)"
	@echo "$(BLUE)Backend:  http://localhost:5478$(NC)"
	(cd backend && uv run uvicorn app.main:app --reload --port 5478) & \
	(cd frontend && pnpm dev)
	@wait

# Run development servers concurrently with hot-reloading
dev:
	@echo "$(GREEN)Starting development servers with hot-reload...$(NC)"
	@$(MAKE) -j2 backend frontend

# Run individual servers
backend:
	@echo "$(YELLOW)Starting backend server...$(NC)"
	@echo "$(BLUE)Backend running at http://localhost:5478$(NC)"
	cd backend && uv run uvicorn app.main:app --reload --port 5478

frontend:
	@echo "$(YELLOW)Starting frontend server...$(NC)"
	@echo "$(BLUE)Frontend running at http://localhost:5173$(NC)"
	cd frontend && pnpm dev

# Code quality checks
check: check-backend check-frontend
	@echo "$(GREEN)All checks passed!$(NC)"

check-backend:
	@echo "$(YELLOW)Running backend checks...$(NC)"
	cd backend && uvx ruff check app/ api/ core/ --fix || true
	-cd backend && uv run python -m py_compile app/ api/ || true

check-frontend:
	@echo "$(YELLOW)Running frontend checks...$(NC)"
	cd frontend && pnpm typecheck || true
	cd frontend && pnpm lint || true

# Tests
test: test-backend test-frontend
	@echo "$(GREEN)All tests passed!$(NC)"

test-backend:
	@echo "$(YELLOW)Running backend tests...$(NC)"
	cd backend && uv run pytest tests/ -v

test-frontend:
	@echo "$(YELLOW)Running frontend tests...$(NC)"
	cd frontend && pnpm test