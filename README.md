# sway

A container runtime that doesn't ship the filesystem. Instead, it mounts a FUSE filesystem that lazily fetches files from a remote fileserver as the container process actually touches them. The container runs via `runc` on a remote worker, and the rootfs is assembled on-demand -- no image pull, no layer extraction at runtime, no multi-gigabyte transfers for a scipy import. An attempt at recreating Modal based on their tech talks.

## Quick start

### 1. Write your app

Create a directory with a Dockerfile and a Python script. The Dockerfile just needs to install your dependencies:

```dockerfile
FROM python:3.10
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py .
CMD ["python", "app.py"]
```

```
# requirements.txt
scipy
```

```python
# app.py
import numpy as np
from scipy import linalg

a = np.random.randn(100, 100)
u, s, vh = linalg.svd(a)
print(f"svd: u={u.shape}, s={s.shape}")
```

### 2. Export the image

```bash
cd your-app/
sway export
```

This builds the Docker image locally, saves it as a tarball, walks every layer, resolves symlink chains, deduplicates via SHA1, and batch-uploads everything to the fileserver. Only files that have changed since last export get uploaded (sync protocol).

### 3. Run it

```bash
export SWAY_USERNAME=yourname
sway run app.py
```

Your script gets uploaded to the fileserver, then a request hits the worker machine. The worker writes an OCI config, calls `runc run`, and the container starts against the FUSE mount. As Python imports scipy, the FUSE filesystem fetches the `.so` files, the bytecode, whatever -- on demand. stdout comes back to your terminal.

That's it. You write a Dockerfile, export once, then iterate on your script with `sway run`.

## Architecture

```
 your machine                  fileserver (8443/TLS)              worker (8444/HTTP)
 ┌──────────┐                  ┌─────────────────┐               ┌──────────────────┐
 │ sway CLI │──export─────────>│ content-addressed│               │ FUSE filesystem  │
 │          │                  │ file store       │<──on-demand───│   (go-fuse)      │
 │          │──run────────────>│                  │   fetches     │       │          │
 └──────────┘                  └─────────────────┘               │       ▼          │
                                                                  │  runc container  │
                                                                  │  (OCI runtime)   │
                                                                  └──────────────────┘
```

Three components, three machines. The interesting part is what *doesn't* happen: no image pull on the worker. The worker never has the full filesystem. It has a FUSE mount and a cache directory.

### Fileserver

Content-addressed blob store. Files are keyed by SHA1 of their content. Directory structure is tracked as a separate index mapping paths to hashes and parent-child relationships.

Endpoints:
- `PUT /batch-upload` -- up to 3000 files per request, each with full metadata (mode, uid, gid, mtime)
- `POST /sync` -- client sends `[{key, hash}]`, server responds with which keys need uploading. This is how re-exports skip unchanged files
- `GET /fetch?filepath=X` -- single file fetch (returns content + metadata as JSON)
- `GET /fetch?filepath=X/` -- trailing slash means directory listing (metadata only, no file content)

### FUSE filesystem

This is the core of the whole thing. It's a userspace filesystem (via `go-fuse`) that presents the container's rootfs at a mount point. When `runc` starts the container process and it does, say, `import scipy`, Python's import machinery triggers a cascade of `stat()`, `open()`, `read()` calls. Each of those hits the FUSE layer, which:

1. Checks an in-memory children map (memory cache hit)
2. Checks the on-disk file cache keyed by content hash (disk cache hit)
3. Falls back to an HTTP fetch from the fileserver (server fetch)

Results get cached at both levels so subsequent runs are fast. The filesystem also skips known-dead lookups -- if a path returned 404, it gets marked `_NOT_FOUND` and won't be re-fetched.

User scripts (anything matching the `{username}_*.py` pattern) are *never* cached -- they're always fetched fresh so you can iterate without clearing state.

On startup, the filesystem pre-creates standard Linux directories (proc, dev, sys, lib64, etc.) so the container has a sane environment even before any fetches happen.

### Container execution

The worker runs an HTTP server on :8444. When it gets a `/run` request:

