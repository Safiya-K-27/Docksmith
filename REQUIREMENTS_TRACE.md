# Requirements Trace

This file maps the requested Docksmith requirements to implementation points.

## Single binary, no daemon
- Implemented in `main.go` as a single CLI executable.

## Local state under ~/.docksmith
- Created and enforced by `mustStoreRoot` in `util.go`.
- Uses `images/`, `layers/`, `cache/`.

## Build language support
- `parseDocksmithfile` in `parser.go` supports only:
  - `FROM`, `COPY`, `RUN`, `WORKDIR`, `ENV`, `CMD`.
- Unknown instruction errors include line number.

## Image format and digest rule
- Manifest structure in `types.go`.
- Canonical digest computed in `manifestDigest` in `store.go` with digest field emptied before hashing.

## Delta layers, immutable, content-addressed
- Snapshot/diff + tar generation in `layer.go`.
- Layer digest from tar raw bytes, file stored by digest.
- No layer mutation after write.

## Deterministic archives
- Sorted entry paths and zeroed tar timestamps in `buildDeltaTar`.

## Build cache and cascade
- Cache index in `cache/index.json`.
- Key computation in `computeCacheKey` in `build.go`.
- Hit requires index match and layer file existence.
- Cascade miss logic in `buildImage`.
- `--no-cache` disables cache lookup and write.

## Instruction semantics
- `FROM`: local image required, no new layer.
- `COPY`: context copy with `*` and `**` via `glob.go`; creates layer.
- `RUN`: isolated command in assembled root; creates layer.
- `WORKDIR`: config-only; deferred directory creation before next producing step.
- `ENV`: config-only, injected for RUN and runtime.
- `CMD`: config-only, JSON array required.

## Runtime isolation
- Linux-only implementation in `runtime_linux.go` using chroot-based process execution.
- Same `runIsolated` used by build RUN and runtime run.
- Temporary rootfs extracted per run and cleaned up.

## CLI commands
- `build`, `images`, `rmi`, `run` in `main.go` + `commands.go`.
- Additional helper: `import-rootfs` for base image bootstrap.

## Sample app and demo
- Sample app in `examples/sample-app/` with all six instructions.
- Demo workflow script in `scripts/demo.sh`.
- Base bootstrap script in `scripts/bootstrap_base.sh`.
