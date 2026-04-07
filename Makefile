.PHONY: help run backend frontend check test install install-backend install-frontend

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
	@echo "  make run        - Start both frontend and backend"
	@echo "  make backend    - Start backend only"
	@echo "  make frontend   - Start frontend only"
	@echo "  make check      - Run ruff/ty for backend, typecheck/lint for frontend"
	@echo "  make test       - Run ty for backend, vitest for frontend"
	@echo "  make install    - Install all dependencies"
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

# Run development servers
run: backend frontend
	@echo "$(GREEN)Both servers started!$(NC)"
	@echo "$(BLUE)Frontend: http://localhost:5173$(NC)"
	@echo "$(BLUE)Backend:  http://localhost:8000$(NC)"

backend:
	@echo "$(YELLOW)Starting backend server...$(NC)"
	@echo "$(BLUE)Backend running at http://localhost:8000$(NC)"
	cd backend && uv run uvicorn app.main:app --reload --port 8000

frontend:
	@echo "$(YELLOW)Starting frontend server...$(NC)"
	@echo "$(BLUE)Frontend running at http://localhost:5173$(NC)"
	cd frontend && pnpm dev

# Code quality checks
check: check-backend check-frontend
	@echo "$(GREEN)All checks passed!$(NC)"

check-backend:
	@echo "$(YELLOW)Running backend checks...$(NC)"
	@echo "Running ruff..."
	cd backend && uvx ruff check app/ api/ core/
	@echo "Running ty..."
	cd backend && uvx ty check --ignore=unresolved-reference --ignore=invalid-argument-type --ignore=unresolved-attribute --ignore=invalid-await

check-frontend:
	@echo "$(YELLOW)Running frontend checks...$(NC)"
	cd frontend && pnpm typecheck
	cd frontend && pnpm lint

# Tests
test: test-backend test-frontend
	@echo "$(GREEN)All tests passed!$(NC)"

test-backend:
	@echo "$(YELLOW)Running backend type checks (ty)...$(NC)"
	cd backend && uvx ty check --ignore=unresolved-reference --ignore=invalid-argument-type --ignore=unresolved-attribute --ignore=invalid-await

test-frontend:
	@echo "$(YELLOW)Running frontend tests...$(NC)"
	cd frontend && pnpm test