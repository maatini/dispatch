# syntax=docker/dockerfile:1
# Build any dispatch service:
#   docker build --build-arg SERVICE=mail-gateway -t dispatch/mail-gateway .
#   docker build --build-arg SERVICE=mail-worker  -t dispatch/mail-worker  .
#   docker build --build-arg SERVICE=mail-admin   -t dispatch/mail-admin   .
#   docker build --build-arg SERVICE=bouncemanagement -t dispatch/bouncemanagement .

ARG SERVICE=mail-gateway
ARG VERSION=0.5.0

# ── Builder ───────────────────────────────────────────────────────────────────
FROM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS builder
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
