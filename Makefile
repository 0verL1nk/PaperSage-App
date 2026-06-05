.PHONY: help dev dev-attach run stop status backend frontend check test install \
       install-backend install-frontend install-clsi check-clsi clsi-dev clean \
       logs log-backend log-frontend log-clsi \
       docker-build docker-push docker-login

# Colors
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
RED := \033[0;31m
NC := \033[0m

# Paths
PROC := ./scripts/proc.sh
PID_DIR := .pids
LOG_DIR := logs

# Default target
help:
	@echo "$(BLUE)PaperSage Makefile$(NC)"
	@echo ""
	@echo "$(GREEN)Development:$(NC)"
	@echo "  make dev          - Start all services with graceful management"
	@echo "  make stop          - Gracefully stop all running services"
	@echo "  make status        - Show status of all services"
	@echo "  make logs          - Tail logs from all services"
	@echo "  make run           - Start frontend + backend (no CLSI)"
	@echo ""
	@echo "$(GREEN)Individual services:$(NC)"
	@echo "  make backend       - Start backend only"
	@echo "  make frontend      - Start frontend only"
	@echo "  make clsi-dev      - Start CLSI LaTeX service only"
	@echo "  make log-backend   - Tail backend logs"
	@echo "  make log-frontend  - Tail frontend logs"
	@echo "  make log-clsi      - Tail CLSI logs"
	@echo ""
	@echo "$(GREEN)Quality:$(NC)"
	@echo "  make check         - Run all code quality checks"
	@echo "  make test          - Run all tests"
	@echo ""
	@echo "$(GREEN)Setup:$(NC)"
	@echo "  make install       - Install all dependencies"
	@echo "  make install-clsi  - Install CLSI prerequisites"
	@echo "  make check-clsi    - Verify CLSI prerequisites"
	@echo ""
	@echo "$(GREEN)Maintenance:$(NC)"
	@echo "  make clean         - Remove PID files, logs, and temp files"
	@echo ""
	@echo "$(GREEN)Docker:$(NC)"
	@echo "  make docker-login  - Login to Docker Hub"
	@echo "  make docker-build  - Build all images"
	@echo "  make docker-push   - Build & push all images to Docker Hub"
	@echo ""

# ─── Graceful dev lifecycle ───

# Start all services (backend, frontend, CLSI) with PID tracking and health checks
dev:
	@$(PROC) start

# Stop all services gracefully (SIGTERM → wait → SIGKILL)
stop:
	@$(PROC) stop || true

# Show running service status and PIDs
status:
	@$(PROC) status

# ─── Individual services (standalone, no PID tracking) ───

backend:
	@echo "$(YELLOW)Starting backend server...$(NC)"
	@echo "$(BLUE)Backend running at http://0.0.0.0:5478$(NC)"
	cd backend && uv run uvicorn app.main:app --reload --host 0.0.0.0 --port 5478

frontend:
	@echo "$(YELLOW)Starting frontend server...$(NC)"
	@echo "$(BLUE)Frontend running at http://0.0.0.0:5173$(NC)"
	cd frontend && pnpm dev --host

clsi-dev:
	@./scripts/clsi/dev.sh

# Legacy alias: frontend + backend without process management
run:
	@echo "$(GREEN)Starting PaperSage...$(NC)"
	@echo "$(BLUE)Frontend: http://localhost:5173$(NC)"
	@echo "$(BLUE)Backend:  http://localhost:5478$(NC)"
	@echo "$(YELLOW)Tip: use 'make dev' for graceful process management$(NC)"
	(cd backend && uv run uvicorn app.main:app --reload --port 5478) & \
	(cd frontend && pnpm dev)
	@wait

# ─── Log tailing ───

logs:
	@echo "$(BLUE)Tailing all service logs (Ctrl+C to stop)$(NC)"
	@tail -f $(LOG_DIR)/*.log 2>/dev/null || echo "$(YELLOW)No log files found. Run 'make dev' first.$(NC)"

log-backend:
	@tail -f $(LOG_DIR)/backend.log 2>/dev/null || echo "$(YELLOW)No backend log. Run 'make dev' first.$(NC)"

log-frontend:
	@tail -f $(LOG_DIR)/frontend.log 2>/dev/null || echo "$(YELLOW)No frontend log. Run 'make dev' first.$(NC)"

log-clsi:
	@tail -f $(LOG_DIR)/clsi.log 2>/dev/null || echo "$(YELLOW)No CLSI log. Run 'make dev' first.$(NC)"

# ─── Install ───

install: install-backend install-frontend install-clsi
	@echo "$(GREEN)All dependencies installed$(NC)"

install-backend:
	@echo "$(YELLOW)Installing backend dependencies...$(NC)"
	cd backend && uv sync --extra dev

install-frontend:
	@echo "$(YELLOW)Installing frontend dependencies...$(NC)"
	cd frontend && pnpm install

# ─── Quality ───

check: check-backend check-frontend
	@echo "$(GREEN)All checks passed!$(NC)"

check-backend:
	@echo "$(YELLOW)Running backend checks...$(NC)"
	cd backend && uvx ruff check app api core --fix
	cd backend && uvx ruff format --check .
	cd backend && uv run python -m py_compile app/ api/ || true

check-frontend:
	@echo "$(YELLOW)Running frontend checks...$(NC)"
	cd frontend && pnpm typecheck
	cd frontend && pnpm lint

# ─── Tests ───

test: test-backend test-frontend
	@echo "$(GREEN)All tests passed!$(NC)"

test-backend:
	@echo "$(YELLOW)Running backend tests...$(NC)"
	cd backend && uv run pytest tests/ -v

test-frontend:
	@echo "$(YELLOW)Running frontend tests...$(NC)"
	cd frontend && pnpm test

# ─── CLSI ───

install-clsi:
	@echo "$(YELLOW)Installing CLSI host-mode prerequisites...$(NC)"
	./scripts/clsi/install.sh

check-clsi:
	@echo "$(YELLOW)Checking CLSI prerequisites...$(NC)"
	./scripts/clsi/check.sh

# ─── Maintenance ───

clean:
	@echo "$(YELLOW)Cleaning runtime files...$(NC)"
	@rm -rf $(PID_DIR) $(LOG_DIR)
	@echo "$(GREEN)Cleaned $(PID_DIR)/ and $(LOG_DIR)/$(NC)"

# ─── Docker ───

DOCKERHUB_USER ?= $(shell docker info 2>/dev/null | grep Username | awk '{print $$2}')
IMAGE_TAG ?= latest

docker-login:
	@echo "$(YELLOW)Logging in to Docker Hub...$(NC)"
	@docker login

docker-build:
	@if [ -z "$(DOCKERHUB_USER)" ]; then \
		echo "$(RED)Error: not logged in to Docker Hub. Run 'make docker-login' first.$(NC)"; \
		exit 1; \
	fi
	@echo "$(GREEN)Building Docker images...$(NC)"
	docker build -t $(DOCKERHUB_USER)/papersage-backend:$(IMAGE_TAG) backend/
	docker build -t $(DOCKERHUB_USER)/papersage-frontend:$(IMAGE_TAG) frontend/
	docker build -t $(DOCKERHUB_USER)/papersage-render:$(IMAGE_TAG) latex_render/
	@echo "$(GREEN)All images built:$(NC)"
	@docker images --format "table {{.Repository}}:{{.Tag}}\t{{.Size}}" | grep papersage

docker-push: docker-build
	@echo "$(GREEN)Pushing images to Docker Hub...$(NC)"
	docker push $(DOCKERHUB_USER)/papersage-backend:$(IMAGE_TAG)
	docker push $(DOCKERHUB_USER)/papersage-frontend:$(IMAGE_TAG)
	docker push $(DOCKERHUB_USER)/papersage-render:$(IMAGE_TAG)
	@echo "$(GREEN)All images pushed to $(DOCKERHUB_USER)$(NC)"
