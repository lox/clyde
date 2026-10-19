# Build from the repository root, after deploy/export-admissions.
FROM golang:1.27.2-bookworm AS harness
WORKDIR /src/harness
COPY harness/go.mod harness/go.sum ./
RUN go mod download
COPY harness/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/clyde ./cmd/clyde

FROM node:24.15.0-bookworm-slim AS body
WORKDIR /src/body
COPY body/package.json body/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY body/tsconfig.json ./
COPY body/src/ ./src/
RUN npm run build && npm prune --omit=dev --no-audit --no-fund

FROM node:24.15.0-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
ARG CLYDE_REVISION
LABEL org.opencontainers.image.source="https://github.com/lox/clyde" \
      org.opencontainers.image.revision=$CLYDE_REVISION
WORKDIR /opt/clyde
COPY --from=harness /out/clyde /usr/local/bin/clyde
COPY --from=body /src/body/dist/ body/dist/
COPY --from=body /src/body/node_modules/ body/node_modules/
COPY body/package.json body/package.json
COPY api/*.schema.json api/
COPY prompts/ prompts/
COPY skills/ skills/
# Explicit, gitignored, model-free gym output; never copy a whole runtime tree.
COPY deploy/admissions/ skills/
RUN test -n "$CLYDE_REVISION" && test "$(cat skills/revision.txt)" = "$CLYDE_REVISION" \
    && clyde skill verify collect-logs craft-item craft-place-table deposit-items eat-food \
    && mkdir memory && chown node:node memory
COPY deploy/entrypoint /usr/local/bin/clyde-start
USER node
ENV BODY_LISTEN_HOST=127.0.0.1 BODY_LISTEN_PORT=7777 CLYDE_BODY_URL=http://127.0.0.1:7777
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/clyde-start"]
# CLI budgets remain the existing defaults; replace command to tune them.
CMD []
