# Multi-stage: the toolchain never reaches the running image.
FROM golang:1.26-alpine AS build

WORKDIR /src

# Dependencies first, as their own layer: they change far less often than the
# code, so an ordinary edit does not re-download the module cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO off gives a static binary that runs on a distroless base.
# -trimpath keeps build paths out of the binary; -s -w drop the symbol table
# and DWARF, which are of no use in production.
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# Distroless: no shell, no package manager, nothing to exploit that we did not
# put there. static-debian12 is the right variant for a CGO-free binary.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/api /api

# Unprivileged by default — the nonroot tag ships uid 65532.
USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/api"]
