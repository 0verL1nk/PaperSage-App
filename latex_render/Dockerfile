# ── Stage: deps ──────────────────────────────────────────────
FROM node:22-bookworm-slim AS deps

WORKDIR /app

COPY package.json package-lock.json ./

# Mount npm cache — survives rebuilds
RUN --mount=type=cache,target=/root/.npm \
    npm ci --omit=dev

# ── Stage: texlive (cached layer) ────────────────────────────
FROM node:22-bookworm-slim AS texlive

ARG CTAN_MIRROR=https://mirrors.tuna.tsinghua.edu.cn/CTAN

# CJK fonts + install-tl dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
        curl \
        perl \
        fonts-noto-cjk \
    && rm -rf /var/lib/apt/lists/* \
    && fc-cache -f

# Install TeX Live via upstream installer (scheme-full)
RUN set -e && \
    TEXLIVE_YEAR=$(date +%Y) && \
    TEXDIR="/usr/local/texlive/${TEXLIVE_YEAR}" && \
    tmpdir=$(mktemp -d) && \
    echo "Downloading install-tl from ${CTAN_MIRROR} ..." && \
    curl -fsSL ${CTAN_MIRROR}/systems/texlive/tlnet/install-tl-unx.tar.gz \
        -o ${tmpdir}/install-tl.tar.gz && \
    tar xzf ${tmpdir}/install-tl.tar.gz -C ${tmpdir} && \
    installer=$(find ${tmpdir} -maxdepth 1 -type d -name "install-tl-*" | head -1) && \
    printf "selected_scheme scheme-full\n\
TEXDIR %s\n\
TEXMFLOCAL /usr/local/texlive/texmf-local\n\
TEXMFSYSCONFIG %s/texmf-config\n\
TEXMFSYSVAR %s/texmf-var\n\
TEXMFHOME ~/texmf\n\
TEXMFCONFIG ~/.texlive/%s/texmf-config\n\
TEXMFVAR ~/.texlive/%s/texmf-var\n\
instopt_adjustpath 0\n\
instopt_adjustrepo 1\n" \
        "${TEXDIR}" "${TEXDIR}" "${TEXDIR}" "${TEXLIVE_YEAR}" "${TEXLIVE_YEAR}" \
        > ${tmpdir}/texlive.profile && \
    echo "Installing TeX Live (scheme-full) ..." && \
    ${installer}/install-tl \
        --profile=${tmpdir}/texlive.profile \
        --repository=${CTAN_MIRROR}/systems/texlive/tlnet && \
    rm -rf ${tmpdir}

# ── Stage: production ───────────────────────────────────────
FROM node:22-bookworm-slim AS production

# Copy TeX Live and fonts from cached stage
COPY --from=texlive /usr/local/texlive /usr/local/texlive
COPY --from=texlive /usr/share/fonts /usr/share/fonts

# Create arch-independent symlink for PATH
RUN TEXBIN=$(find /usr/local/texlive -maxdepth 4 -type d -name '*-linux' | head -1) && \
    ln -sf "${TEXBIN}" /usr/local/texlive/texbin && \
    fc-cache -f

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