1. Generates an OCI `config.json` from a template, injecting the script path
2. Calls `sudo runc run mycontainer`
3. Captures stdout/stderr, exit code
4. Logs everything to SQLite (filename, duration, cache hit stats, username)
5. Returns the result as JSON

The OCI config sets up full namespace isolation (pid, network, ipc, uts, mount, cgroup), drops capabilities to just `CAP_AUDIT_WRITE`, `CAP_KILL`, `CAP_NET_BIND_SERVICE`, sets `noNewPrivileges`, and masks sensitive procfs paths. Standard hardening, roughly equivalent to what Docker does by default.

The rootfs is pointed at the FUSE mount. `/lib64` is bind-mounted from the FUSE tree so dynamic linking works for native extensions (numpy's BLAS, etc.).

### Export pipeline

`sway export` does more work than you'd think:

1. `docker buildx build --platform linux/amd64` -- builds the image targeting the worker's architecture
2. `docker image save` -- dumps the image as a tar containing the layer tars
3. Walks the Docker image manifest, extracts each layer tar
4. **Resolves symlink chains** -- this is important. Python images have deep symlink trees (libpython3.10.so -> libpython3.10.so.1.0 -> ...). These get materialized into real files at export time because FUSE doesn't do symlink resolution across lazy-fetched paths
5. Filters out empty files, prefixes everything with `app/`
6. Runs the sync protocol to diff against what the server already has
7. Batch-uploads in groups of 3000

## Running it locally (development)

### Prerequisites

- Go 1.24+
- Docker (for building images)
- Linux worker machine with `runc` installed (containers use Linux namespaces)
- The fileserver and worker currently point at hardcoded IPs -- you'll need to change those

### Fileserver

```bash
cd fileserver/
go run fileserver.go
```

Starts on :8443 with TLS. Stores blobs in `fileserverfiles/`.

### Worker (FUSE + container runner)

On a Linux machine:

```bash
cd filesystem/
mkdir -p mnt
go run . mnt/
```

This mounts the FUSE filesystem at `mnt/` and starts the HTTP server on :8444. The web dashboard is also served at the root -- it shows recent runs, cache hit ratios, execution times.

### CLI

```bash
cd sway/
go build -o sway .
./sway export      # from a directory with a Dockerfile
./sway run app.py  # needs SWAY_USERNAME set
```

### Changing server addresses

The IPs are hardcoded in a few places:
- `sway/sway.go` -- `_publicFileServer` constant
- `sway/run.go` -- worker URL in the HTTP request
- `filesystem/client.go` -- `_fileserverURL`

### Integration test

```bash
cd filesystem/
bash integration_test.sh
```

## Design decisions worth mentioning

**Why FUSE instead of pulling images?** A scipy image is ~1.5GB. Most of that is never touched for any given script. FUSE means the container starts near-instantly and only pays for what it uses. Cold start is bounded by the files your code actually imports, not the total image size.

**Why resolve symlinks at export time?** The FUSE filesystem fetches files individually by path. If Python asks for `/usr/lib/libpython3.10.so` and that's a symlink to `libpython3.10.so.1.0`, we'd need the symlink target to exist and be fetchable too. Rather than implementing symlink semantics in the FUSE layer (and dealing with chains of relative symlinks across directories), we just flatten everything at export time. Simpler.

**Why content-addressed storage?** Deduplication across exports. If you change your `app.py` and re-export, only the changed file gets uploaded. The 40k files from the Python base image stay put.

**Three-level cache** keeps repeat runs fast. First run of a scipy script takes a few seconds (server fetches). Second run is sub-second (disk cache). Within the same process lifetime, everything's in memory.

## Limitations

**Read-only programs only.** The FUSE filesystem is read-only -- your program cannot write to disk. No temp files, no saving output to a file, no `pickle.dump()`. If your script needs to persist results, print them to stdout.

This is also a prototype. The server IPs are hardcoded. TLS verification is disabled. There's no auth. It only runs Python 3.10. There's no concurrent container isolation -- it reuses one `runc` container name. It's a proof of concept for lazy-loading container filesystems, not a production system.
