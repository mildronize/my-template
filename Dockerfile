# syntax=docker/dockerfile:1

# --- web-build stage ------------------------------------------------------
# node:22-alpine — matches docs/GETTING-STARTED.md's Prerequisites section
# (Node, stated there for the same reason Go's own toolchain is). This
# stage builds web/dist *before* the Go build stage even starts, milestone-3
# /task-1's "make build ordering" requirement carried into Docker too:
# cmd/server's //go:embed (web/embed.go) bakes in whatever's on disk under
# web/dist at Go compile time, so a Docker build that skipped this stage
# (or ran it after the Go build) would silently ship the tracked
# dist/.gitkeep placeholder — or a stale layer-cached build — instead of a
# real SPA. Kept as its own stage (not `RUN npm ...` inside the Go build
# stage) so its layer cache only invalidates on web/'s own changes, not on
# every Go source edit.
FROM node:22-alpine AS web-build

WORKDIR /src/web

# package.json + package-lock.json first, same reasoning as the Go build
# stage's own go.mod/go.sum-first layer: `npm ci` (reproducible, fails on
# drift — see Makefile's web-build target) only needs these two files, so
# this layer is cached across rebuilds that don't touch dependencies.
COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# --- build stage --------------------------------------------------------
# golang:1.26-alpine, not the full golang:1.26 image, purely to keep the
# build stage's own layer cache small — none of it ships in the final
# image either way. Pinned to the go.mod `go 1.26.0` directive's minor
# version so the compiler a fork builds with matches the one this
# milestone tested against.
FROM golang:1.26-alpine AS build

WORKDIR /src

# Module download is its own layer so `go mod download` is skipped on
# rebuilds that only change application code, not go.mod/go.sum.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Overwrites the tracked dist/.gitkeep placeholder (copied in by `COPY . .`
# above) with web-build's real Vite output — this has to happen after
# `COPY . .`, not before, or the plain copy would win. This is the one
# line that actually closes the stale-embed gap for `docker compose up`
# (.chief/milestone-3/_goal/GOAL.md Done-when 1 and 11): without it, this
# stage would go build a binary that embeds nothing but the placeholder,
# same as an un-built local `go build ./...` does, and it would do so
# silently — no error, no missing-file warning, just a binary that serves
# an empty SPA.
COPY --from=web-build /src/web/dist ./web/dist

# CGO_ENABLED=0: this template's only DB driver is modernc.org/sqlite, a
# pure-Go/CGO-free SQLite implementation (see internal/platform/db.go) —
# chosen specifically so the binary can be static and the runtime image
# can be distroless (below) with no libc/libsqlite3 dependency to satisfy.
# Both binaries this image ever runs are built here: server is the long-
# running process, issue-key is the one-off CLI docs/DEPLOY-REQUIREMENTS.md
# and docs/GETTING-STARTED.md tell an operator to `docker compose exec`
# for seeding an agent's first API key.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/issue-key ./cmd/issue-key

# An empty /out/data dir, pre-owned by distroless/nonroot's uid:gid
# (65532:65532 — that image has no /etc/passwd entry to name it by, so
# numeric here). Baked into the image and copied into the runtime stage
# below at /app/data *before* docker-compose.yml's named volume ever
# mounts there: Docker initializes a fresh named volume by copying
# whatever already exists at its mount path in the image, ownership
# included, so without this step the volume would be created root-owned
# and the nonroot server process couldn't write app.db into it — the
# runtime stage has no shell/mkdir/chown to fix this after the fact, so
# it has to be prepared here, in the stage that has one.
RUN mkdir -p /out/data && chown -R 65532:65532 /out/data

# --- runtime stage -------------------------------------------------------
# distroless/static, not alpine: the runtime binary is fully static
# (CGO_ENABLED=0, no cgo/libc dependency) and needs nothing else from a
# base OS — no shell, no package manager, no libc — which is the smallest
# attack surface available and is why distroless was chosen over an
# alpine (or scratch) runtime stage. `docker compose exec` runs
# /app/issue-key directly (no shell needed to invoke it, matching a
# distroless image's no-shell constraint). Runs as the image's built-in
# nonroot user (uid 65532), not root.
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

WORKDIR /app
COPY --from=build --chown=nonroot:nonroot /out/server /app/server
COPY --from=build --chown=nonroot:nonroot /out/issue-key /app/issue-key
COPY --from=build --chown=nonroot:nonroot /out/data /app/data

# DATABASE_PATH (internal/platform/config.go) defaults to ./data/app.db,
# resolved against WORKDIR /app — i.e. /app/data/app.db. docker-compose.yml
# mounts its named SQLite volume at exactly /app/data so that default
# already points at persisted storage with no DATABASE_PATH override
# needed; a plain `docker run` (no compose) still writes somewhere sane
# inside the container even without a volume mounted, it just won't
# survive a `docker rm`.
EXPOSE 8080

ENTRYPOINT ["/app/server"]
