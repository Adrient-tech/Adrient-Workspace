# syntax=docker/dockerfile:1
# ---------------------------------------------------------------------------
# adrient-workspace — single image: Go backend that also serves the built React
# SPA (--static-dir) on :8080 (HTTP/REST) and :9000 (gRPC). pandoc is included
# for the Docs export endpoint. Migrations are embedded in the binary.
# ---------------------------------------------------------------------------

# ---- 1. Build the React SPA ------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /web
# Build-time public URLs baked into the SPA. Override per-environment with
# --build-arg. Instance-specific app hosts (CRM/Assemble) are left empty here and
# injected per-deployment; same-origin apps (PDF/Learn) keep their defaults.
# PDF is served IN-PROCESS at /pdf on the same origin (ADRIENT_PDF_BUILTIN +
# ADRIENT_PDF_STATIC_DIR); the standalone external PDF host is retired, so the
# tiles point at the same-domain /pdf rather than a dead external host.
ARG VITE_PDF_URL=/pdf/
# CRM host is deployment-specific — inject VITE_CRM_URL at build time rather than
# baking an instance hostname into the public image. Empty => the app's built-in
# localtest fallback (see web/app/src/catalog/apps.ts).
ARG VITE_CRM_URL=
ARG VITE_GIT_URL=https://github.com/Adrient-tech
# Assemble host is deployment-specific too — inject VITE_ASSEMBLE_URL per-deploy.
ARG VITE_ASSEMBLE_URL=
ENV VITE_PDF_URL=$VITE_PDF_URL \
    VITE_CRM_URL=$VITE_CRM_URL \
    VITE_GIT_URL=$VITE_GIT_URL \
    VITE_ASSEMBLE_URL=$VITE_ASSEMBLE_URL
COPY VERSION /VERSION
COPY web/app/package.json web/app/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY web/app/ ./
RUN npm run build   # -> /web/dist

# ---- 1b. Build the PDF signing SPA (served in-process under /pdf) ----------
# The PDF frontend (React 19 + Vite) is bundled and served by adrient itself when
# ADRIENT_PDF_BUILTIN=true and ADRIENT_PDF_STATIC_DIR points at its dist (see runtime
# stage), so no separate pdf-frontend container is needed. Build env wires it to
# adrient's session: it talks to the in-process backend at /pdf-api/api with cookie
# auth and redirects 401s to adrient's login. Vite base "/pdf/" makes asset URLs
# resolve under adrient's /pdf mount (vite.config.ts reads ADRIENT_PDF_BASE).
FROM node:22-alpine AS pdfweb
WORKDIR /pdf
ENV ADRIENT_PDF_BASE=/pdf/ \
    VITE_ADRIENT_INTEGRATED=true \
    VITE_API_BASE=/pdf-api/api \
    VITE_ADRIENT_LOGIN_URL=/api/v1/auth/login
# tibui is fetched per the lockfile.
COPY pdf/frontend/package.json pdf/frontend/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY pdf/frontend/ ./
RUN npm run build   # -> /pdf/dist

# ---- 2. Generate protos + build the Go binary ------------------------------
FROM golang:1.25-alpine AS build
RUN apk add --no-cache git
# buf generates the gRPC + grpc-gateway code (gen/ is gitignored) using public
# BSR remote plugins, so the build needs network but no extra local toolchain.
RUN go install github.com/bufbuild/buf/cmd/buf@v1.64.0
WORKDIR /src
# Prime the module cache.
COPY go.mod go.sum ./
RUN go mod download
# Source needed for generation + build. buf.lock pins the googleapis BSR dep
# (google/api/annotations.proto); buf generate fetches it from the lock.
COPY proto/ ./proto/
COPY buf.yaml buf.gen.yaml buf.lock ./
RUN buf dep update && buf generate
# Everything else, including integrations/homeassistant (the Home Assistant
# custom integration is go:embed-ded and served at
# /integrations/homeassistant/adrient.zip; see internal/hacomponent).
COPY . .
# Stamp the build so GET /healthz reports the real version/commit instead of the
# "0.0.0-dev"/"unknown" defaults. CI passes these via --build-arg; harmless if unset.
ARG VERSION=0.0.0-dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/server ./cmd/server \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/adrient-admin ./cmd/adrient-admin

# ---- 3. Runtime ------------------------------------------------------------
FROM alpine:3.20
# pandoc comes from the upstream static release, not apk: Alpine's pandoc
# (3.1.13 in 3.20/3.21, 3.6.4 in 3.22) drops tables and code blocks when
# importing ODT, and pre-3.2 builds ignore --sandbox for embedded resources.
# internal/docs tests pin the behaviour; CI runs them against this version.
ARG PANDOC_VERSION=3.10
ARG TARGETARCH=amd64
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 adrient \
 && case "$TARGETARCH" in \
      amd64) sum=e0f8af62d0f267d22baa5bcefe6d5dda3a097ccc60de794b759fe03159923244 ;; \
      arm64) sum=55413dfb0c1aec861641fe858f1f73e84848f3db497b1c0c02e62887ea76f4a4 ;; \
      *) echo "no pandoc checksum for $TARGETARCH" >&2; exit 1 ;; \
    esac \
 && wget -qO /tmp/pandoc.tgz "https://github.com/jgm/pandoc/releases/download/${PANDOC_VERSION}/pandoc-${PANDOC_VERSION}-linux-${TARGETARCH}.tar.gz" \
 && echo "$sum  /tmp/pandoc.tgz" | sha256sum -c - \
 && tar -xzf /tmp/pandoc.tgz -C /usr/local/bin --strip-components 2 "pandoc-${PANDOC_VERSION}/bin/pandoc" \
 && rm /tmp/pandoc.tgz && pandoc --version | head -1
WORKDIR /app
COPY --from=build  /out/server /app/server
# Operator CLI: docker compose exec adrient /app/adrient-admin doctor
COPY --from=build  /out/adrient-admin /app/adrient-admin
COPY --from=web    /web/dist   /app/web/dist
COPY --from=pdfweb /pdf/dist   /app/pdf-web
ENV ADRIENT_STATIC_DIR=/app/web/dist \
    ADRIENT_PDF_STATIC_DIR=/app/pdf-web
USER adrient
EXPOSE 8080 9000
ENTRYPOINT ["/app/server"]

# ---------------------------------------------------------------------------
# Optional: LibreOffice import (CC8). Not in the default image: LibreOffice
# adds ~500 MB. To accept legacy .doc/.xls/.ppt, build a derived image and
# run it with ADRIENT_LIBREOFFICE=1 (Helm: adrient.libreoffice.enabled=true):
#
#   FROM ghcr.io/adrient-tech/adrient-workspace:<tag>
#   USER root
#   RUN apk add --no-cache libreoffice-writer libreoffice-calc \
#         libreoffice-impress font-liberation font-carlito
#   USER adrient
#   # soffice is auto-detected (PATH, /usr/lib/libreoffice/program)
#   ENV ADRIENT_LIBREOFFICE=1
#
# LibreOffice (MPL-2.0) is exec'd as a separate program over files, never
# linked. Each conversion uses its own throwaway profile with macros
# disabled (internal/convert). For extra isolation run adrient with no egress
# network policy; soffice never needs the network.
# ---------------------------------------------------------------------------
