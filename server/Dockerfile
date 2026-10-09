# PaperSage server — single-binary image (frontend embedded)
#
# Build:  docker build -t papersage -f server/Dockerfile .
# Run:    docker run -p 5478:5478 papersage

# ── Stage 1: frontend deps ──────────────────────────────────
FROM node:22-alpine AS deps
RUN corepack enable && corepack prepare pnpm@10 --activate
WORKDIR /app
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/.npmrc ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

# ── Stage 2: frontend build ─────────────────────────────────
FROM node:22-alpine AS build
RUN corepack enable && corepack prepare pnpm@10 --activate
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY frontend/ .
RUN pnpm build

# ── Stage 3: go build with embedded assets ──────────────────
FROM golang:1.26.9-alpine AS gobuild
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY server/ .
# 前端产物复制进 embed 目录（覆盖占位）
COPY --from=build /app/dist ./web/assets
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/papersage ./cmd/papersage

# ── Stage 4: runtime ────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=gobuild /out/papersage /papersage
ENV PORT=5478
EXPOSE 5478
USER nonroot:nonroot
ENTRYPOINT ["/papersage"]
