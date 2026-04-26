# ── Stage: deps ──────────────────────────────────────────────
FROM node:22-bookworm-slim AS deps

WORKDIR /app

COPY package.json package-lock.json ./

# Mount npm cache — survives rebuilds
RUN --mount=type=cache,target=/root/.npm \
    npm ci --omit=dev

# ── Stage: texlive (cached layer) ────────────────────────────
FROM node:22-bookworm-slim AS texlive

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        texlive-latex-base \
        texlive-latex-extra \
        texlive-fonts-recommended \
        texlive-lang-chinese \
        texlive-xetex \
        texlive-luatex \
        fonts-noto-cjk \
    && rm -rf /var/lib/apt/lists/* \
    && fc-cache -f

# ── Stage: production ───────────────────────────────────────
FROM node:22-bookworm-slim AS production

# Copy TeX Live from cached stage (changes rarely)
COPY --from=texlive /usr/share/texlive /usr/share/texlive
COPY --from=texlive /usr/share/fonts /usr/share/fonts
COPY --from=texlive /var/lib/texmf /var/lib/texmf
COPY --from=texlive /usr/bin/pdflatex /usr/bin/xelatex /usr/bin/lualatex /usr/bin/
COPY --from=texlive /usr/bin/bibtex /usr/bin/biber /usr/bin/
COPY --from=texlive /usr/lib/x86_64-linux-gnu/libkpathse* /usr/lib/x86_64-linux-gnu/
# Ensure all texlive binaries are available
COPY --from=texlive /usr/bin/ /usr/bin/

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
