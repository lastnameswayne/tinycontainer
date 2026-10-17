# Methodology

Status: **measured** on 11–13 October 2026 (two EC2 instances, see `machine.txt`). Numbers and findings are in `SUMMARY.md`; every individual run is in the CSVs.

## Machines and network

- Two EC2 instances in the same AZ and subnet:
  - **Worker box** (m6i.2xlarge): the FUSE worker, runc, the docker baseline (an isolated `docker:dind`), and image builds.
  - **Store box** (m6i.xlarge): the fileserver and a `registry:2`.
- Instance types, kernel, disks and the measured round-trip time between the boxes are in `machine.txt` (`scripts/record_machine.sh`). One machine also works: leave `STORE_HOST` unset.
- The store box's page cache is **never dropped**. A real store serves hot blobs from memory; dropping its cache would add disk reads a production store doesn't have.
- Separate ports (fileserver `:9443`, worker `:9444`, registry `:5000`) and directories (`/opt/tc-exp`) keep the stack apart from any production instance on the same host.

## Apps

- **Core:** the apps already known to run on tinycontainer: `testapp2`, `testapp3`, `testappscipy`, `testappplotting`, `ml-inference`.
- **Extra:** `hello`, `numpy`, `scipy`, `pandas`, `torch`, `reader`. `reader` reads every file in the image, which is the worst case for lazy loading.
- **Suite** (`testapps/suite/`, 46 apps): short realistic tasks chosen to vary image size, import shape (a few huge `.so` files vs thousands of small `.py` files), how much of the image is read, and what happens after imports. Domains are stdlib, numeric, data, classical ML, deep learning on CPU, text, images, web handlers, plus 5 base-image variants (python 3.9, 3.11, 3.12).
- All suite apps are `FROM python:3.x`, with versions pinned and shared across apps (numpy 1.26.4 everywhere), so images can share a store.
- Each app prints `ready: <app>` right after its imports, so first output means "imports done".
- The version measured includes your commits `cb8545a` (tmpfs `/var/tmp`: SQLite and others pick it for temp files before `/tmp`), `ced8b27` (`.json`/`.txt` served again), `df882be` (tmpfs `/tmp` and `/root`, `HOME=/root`, 8 GiB memory, 4096 pids) and `e3b26f3` (real file mtimes, `__pycache__` lookups allowed, so precompiled `.pyc` files are used).
- Remaining limits the apps respect: there is no network inside the container, so data and models are built into the image (`build.py`). Apart from the tmpfs `/tmp`, `/root` and `/dev/shm`, the root filesystem is read-only.
- **Smoke test:** before measuring, each app is exported and run once. Failures are classified (build, export, import error, missing file or shared library, read-only filesystem, out of memory, too many threads, timeout) in `smoke.csv`. Only passing apps (`apps_ok.txt`) are measured. The pass rate is reported, not hidden.
- Not a design change, but needed for the measurement: `PYTHONUNBUFFERED=1` is set for both runc and docker. Without it, Python buffers piped stdout, and the first output would only arrive at exit.

- Fixes to the apps found by the smoke test (none changes what is measured): `testappplotting` pinned to `plotext<6` (plotext 6.1.0 removed `plot`, the app fails in plain Docker too); `t-spacy` pins `typer`/`click`; `w-networkx` adds numpy/scipy; data files of `pandas`, `s-csv` and `dl-onnxruntime` live in `/usr/local/share/tcdata` because of two tinycontainer path bugs (see Known findings). Result: 57 of 57 apps pass.
- `reader` prints its first line right after `import os`, then reads the whole image, so its headline time is **exit**, not first output (marked `*` in plots; `metric` column in `e2_scatter.csv`).

## Cache states (`scripts/set_cache_state.sh`)

| state | worker's filecache/ | worker process | worker box page cache |
|---|---|---|---|
| cold (L3: fileserver) | emptied | restarted | dropped |
| warm (L2: worker disk) | kept from the previous run of the same app | restarted | dropped |
| hot (L1: memory) | kept | not restarted (second consecutive run) | not dropped |

