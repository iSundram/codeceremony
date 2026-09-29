# CodeCeremony — a single static binary, no runtime dependencies.
#
# The build needs the network once, to fetch the toolchain, the one Go module
# (golang.org/x/crypto, for password hashing) and the frontend's npm packages.
# The resulting image has nothing in it but the binary: no shell, no package
# manager, no libc, and no Node runtime. That is what makes the offline
# requirement hold at runtime rather than only in principle.

# ---- frontend --------------------------------------------------------------
# The frontend is built in its own stage and only its output is carried forward.
# Node never reaches the runtime image, so the portal still runs from one binary
# and `docker compose up` still needs no cloud account, hosted database or
# external service. This is the reason for Vite over a server-rendered frontend
# with a Node server, and the reason for Vite over a framework that would need a
# Node process at runtime.
FROM node:22-alpine AS frontend

WORKDIR /web

# Dependencies first, so a source-only change reuses the cached module layer and
# does not re-resolve the tree.
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund

COPY tsconfig.json tsconfig.app.json vite.config.ts index.html ./
COPY public ./public
COPY src ./src

# Typecheck before building. A bundle that compiles but does not typecheck is a
# bundle that will fail later in a harder-to-diagnose place.
RUN npx tsc -b

RUN npx vite build

# ---- build -----------------------------------------------------------------
FROM golang:1.25-alpine AS build

# Honour the build host's architecture so the same Dockerfile works on an
# Apple Silicon laptop and an x86 server without a --platform flag.
ARG TARGETOS=linux
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /src

# Dependencies first, so a source-only change reuses the cached module layer.
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

# The built frontend, staged into the package the Go embed directive points at.
# It has to land before `go build`, because the embed happens at compile time.
COPY --from=frontend /web/dist/ ./internal/httpapi/webassets/spa/

# CGO off produces a genuinely static binary, which is what lets the runtime
# stage be distroless/static rather than alpine.
# -trimpath removes local filesystem paths from the binary.
# -s -w drop the symbol table and DWARF data.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    GOARM="" \
    go build -trimpath -ldflags="-s -w" -o /out/codeceremony ./cmd/codeceremony

# Prove the binary actually runs before it is copied into the image. A build
# that produces a binary which cannot execute is worse than a build failure,
# because it fails on someone else's machine.
RUN /out/codeceremony -version

# An empty data directory, created here rather than in the runtime stage
# because distroless has no shell to run mkdir or chown with.
RUN mkdir -p /data

# ---- runtime ---------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/codeceremony /codeceremony
# The shared fixture file travels with the image so a cold clone needs no
# download step to seed.
COPY --from=build /src/fixtures.json /app/fixtures.json

# The data directory is owned by 65532, the distroless nonroot user. Docker
# initialises a fresh named volume from the image's directory at the mount
# point, including its ownership, so `docker compose up` can write to the
# volume while still running unprivileged. Without this the volume would be
# created root-owned and the portal would fail to start on a first boot.
COPY --from=build --chown=65532:65532 /data /var/lib/codeceremony

USER nonroot:nonroot

ENV DATA_DIR=/var/lib/codeceremony \
    HTTP_ADDR=:8080

EXPOSE 8080

# The healthcheck in docker-compose.yml invokes this flag directly, because the
# image has no shell to run a command in.
ENTRYPOINT ["/codeceremony"]
