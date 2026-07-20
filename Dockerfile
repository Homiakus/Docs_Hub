# syntax=docker/dockerfile:1

# ---- build stage ----
# По умолчанию используется официальный компактный Alpine-образ.
# При необходимости зеркало можно передать так:
#   --build-arg GO_IMAGE=docker.m.daocloud.io/library/golang:1.25-alpine
ARG GO_IMAGE=golang:1.25-alpine
FROM ${GO_IMAGE} AS build

WORKDIR /src

# git нужен для зависимостей, которые загружаются напрямую из VCS.
RUN apk add --no-cache ca-certificates git

ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=local

# GOPROXY можно переопределить при сборке без изменения Dockerfile.
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

# Сначала копируются только файлы модулей, чтобы кэш зависимостей
# не сбрасывался при каждом изменении исходного кода.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download && go mod verify

COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-amd64}" \
    go build \
      -mod=readonly \
      -trimpath \
      -buildvcs=false \
      -ldflags="-s -w" \
      -o /out/docshub \
      ./cmd/docshub && \
    mkdir -p /out/data

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=build --chown=nonroot:nonroot /out/docshub /app/docshub
COPY --from=build --chown=nonroot:nonroot /out/data /data

ENV ADDR=:8080 \
    DATA_DIR=/data \
    LOG_LEVEL=info \
    RATE_LIMIT_ENABLED=true \
    RATE_LIMIT_RPM=60 \
    RATE_LIMIT_BURST=10

VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD ["/app/docshub", "healthcheck", "--url=http://127.0.0.1:8080/healthz"]

ENTRYPOINT ["/app/docshub"]
