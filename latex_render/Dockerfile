# ── Stage: deps ──────────────────────────────────────────────
FROM node:22-bookworm-slim AS deps

WORKDIR /app

COPY package.json package-lock.json ./

# Mount npm cache — survives rebuilds
RUN --mount=type=cache,target=/root/.npm \
    npm ci --omit=dev

# ── Stage: production ───────────────────────────────────────
FROM node:22-bookworm-slim AS production

# Copy TeX Live from pre-built image (danteev/texlive = full scheme)
COPY --from=danteev/texlive:latest /usr/local/texlive /usr/local/texlive

# CJK fonts + fontconfig
RUN apt-get update && apt-get install -y --no-install-recommends \
        fontconfig \
        fonts-noto-cjk \
    && rm -rf /var/lib/apt/lists/* \
    && fc-cache -f

# Create arch-independent symlink for PATH
RUN TEXBIN=$(find /usr/local/texlive -maxdepth 4 -type d -name '*-linux' | head -1) && \
    ln -sf "${TEXBIN}" /usr/local/texlive/texbin

ENV PATH="/usr/local/texlive/texbin:${PATH}"

WORKDIR /app

RUN addgroup --system appgroup && adduser --system --ingroup appgroup appuser

COPY --from=deps --chown=appuser:appgroup /app/node_modules ./node_modules
COPY --chown=appuser:appgroup libraries/ libraries/
COPY --chown=appuser:appgroup services/ services/
COPY --chown=appuser:appgroup patches/ patches/
COPY --chown=appuser:appgroup package.json ./

ENV HOST=0.0.0.0 \
    PORT=3013 \
    SANDBOXED_COMPILES=false \
    COMPILES_DIR=/tmp/clsi/compiles \
    OUTPUT_DIR=/tmp/clsi/output \
    CACHE_DIR=/tmp/clsi/cache \
    UPLOAD_DIR=/tmp/clsi/uploads \
    TEXMFVAR=/tmp/clsi/texmf-var \
    NODE_ENV=production

RUN mkdir -p /tmp/clsi/compiles /tmp/clsi/output /tmp/clsi/cache \
    /tmp/clsi/uploads /tmp/clsi/texmf-var \
    && chown -R appuser:appgroup /tmp/clsi

USER appuser

EXPOSE 3013

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD node -e "fetch('http://localhost:3013/health_check').catch(() => process.exit(1))" || exit 1

CMD ["node", "--max-semi-space-size=64", "services/clsi/app.js"]
