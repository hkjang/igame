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

FROM golang:1.24-alpine AS go-build
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
    CGO_ENABLED=0 GOOS=linux go build \
      -buildvcs=false \
      -trimpath \
      -ldflags="-s -w \
        -X github.com/hkjang/igame/internal/version.Version=${VERSION} \
        -X github.com/hkjang/igame/internal/version.Commit=${COMMIT} \
        -X github.com/hkjang/igame/internal/version.BuildDate=${BUILD_DATE}" \
      -o /out/igame ./cmd/igame

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 igame \
    && adduser -S -D -H -u 10001 -G igame igame \
    && mkdir -p /app/data /licenses \
    && chown -R igame:igame /app/data

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="igame" \
      org.opencontainers.image.description="Offline-ready internal game platform" \
      org.opencontainers.image.source="https://github.com/hkjang/igame" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

COPY --from=go-build --chown=igame:igame /out/igame /app/igame
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
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/igame"]
