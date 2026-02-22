# tinycontainerruntime

A container runtime built around lazy-loading container filesystems via FUSE. The container never has the full image. Instead, a userspace filesystem intercepts `stat()`/`open()`/`read()` calls and fetches files on-demand from a content-addressed fileserver, with a three-level cache (memory, disk, server) to make subsequent runs faster.

Built to understand the infrastructure that makes Modal work. This is just for fun.

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
                           ┌─────────────────────────────────────────┐
                           │              fileserver                  │
                           │       content-addressed blob store       │
                           │         (SHA1-keyed, TLS :8443)         │
                           └──────────┬──────────────────────────────┘
                                      │
                              on-demand fetches
                                      │
┌──────────┐    run request    ┌──────▼───────────────────────────────┐
│          │ ─────────────────>│               worker                  │
│ sway CLI │                   │                                      │
│          │ ─── export ──────>│  ┌────────────────────────────────┐  │
└──────────┘   (to fileserver) │  │     FUSE filesystem (go-fuse)  │  │
                               │  │                                │  │
                               │  │  lookup chain:                 │  │
                               │  │   1. memory cache (children)   │  │
                               │  │   2. disk cache (filecache/)   │  │
                               │  │   3. server fetch (HTTP GET)   │  │
                               │  └───────────────┬────────────────┘  │
                               │                  │ mounted as rootfs  │
                               │  ┌───────────────▼────────────────┐  │
                               │  │      runc container (OCI)      │  │
                               │  │  namespaces: pid,net,ipc,uts,  │  │
                               │  │    mount,cgroup                │  │
                               │  │  caps: audit_write,kill,       │  │
                               │  │    net_bind_service             │  │
                               │  │  limits: 1GB mem, 128 PIDs     │  │
                               │  └────────────────────────────────┘  │
                               │                                      │
                               │  SQLite: logs per-run cache stats    │
                               └──────────────────────────────────────┘
```


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
cd sway/
go build -o sway .
./sway export           # from a directory with a Dockerfile
SWAY_USERNAME=yourname ./sway run app.py
```

### Integration tests

```bash
cd filesystem/
bash integration_test.sh
```

Runs the test apps in the test apps folder. 
