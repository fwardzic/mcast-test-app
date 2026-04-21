# =============================================================================
# Stage 1: Builder
#
# We use golang:1.24-alpine as the build environment because:
#   - It has the Go toolchain we need to compile our binaries
#   - Alpine is small, keeping this stage lightweight
#   - We compile here and discard this layer in the final image
# =============================================================================
FROM golang:1.24-alpine AS builder

# Install 'file' utility so we can verify binaries are truly statically linked
# during debugging — not strictly needed at runtime but useful for CI checks
RUN apk add --no-cache file

WORKDIR /src

# Copy dependency manifests first so Docker can cache the module download layer.
# If go.mod/go.sum don't change, 'go mod download' won't re-run on rebuilds.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# TARGETARCH is set automatically by 'docker buildx build --platform'.
# Defaults to amd64 for plain 'docker build' invocations.
ARG TARGETARCH=amd64

# Build both binaries as fully static ELFs:
#   CGO_ENABLED=0   — disable C-Go so no libc dependency is introduced
#   GOOS=linux      — target Linux (the container OS)
#   GOARCH           — from build arg, enables cross-compilation
#   -tags netgo     — use pure-Go net package (avoids glibc resolver)
#   -extldflags '-static' — tell the linker to produce a static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -tags netgo -ldflags="-extldflags '-static'" \
    -o /sender ./cmd/sender \
    && \
    CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -tags netgo -ldflags="-extldflags '-static'" \
    -o /receiver ./cmd/receiver

# =============================================================================
# Stage 2: Runtime
#
# nicolaka/netshoot is the target container image. It ships with networking
# debug tools like tcpdump, ip, ping, etc., which are invaluable for
# troubleshooting multicast behaviour in Kubernetes.
#
# Because our binaries are fully static, they carry zero runtime dependencies
# and drop cleanly into any base image.
# =============================================================================
FROM nicolaka/netshoot:v0.15

# Copy compiled binaries from the builder stage into a standard PATH location
COPY --from=builder /sender   /usr/local/bin/sender
COPY --from=builder /receiver /usr/local/bin/receiver

# Default entrypoint is the sender binary.
# Override with 'command: [receiver]' in your Kubernetes pod spec to run
# the receiver instead.
ENTRYPOINT ["sender"]
