# =============================================================================
# SquareGuardian — Multi-stage Docker Build
# Item detection event processor for Frigate-based camera systems.
#
# Usage:
#   docker build -t squareguardian .
#   docker build --target export -o dist .
#   # → dist/linux-amd64/squareguardian
# =============================================================================

# ---------------------------------------------------------------------------
# Stage 1: deps — Go module cache (rarely changes)
# ---------------------------------------------------------------------------
FROM golang:1.25-bookworm AS deps

WORKDIR /src
COPY go.mod ./
RUN go mod download

# ---------------------------------------------------------------------------
# Stage 2: build-linux — Linux amd64 binary
# ---------------------------------------------------------------------------
FROM deps AS build-linux

COPY . .
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

RUN go build -trimpath -ldflags="-s -w" \
    -o /out/linux-amd64/squareguardian ./cmd/squareguardian

# ---------------------------------------------------------------------------
# Stage 3: runtime — minimal production image
# ---------------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

COPY --from=build-linux /out/linux-amd64/squareguardian /usr/local/bin/squareguardian

EXPOSE 8080

ENTRYPOINT ["squareguardian"]

# ---------------------------------------------------------------------------
# Stage 4: export — extract binary for --output builds
# ---------------------------------------------------------------------------
FROM scratch AS export

COPY --from=build-linux /out/ /