Measured (`e1_tier_check.csv`): cold runs fetch 99.8% of bytes remotely as intended, but **warm runs still fetch a median 54% of bytes from the fileserver** (torch: 92%), because the worker's disk-cache index lives in memory and is only rebuilt for directories that are listed again. "Warm" therefore means "worker restarted with its disk cache present", not "served from disk". "Hot" is served mostly by the kernel's dentry and page caches before requests reach the worker. So L1 means "local memory, kernel plus worker". `e1_tier_check.csv` reports, for every app and state, which tier actually served the file fetches, so each label can be checked against the data.

## Timing

- `t_first_output_ms` and `t_exit_ms` are measured from the `sway run` invocation, on a monotonic clock. `sway run` doesn't stream output, so the worker timestamps the first stdout byte and the exit relative to the moment the request arrived, plus the total handler time. The harness converts to the client clock with `pre = client_total − handler_ms`. Post-exit bookkeeping (`runc delete`, the event flush and the DB write) is excluded.
- **docker pull + run:** the worker box's page cache is dropped and the isolated dockerd emptied. Then `docker pull` and `docker run` are timed as one sequence from the start of the pull. The image comes from the registry on the store box. The pull's own duration is kept separately (`pull_ms`).
- **docker run, image present:** a reference point to show the Docker runtime itself isn't the slow part.
- **The first `docker run` after a pull takes 8–21 s** (hello 8.6 s, scipy 11 s, TensorFlow 21 s) and the second 0.2–2.7 s. Checked on 13 Oct: a second, ordinary (not nested) dockerd with an empty store on the same host behaves identically, and syncing the disk between pull and run cuts the first run to 1.3 s. So it is the first container start waiting for the freshly unpacked layers to reach the EBS disk: real Docker behaviour on this hardware, not an artefact of the isolated (dind) daemon.
- docker uses the same limits and environment as the runc config: 1 CPU, 8 GiB memory, 4096 pids, no network, `PYTHONUNBUFFERED=1`. Timings include the `docker exec` hop into dind, which is tens of ms.

## Instrumentation (E0)

- `file_events(run_id, ts_us, op, path, size_bytes, tier, latency_us, hit, hedge)` is stored in the worker's `runs.db`.
  - `op` is one of `lookup`, `open`, `read` or `readdir`. For `readdir`, `size_bytes` holds the number of entries.
  - `tier` is one of `mem`, `disk`, `remote`, `negative` or `error`.
  - `hit` = 1 means the fileserver wasn't contacted.
  - `hedge`: 0 = none sent, 1 = sent and the original won, 2 = the hedge won.
- Latency is measured inside the FUSE handler. Events go through a 65,536-entry channel to one writer goroutine, which inserts in batches. If the channel is full, events are dropped and counted (`runs.dropped_events`).
- The fileserver counts what it serves (`GET /metrics`, reset with `POST /metrics/reset`).
- **Sanity check** (`e0_sanity.csv`): on one cold run, the worker's logged remote bytes and fetch count must exactly equal what the fileserver reports. `run_all.sh` stops otherwise.
- **Logging overhead** (`e0_logging_overhead.csv`): 10 cold runs with logging on and 10 off, interleaved. _(Filled in by the run.)_

### What the tiers mean in tinycontainer

- tinycontainer fetches a file's **whole content during `Lookup`**. The tier is therefore decided at lookup, and `Read` is always served from worker memory.
- The worker's in-memory map holds only directories. A repeated file lookup on a live worker goes to the disk cache.
- Lookups the kernel answers (5-minute entry timeout) never reach FUSE, so they aren't in `file_events`.
- After a restart, disk hits only happen for files in directories that have been listed again (`Readdir` repopulates the index).
- E6 uses file-lookup latency, including the fetch, as the counterpart of Lambda's Fig. 11 read latency.
- E5 attributes each read to the tier of that run's first lookup of the file. Reads of files not looked up in the run count as `mem`.

## Stores

