# Docksmith

Docksmith is a simplified Docker-like local container build and runtime system.

## What this implementation includes

- Single CLI binary, no daemon.
- Persistent local store at `~/.docksmith/` with:
  - `images/` one JSON manifest per image.
  - `layers/` content-addressed tar layers by SHA-256 digest.
  - `cache/` cache index mapping cache keys to layer digests.
- Build file parser for `Docksmithfile` with strict support for:
  - `FROM`, `COPY`, `RUN`, `WORKDIR`, `ENV`, `CMD`.
- Unknown instruction fails with line-numbered error.
- `COPY` and `RUN` generate immutable delta layers.
- Deterministic tar output (sorted paths, zero timestamps).
- `COPY` sources are strictly contained to the build context root; escaping paths are rejected.
- Symlink sources in `COPY` are rejected to prevent context escapes.
- Cache key includes:
  - previous producing digest (or base manifest digest),
  - raw instruction text,
  - current `WORKDIR`,
  - sorted serialized `ENV`,
  - `COPY` source file hashes in sorted order.
- Cache cascade rule: once a miss happens, all later producing steps miss.
- `--no-cache` disables cache read/write.
- Linux runtime isolation uses `chroot` with mount/UTS/IPC/PID/network namespaces.
- Build `RUN` and `docksmith run` execute with network namespace isolation (no host network access by default).
- Same isolation mechanism used for `RUN` during build and `docksmith run`.

## Important platform note

Process isolation requires Linux. On Windows/macOS, use WSL2 or a Linux VM.

## CLI

```bash
docksmith build -t <name:tag> [--no-cache] <context>
docksmith images
docksmith rmi <name:tag>
docksmith run [-e KEY=VALUE ...] <name:tag> [cmd arg ...]
```

Additional helper command for local offline base setup:

```bash
docksmith import-rootfs -t <name:tag> <rootfsDir>
```

## Build and sample usage

1. Build binary:

```bash
go build -o docksmith .
```

2. One-time base image import from local rootfs (Linux/WSL2):

```bash
./scripts/bootstrap_base.sh
```

3. Cold build sample app:

```bash
sudo ./docksmith build -t myapp:latest ./examples/sample-app
```

4. Warm build:

```bash
sudo ./docksmith build -t myapp:latest ./examples/sample-app
```

5. Run with default `CMD`:

```bash
sudo ./docksmith run myapp:latest
```

6. Run with environment override:

```bash
sudo ./docksmith run -e MESSAGE=Overridden myapp:latest
```

7. List images:

```bash
./docksmith images
```

8. Remove image and layers:

```bash
./docksmith rmi myapp:latest
```

## Demo script

```bash
./scripts/demo.sh
```

## Notes on constraints

- No network access is required for build/run after base import.
- Layers are immutable and content-addressed.
- No detached mode, no daemon, no bind mounts, no registry support.
- No layer reference counting in `rmi`; shared layers may be deleted.
