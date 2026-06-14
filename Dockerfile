ARG NODE_VERSION=22-bookworm-slim
ARG GO_VERSION=1.24-bookworm

FROM --platform=$BUILDPLATFORM node:${NODE_VERSION} AS web-builder
WORKDIR /src/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS go-builder
WORKDIR /src

ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY go.mod go.sum ./
COPY pkg/runner/go.mod pkg/runner/go.sum ./pkg/runner/
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
COPY data ./data
COPY runbooks ./runbooks
COPY proto ./proto

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/ai-server ./cmd/ai-server

RUN mkdir -p /out/artifacts/host-agent/v0.1.0/linux-amd64 \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/artifacts/host-agent/v0.1.0/linux-amd64/host-agent ./cmd/host-agent

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tini \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 aiops \
    && useradd --system --uid 10001 --gid aiops --home-dir /var/lib/aiops --shell /usr/sbin/nologin aiops \
    && mkdir -p /app/web/dist /var/lib/aiops \
    && chown -R aiops:aiops /app /var/lib/aiops

WORKDIR /app

COPY --from=go-builder /out/ai-server /usr/local/bin/ai-server
COPY --from=go-builder /out/artifacts /app/artifacts
COPY --from=web-builder /src/web/dist /app/web/dist
COPY data ./data
COPY runbooks ./runbooks
COPY docker/entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENV AIOPS_HTTP_ADDR=:8080 \
    AIOPS_GRPC_ADDR=:18090 \
    AIOPS_DATA_DIR=/var/lib/aiops \
    AIOPS_WEB_DIST_DIR=/app/web/dist \
    AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR=/var/lib/aiops/model-input-traces

EXPOSE 8080 18090

HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
  CMD curl -fsS "http://127.0.0.1:${AIOPS_HEALTH_PORT:-8080}/" >/dev/null || exit 1

USER 10001:10001

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["/usr/local/bin/ai-server"]
