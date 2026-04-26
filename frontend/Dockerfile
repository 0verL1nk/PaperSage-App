# ── Stage: deps ──────────────────────────────────────────────
FROM node:22-alpine AS deps

RUN corepack enable && corepack prepare pnpm@latest --activate

WORKDIR /app

COPY package.json pnpm-lock.yaml .npmrc ./

# Mount pnpm store cache — survives rebuilds
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

# ── Stage: build ─────────────────────────────────────────────
FROM node:22-alpine AS build

RUN corepack enable && corepack prepare pnpm@latest --activate

WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN pnpm build

# ── Stage: production ───────────────────────────────────────
FROM nginx:1.27-alpine AS production

# Optimized nginx config: gzip, static caching, security headers
COPY <<'NGINX' /etc/nginx/conf.d/default.conf
server {
    listen 5173;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    # ── Gzip compression ────────────────────────────────────
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_min_length 256;
    gzip_types
        text/plain
        text/css
        text/javascript
        application/javascript
        application/json
        application/xml
        image/svg+xml;

    # ── API reverse proxy ───────────────────────────────────
    location /api/ {
        proxy_pass http://backend:5478;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
    }

    # ── Hashed static assets: aggressive cache ──────────────
    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
        access_log off;
    }

    # ── SPA fallback ────────────────────────────────────────
    location / {
        try_files $uri $uri/ /index.html;
    }
}
NGINX

COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 5173

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD wget -qO- http://localhost:5173/ || exit 1

CMD ["nginx", "-g", "daemon off;"]
