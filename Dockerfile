# syntax=docker/dockerfile:1.7
# Multi-stage build producing a minimal distroless image per deployable.
#
# Build one deployable:
#   docker build --build-arg DEPLOYABLE=bff -t bff:dev .
#
# The same Dockerfile is reused for ingestion / bff / notifier / setup by
# varying the DEPLOYABLE build arg.

ARG GO_VERSION=1.26

# --- Build stage -------------------------------------------------------------
FROM golang:${GO_VERSION}-bookworm AS builder

WORKDIR /src

# Cache go modules before copying the whole source tree.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG DEPLOYABLE
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    test -n "${DEPLOYABLE}" && \
    go build -trimpath -ldflags="-s -w" \
        -o /out/app ./cmd/${DEPLOYABLE}

# --- Final stage -------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/app /app

USER nonroot:nonroot
ENTRYPOINT ["/app"]
