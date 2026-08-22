# syntax=docker/dockerfile:1.7

FROM node:22-alpine AS web-build
WORKDIR /src

COPY sdk/gamehub-js/package.json sdk/gamehub-js/package-lock.json ./sdk/gamehub-js/
RUN --mount=type=cache,target=/root/.npm \
    npm --prefix sdk/gamehub-js ci
COPY sdk/gamehub-js ./sdk/gamehub-js
RUN npm --prefix sdk/gamehub-js run build

COPY web/package.json web/package-lock.json ./web/
RUN --mount=type=cache,target=/root/.npm \
    npm --prefix web ci
COPY web ./web
COPY VERSION ./VERSION
COPY scripts/check-offline-bundle.sh ./scripts/check-offline-bundle.sh
RUN npm --prefix web run build \
    && sh ./scripts/check-offline-bundle.sh /src

FROM golang:1.26.6-alpine3.23 AS go-build
WORKDIR /src
RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
COPY --from=web-build /src/web/dist ./internal/web/dist

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    mkdir -p /out/app/data \
    && \
    CGO_ENABLED=0 GOOS=linux go build \
      -buildvcs=false \
      -trimpath \
      -ldflags="-s -w \
        -X github.com/hkjang/igame/internal/version.Version=${VERSION} \
        -X github.com/hkjang/igame/internal/version.Commit=${COMMIT} \
        -X github.com/hkjang/igame/internal/version.BuildDate=${BUILD_DATE}" \
      -o /out/app/igame ./cmd/igame

FROM scratch AS runtime

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="igame" \
      org.opencontainers.image.description="Offline-ready internal game platform" \
      org.opencontainers.image.source="https://github.com/hkjang/igame" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      io.igame.build.go-version="1.26.6"

COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=go-build --chown=10001:10001 /out/app /app
COPY LICENSE /licenses/LICENSE
COPY --from=web-build /src/web/package.json /licenses/web/package.json
COPY --from=web-build /src/web/package-lock.json /licenses/web/package-lock.json
COPY --from=web-build /src/sdk/gamehub-js/package.json /licenses/sdk/gamehub-js/package.json
COPY --from=web-build /src/sdk/gamehub-js/package-lock.json /licenses/sdk/gamehub-js/package-lock.json
COPY --from=web-build /src/web/node_modules/phaser/package.json /licenses/phaser/package.json
COPY --from=web-build /src/web/node_modules/phaser/LICENSE.md /licenses/phaser/LICENSE.md

USER 10001:10001
WORKDIR /app
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=5 \
  CMD ["/app/igame", "healthcheck"]

ENTRYPOINT ["/app/igame"]
