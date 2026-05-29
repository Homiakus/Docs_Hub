# ---- build stage ----
FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/docshub ./cmd/docshub

# ---- run stage ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/docshub /app/docshub

ENV ADDR=:8080 \
    DATA_DIR=/data \
    LOG_LEVEL=info \
    RATE_LIMIT_ENABLED=true \
    RATE_LIMIT_RPM=60 \
    RATE_LIMIT_BURST=10

VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/docshub"] || exit 1

ENTRYPOINT ["/app/docshub"]
