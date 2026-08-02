# Builds the Mini App and bakes it into a Caddy image, so the VPS needs no Node
# toolchain and a deploy is `docker compose up -d --build`.
#
# Build context is the repository root — the Caddyfile lives there.
FROM node:24-alpine AS build

WORKDIR /app

# Lockfile first: dependencies change far less often than source.
COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM caddy:2-alpine

COPY Caddyfile /etc/caddy/Caddyfile
COPY --from=build /app/dist /srv
