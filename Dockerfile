# syntax=docker/dockerfile:1
# Build any dispatch service:
#   docker build --build-arg SERVICE=mail-gateway -t dispatch/mail-gateway .
#   docker build --build-arg SERVICE=mail-worker  -t dispatch/mail-worker  .
#   docker build --build-arg SERVICE=mail-admin   -t dispatch/mail-admin   .
#   docker build --build-arg SERVICE=bouncemanagement -t dispatch/bouncemanagement .

ARG SERVICE=mail-gateway
ARG VERSION=0.7.0

# ── Builder ───────────────────────────────────────────────────────────────────
FROM golang:1.26-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83 AS builder
ARG SERVICE
ARG VERSION

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w -X dispatch/internal/version.Version=${VERSION}" \
    -o /bin/service ./cmd/${SERVICE}

# ── Runtime ───────────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot@sha256:aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b

COPY --from=builder /bin/service /service

USER nonroot:nonroot
ENTRYPOINT ["/service"]
