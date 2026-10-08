# ── Stage: deps ──────────────────────────────────────────────
FROM node:22-bookworm-slim AS deps

# pprof has no prebuilt binary for every Node 22 ABI; allow node-gyp to compile it.
RUN apt-get update && apt-get install -y --no-install-recommends \
        python3 make g++ \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY package.json package-lock.json ./
# workspaces 本地包必须随行：npm ci 要把它们链进 node_modules
# （deps 漏 COPY libraries 时 @overleaf/* 装不出来，容器启动即
#  ERR_MODULE_NOT_FOUND——该镜像此前只 build 从未运行，问题未暴露）
COPY libraries/ libraries/
# express 等运行时依赖声明在 services/clsi 这个 workspace 里：npm ci
# 需要各 workspace 的 package.json 才会装其依赖
COPY services/clsi/package.json services/clsi/package.json

# lockfile 由 npm 11 生成：npm 10 的 ci 读不懂 workspaces 条目
RUN npm install -g npm@11 --silent

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

# 变量名与 settings.defaults.cjs 对齐（旧名从未被读取）
ENV LISTEN_ADDRESS=0.0.0.0 \
    PORT=3013 \
    SANDBOXED_COMPILES=false \
    CLSI_COMPILES_PATH=/tmp/clsi/compiles \
    CLSI_OUTPUT_PATH=/tmp/clsi/output \
    CLSI_CACHE_PATH=/tmp/clsi/cache \
    CLSI_UPLOAD_PATH=/tmp/clsi/uploads \
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
