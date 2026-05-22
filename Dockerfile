FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY deploytui ./deploytui
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /docs-hub .

FROM alpine:3.20
RUN addgroup -S docshub && adduser -S docshub -G docshub
WORKDIR /app
COPY --from=build /docs-hub /app/docs-hub
RUN mkdir -p /data && chown -R docshub:docshub /data /app
USER docshub
ENV ADDR=:8080
ENV DATA_FILE=/data/storage.json
EXPOSE 8080
VOLUME ["/data"]
CMD ["/app/docs-hub", "serve"]
