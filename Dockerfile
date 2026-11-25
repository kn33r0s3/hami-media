# Job 17.1 — hami-media Docker image.
#
# Pinned bases (tags; digest pinning is a follow-up — see
# docs/docker-image-17.1.md — because the VM has no Docker daemon and
# the registry digest lookup was not completed in-session).
# Multi-stage: build in Go, run in a slim runtime as a non-root user.
# The binary is static (CGO_ENABLED=0) with -trimpath (job 1.3).
#
# NOTE: the m7h-sfu server takes its HTTP listener from the parent
# process on fd 3 and has no --listen flag, so this image is for proof
# runs (capacity / pair / sfu-single) and for the metrics endpoint
# (-metrics-listen 127.0.0.1:9464). Serving behind a real socket needs
# a --listen flag first; that gap is listed in docs/known-limitations.md.
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/hami-media .

FROM debian:bookworm-slim
RUN groupadd --system --gid 10001 hami \
 && useradd --system --uid 10001 --gid hami --no-create-home --shell /usr/sbin/nologin hami
COPY --from=build /out/hami-media /usr/local/bin/hami-media
USER hami:hami
WORKDIR /work
# Health probe: the binary's own capacity detector exercises the
# binary, the host readers and the heap-budget path. (The m7h-sfu
# server takes its HTTP listener on fd 3, so there is no fixed port
# for a curl-style probe until a --listen flag exists.)
HEALTHCHECK --interval=60s --timeout=10s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/hami-media", "capacity"]
ENTRYPOINT ["/usr/local/bin/hami-media"]
CMD ["capacity"]
