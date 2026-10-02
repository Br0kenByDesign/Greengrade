# Build context: repository root (see docker-compose.yml)

# 1) Web app
FROM node:22-alpine AS web
WORKDIR /src/app/web
COPY design /src/design
COPY app/web/package.json app/web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY app/web ./
RUN npm run build

# 2) Go binary with embedded web app
FROM golang:1.26-alpine AS build
WORKDIR /src/app
COPY app/go.mod app/go.sum* ./
COPY app/ ./
COPY --from=web /src/app/webui/dist ./webui/dist
RUN go mod tidy && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/greengrade . \
 && mkdir -p /out/data/photos

# 3) Minimal runtime: no shell, no package manager, non-root
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/greengrade /greengrade
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s CMD ["/greengrade", "healthcheck"]
ENTRYPOINT ["/greengrade"]