- **E0–E3, E7, E8:** one fresh store per app, so "the image" is exactly that app's image.
- **E5:** one shared store. tinycontainer has a single namespace, so two images with different content at the same path overwrite each other. `select_union.py` finds the largest group of apps whose files don't conflict (ignoring `__pycache__` and `/app/`, which only holds each image's build inputs): **37 of 57** (`reader` excluded on purpose; it would load every image). The other 19 have genuine version clashes (numpy 2.x in the unpinned apps and jax, other Python versions, transitive versions of pydantic/sympy/Pillow/jinja2): `raw/union_conflicts.csv`. A first E5 run used a wrong selection (one app); it is kept in `raw/superseded_e5_single_app/`.
- **E9:** images are exported into one store, one at a time, immediately before each is run, drawn from E5's 37 compatible apps. A first run over all 57 apps is kept in `raw/superseded_e9_all_apps/`: mixing numpy 1.26 and 2.x files in one directory broke **63 of 114** first-time runs at import, which is itself the clearest evidence of the single-namespace limitation.
- The fileserver is **never restarted between export and runs**. Blob files are named by content hash only. On restart, the index keeps one path per unique content, and other paths with the same content (e.g. files that were symlinks) disappear. This is a bug in the current fileserver.

## Experiments

| | Question | Conditions |
|---|---|---|
| E1 | Start time from each cache level vs docker pull + run | per app: cold, warm, hot, docker pull+run, docker run with the image present; `E1_REPS` = 5 by default, interleaved |
| E2 | Share of the image read vs fetched; negative lookups | from E1's cold runs |
| E3 | When bytes arrive during a cold start; is fetch order stable? | the cold run with the median `t_exit_ms`; Jaccard similarity of the first 200 fetched paths over `KEY_APP`'s cold runs |
| E4 | Whole-file vs chunk dedup (Fig. 5) | every app image, 8 version variants of `testappscipy`, and up to 30 GitHub repos (`scripts/select_github_corpus.py`: most-starred Python repos with a root Dockerfile running `pip install`, pinned commits, built in order, first 30 that build in under 10 minutes); 3 orderings |
| E5 | Tier shares under a mixed workload (Fig. 7) | conflict-free apps, Zipf-like weights, 200 runs on one live worker, disk cache caps of 2 GiB and 512 MiB |
| E6 | Per-file latency distribution (Fig. 11) | from E5 |
| E7 | Hedged requests (Fig. 9 counterpart) | netem on worker→fileserver traffic (`raw/e7_netem.txt`); threshold = p90 of 10 unhedged pilot runs under the same netem (`raw/e7_threshold.txt`); 30 runs off and 30 on, interleaved |
| E8 | Cold start vs docker pull + run as round-trip time grows | +0, +2, +10 and +40 ms on top of the real RTT, on a subset of apps spanning size and import shape |
| E10 | What precompiled `.pyc` files are worth | per app in E8's subset: cold and hot with `.pyc` files used vs with `SWAY_PYC=0` (the behaviour before `e3b26f3`: every module compiled from source), interleaved |
| E9 | Share of a first-time image's fetches already cached locally (Fig. 8 counterpart: worker-local, not an AZ-shared cache) | 2 random arrival orders over all passing apps; worker restarted between arrivals with its disk cache kept |

Fig. 8 of the Lambda paper (the AZ-shared L2 hit rate) can't be reproduced directly, because tinycontainer has no shared cache tier. E9 is the closest honest equivalent.

## Known findings from reading the code (relevant to interpreting the numbers)

- Fixed before measuring, in your commits: the fileserver refused `.json`/`.txt`; containers had no writable `/tmp` or home and were limited to 1 GiB and 128 pids; the worker hid `__pycache__` and reported mtime 0, so Python recompiled every import from source on every run (E10 measures what that cost).
- `sway export` (tarread.go) does not prefix paths that already start with `app/`, so the image's `/app` directory lands at the container root (`/app/data.csv` becomes `/data.csv`).
- The worker pre-creates `home`, `lib`, `media`, `mnt`, `opt`, `proc`, `dev`, `sys`, `lib64` as local directories whose paths lack the `app/` prefix, so nothing under `/opt`, `/home`, `/media` or `/mnt` can be fetched (and `/lib` lookups miss; the loader falls back to `/usr/lib`).
- With docker 29's default containerd image store, `docker save` writes gzip-compressed layers that `sway export` can't read; it then "succeeds" with 0 files. The worker box uses the classic image store (as Docker Desktop 27 does).
- E8: `reader` at +40 ms RTT exceeded the 30-minute run timeout (about 32,000 sequential fetches); recorded as a timeout, not a measurement.
- A live worker keeps serving the old content of paths it has already loaded after a re-export (kernel caches plus worker memory).
- `isScript()` only recognises `<user>_app.py` usernames without an underscore, so the harness uses `exp<app>` usernames.
