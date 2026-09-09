# Buddy AI backend (Go) — built for Cloud Run.
#
# Production auth: leave FIREBASE_CREDENTIALS_FILE unset when running this image on
# Cloud Run. The server then uses Application Default Credentials from the Cloud Run
# service's attached service account instead of a key file — no credentials file is
# ever baked into this image.

FROM golang:1.27-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# Minimal, shell-less runtime image.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /out/api /api

# Cloud Run sets PORT and expects the container to listen on it; config.go already
# reads PORT from the environment (defaulting to 8080 if unset).
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
