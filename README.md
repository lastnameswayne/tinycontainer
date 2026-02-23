# tinycontainerruntime

A container runtime built around lazy-loading container filesystems via FUSE. Built to understand the infrastructure that makes Modal work. This is just for fun.

View recent runs at http://167.71.54.99:8444/

## Quick start example

```dockerfile
FROM python:3.10
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py .
CMD ["python", "app.py"]
```

```python
# app.py
import numpy as np
from scipy import linalg

a = np.random.randn(100, 100)
u, s, vh = linalg.svd(a)
print(f"svd: u={u.shape}, s={s.shape}")
```

Install the CLI

```bash
# macOS (Apple Silicon)
curl -L https://github.com/lastnameswayne/tinycontainer/releases/latest/download/sway_darwin_arm64.tar.gz | tar -xz
sudo mv sway /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/lastnameswayne/tinycontainer/releases/latest/download/sway_darwin_amd64.tar.gz | tar -xz
sudo mv sway /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/lastnameswayne/tinycontainer/releases/latest/download/sway_linux_amd64.tar.gz | tar -xz
sudo mv sway /usr/local/bin/
```

Or with Go installed: `go install github.com/lastnameswayne/tinycontainer/sway@latest`

```bash
sway export # populates fileserver
export SWAY_USERNAME=yourname
sway run app.py # runs the script
# svd: u=(100, 100), s=(100,), vh=(100, 100)
```

`sway` is the cli used to run the code! There are two commands, `sway export` and `sway run`:
- `sway export` reads the docker file and sends all the required files to the fileserver. Run this when you add a new dependency. It might take a few minutes...
- `sway run <path_to_script>` runs the script in the cloud and retuns the result. 

## Architecture

Cold start latency should be bounded by the files a process actually touches, not by total image size. A scipy image is ~1.5GB. `import scipy; scipy.linalg.svd(...)` touches maybe 50MB of that. By mounting a FUSE filesystem as the container rootfs and fetching lazily, the container starts in seconds instead of waiting for a full image pull.

```
                              ┌──────────────────────────────────┐
                              │           fileserver              │
                              │    content-addressed blob store   │
                              └───────────────┬──────────────────┘
                                              │ on-demand fetches
                                              │
┌──────────┐   run request    ┌───────────────▼──────────────────┐
│ sway CLI │ ────────────────>│   worker (FUSE filesystem)       │
└──────────┘                  │                                  │
                              │   lookup chain:                  │
                              │    1. memory cache (children)    │
                              │    2. disk cache (filecache/)    │
                              │    3. server fetch (HTTP GET)    │
                              └──────────────────────────────────┘
```

**Fileserver** -- content-addressed blob store. Files keyed by SHA1. `sway export` populates it; after that it just serves fetches.

**Worker** -- mounts a FUSE filesystem (`go-fuse`) as the container rootfs, then runs containers via `runc`. When the container process touches a file, FUSE checks memory cache, then disk cache, then fetches from the fileserver. Logs cache stats per run to SQLite.

**CLI** -- `sway export` builds and syncs the image. `sway run` sends the script to the worker and streams back stdout/stderr.


## Running it locally

### Prerequisites

- Go 1.24+
- Docker (for image builds)
- Linux worker machine with `runc` installed
- Server addresses are hardcoded in `sway/sway.go`, `sway/run.go`, and `filesystem/client.go`

### Fileserver

```bash
cd fileserver/
go run fileserver.go    # starts on :8443 with TLS
```

### Worker

```bash
cd filesystem/
mkdir -p mnt
go run . mnt/           # mounts FUSE at mnt/, HTTP server on :8444
```

### CLI

```bash
sway export             # from a directory with a Dockerfile
SWAY_USERNAME=yourname sway run app.py
```

### Integration tests

```bash
cd filesystem/
bash integration_test.sh
```

Runs the test apps in the test apps folder. 
