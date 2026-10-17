# Results summary

Measured 11–13 October 2026 on two m6i instances in eu-north-1b (worker m6i.2xlarge, store m6i.xlarge, 0.14 ms apart; `machine.txt`). Definitions, deviations and caveats are in `METHODOLOGY.md`. Times are medians unless stated; every individual run is in the CSVs.

## Apps

57 apps: your 5, 6 extras and a 46-app suite across stdlib, numeric, data, ML, deep learning, text, images and web. **All 57 pass** after fixing 5 smoke-test failures:
- 2 were bugs in the apps: an unpinned dependency (`testappplotting`, `t-spacy`) and a missing dependency (`w-networkx`).
- 2 were tinycontainer path bugs, worked around in the apps (`pandas`, plus the data files of `s-csv` and `dl-onnxruntime`).
- 1 was fixed in tinycontainer, your commit `cb8545a` (`s-sqlite`).

## E0: instrumentation

- On a cold `testappscipy` run, the worker logged exactly the bytes and fetch count the fileserver served: **202,887,759 bytes in 2,019 fetches**. 0 of 33,360 events were dropped.
- Logging costs **+1.9%** (median first output 11.27 s with it on, 11.06 s off; p90 11.31 vs 11.08 s).

## E1: cold start at each cache level vs `docker pull` + `docker run`

- **tinycontainer's cold start beats Docker pull + run for 54 of 57 apps; the median app starts 3.2× faster.**
  - Small apps win big: `hello` 2.1 s vs 17.4 s (8.5×), `w-django` 2.9 s vs 18.9 s (6.5×).
  - The scientific stack wins by 1.8–2.8×: `testappscipy` 11.3 s vs 20.2 s, `pandas` 7.8 s vs 19.6 s.
- **Lazy loading loses at the large end.**
  - `dl-resnet18` roughly breaks even: 29.6 s vs 28.7 s.
  - `dl-tensorflow` is slower: 45.0 s vs 35.4 s.
  - **`reader`, which reads the whole image, takes 98.8 s vs 21.7 s for Docker (4.5× slower; measured to exit).**
- Docker pull + run is nearly flat, 17.3–35.4 s. About half of it is the pull (9–20 s). **The other half is the first container start waiting for the freshly unpacked layers to reach disk.** That is real Docker behaviour here, checked against a plain non-nested dockerd. With the image already present, `docker run` takes 0.2–2 s.
- **Warm (L2) is less than its name suggests.** Warm runs still fetched a **median 54% of bytes from the fileserver**, and torch 92%. The worker's disk-cache index is in memory and only rebuilt for directories that are listed again, so warm is only 1.3–1.5× faster than cold for most apps.
- **Hot (L1) is 3–7× faster than cold**, but still above Docker's warm run for the big stacks: TensorFlow hot is 18.4 s, Docker with the image present 2.0 s.
- **Bytes moved:** Docker pulls 410–720 MB compressed. tinycontainer's cold start fetches 38–925 MB (median app about 160 MB), which is less than Docker for every app except the deep-learning stacks and `reader`.

## E2: working set

- **The median app reads 7.8% of its image and fetches 10.6%.** The range is 1.7% (`hello`, 18 MB of 1.1 GB) to 37% (`dl-tensorflow`, 849 MB of 2.3 GB), and 98% for `reader`.
- **Fetched exceeds read by a median 1.29×** (up to 2× for small apps), because tinycontainer downloads whole files at lookup, including files that are only checked, never opened.
- **Python checks many paths that don't exist:** a median ~200 negative lookups per cold run, and 2,824 for TensorFlow.

## E3: when bytes arrive

- **Fetching is entirely front-loaded.** For the median app, 100% of fetched bytes arrive before the first output, i.e. during imports. The exceptions are apps that read data after starting: `w-networkx` (32% before first output) and `reader` (3%).
- **The fetch order is almost deterministic:** the first 200 fetched paths overlap with **Jaccard similarity 0.99** across `testappscipy`'s cold runs. Recording a trace and prefetching it in parallel is therefore viable, which is relevant because fetches are currently made one at a time.

## E4: dedup, whole files vs chunks (Lambda Fig. 5)

- The corpus is 477 images: 57 apps, 8 version variants, 30 GitHub repos (14 more failed to build), and 382 official Docker Hub images. The Docker Hub set is every official image in order of pull count, `latest` plus up to four more tags each, pulled from AWS's public mirror; 68 tags weren't on the mirror.
- **By kind of image (whole-file store, median new bytes per image):** Python apps and variants like tinycontainer users build **2.5%**, the same as Lambda's reported median; official Docker Hub images 23%; GitHub Python repos 51%. Overall, 466 GB of image content fits in 148 GB of store (68% less).
- **Store size after all 477 images:**
  - whole files: 148.4 GB in 1,735,776 objects
  - 4 MiB chunks: 144.0 GB, −3.0%
  - 512 KiB chunks: 141.7 GB, −4.6%
  - 64 KiB chunks: 139.4 GB, **−6.1%**, but **2.1×** the objects
- **Per image** (461 images that aren't exact re-uploads): the median new image is **21.5% new bytes as whole files and 16.6% as 64 KiB chunks** (means 32.5% and 30.0%). The largest quarter of images (over 1.3 GB) dedups slightly better: a median of 16.9% against 22.0% for the rest.
- **Far above Lambda's 2.5% median.** This corpus mixes many unrelated base images (Alpine, Debian, Ubuntu, distroless), while Lambda's customers mostly build on a few shared bases, and the paper excludes the ~80% of uploads that are byte-for-byte re-uploads. Here only 3.2% of images were entirely already stored.
- **Whole-file dedup still gets most of the benefit** at half the objects. Totals are identical across the three orderings.
- A first pass with only the 95 images other than Docker Hub's gave a median of about 6% new bytes; that corpus was dominated by my own similar Python apps.

## E5: cache tiers under a mixed workload (Lambda Fig. 7)

- The workload is 37 apps that can share one store (19 of 57 can't, because of real version clashes), 200 runs per disk-cache cap on one live worker.
- **Share of reads by tier, median [p10–p90]:**
  - 2 GiB cap: memory 61% [37–91], disk 36% [7–61], **remote 0.3%** [0.1–12]
  - 512 MiB cap: memory 61%, disk 27%, **remote 1.5%** [0.2–37]
- **Remote reads fade as the cache warms:** 7.5% of reads in the first 20 runs, 0.2% (2 GiB) and 1.2% (512 MiB) over the last 100.
- **Median start: 1.46 s with the 2 GiB cap, 1.80 s with 512 MiB.**

## E6: per-file latency (Lambda Fig. 11)

- The data is 454,178 file lookups from E5. Lookup latency includes fetching the file, because tinycontainer fetches at lookup; reads are always served from memory in under 1 µs.
- **Disk:** p50 12 µs, p99 0.23 ms.
- **Remote:** p50 0.68 ms, p90 3.1 ms, p99 12.7 ms.
- **Remote latency depends on size:**
  - under 10 KB: 0.27 ms, limited by round-trip time
  - 10–100 KB: 2.6 ms
  - 1–10 MB: 43 ms
  - over 10 MB: 450 ms, limited by bandwidth (whole files as base64 JSON)
- **Surprise: about 55% of remote fetches and every 404 cost about 2.2 ms because they open a new TCP+TLS connection.** A diagnostic cold run of `w-django` made 1,508 requests and opened **915** new connections. The cause is that the worker client doesn't read responses to the end (it returns early on 404, and the JSON decoder stops before the trailing newline), so Go's HTTP client can't reuse the connection. **Draining the body before closing it** should bring most fetches to about 0.3 ms.
- **Negative lookups:** a median 237 per run. Half are answered from the worker's cache in 0 µs; the other half are 404s from the fileserver, at 2.2 ms each.

## E7: hedged requests (Lambda Fig. 9 counterpart)

- Set-up: injected 5 ± 3 ms delay with 0.2% loss on worker→fileserver traffic. The hedge fires after 29 ms (the p90 of the unhedged pilot runs). 30 runs each way.
- **The tail improves:**
  - p99 fetch latency 114 → 77 ms
  - p99.9 1,031 → 502 ms
  - cold start 51.9 → **46.3 s (−11%)**
  - p50 and p90 are unchanged
- **The cost is +2.2% requests but +55% bytes.** The fetches that are slow enough to hedge are mostly large files, so each hedge duplicates a big transfer, and the hedge itself won only 0.5% of the time. A size-aware hedge (only hedge small files, or hedge by expected time) would keep the tail win without the extra bytes.

## E8: distance between worker and store

| Added round trip | `hello` (s) | `testappscipy` (s) | `dl-tensorflow` (s) | Docker pull + run (s) |
|---|---|---|---|---|
| 0 ms | 2.1 | 11.3 | 45.0 | 17–37 |
| +2 ms | 4.8 | 24.8 | 91.7 | |
| +10 ms | 15.9 | 79.3 | 270.9 | |
| +40 ms | 56.9 | 282.0 | 944.4 | 17–37 (almost flat) |

- **Docker overtakes lazy loading at +10 ms for most apps, at +2 ms for the scipy and torch stacks, and at +40 ms for even the smallest apps.**
- `reader` exceeded the 30-minute timeout at +40 ms (about 32,000 one-at-a-time fetches).
- Lazy loading only wins when the store is close: same AZ or better. With one-at-a-time fetches, cold start grows by roughly (number of fetches × added round-trip time).

## E9: first-time images (Lambda Fig. 8 counterpart, worker-local)

- When a worker sees an image for the first time, after running other compatible images, **a median 37% of the bytes it fetches (63% of files) are already on its disk** (p10 15%, p90 60%).
- 72 of 74 arrivals succeeded. The 2 failures were a `click` version clash my compatibility check missed (stale manifest).
- **A first E9 run over all 57 apps, with incompatible images sharing one store, broke 63 of 114 first-time runs at import** (numpy 1.26 and 2.x files mixed in one directory). That run is kept in `raw/superseded_e9_all_apps/` and is the clearest evidence of the single-namespace problem.

## E10: what precompiled `.pyc` files are worth (your commit `e3b26f3`)

- **Cold: 0–9% faster** (`n-sympy` 9%). Cold start is dominated by fetching.
- **Hot: 1.1–2.0× faster** (`n-sympy` 2.0×, `w-django` and `w-fastapi` 1.5×, torch 1.4×, TensorFlow 1.2×). Once files are local, compiling every import from source was a large share of start time.

## tinycontainer bugs found along the way (yours to fix)

1. **Connection reuse:** the worker doesn't drain HTTP response bodies, so ~60% of fetches pay a new TLS handshake (E6).
2. **The `/app` path:** `sway export` (`tarread.go`) doesn't prefix paths that already start with `app/`, so the image's `/app` directory lands at the container root.
3. **Pre-created directories:** `/opt`, `/home`, `/lib`, `/media` and `/mnt` are local empty directories without the `app/` prefix, so nothing under them can be fetched.
4. **Fileserver restart:** blob names are content hashes, so after a restart only one path survives per unique content.
5. **Warm cache:** the disk-cache index isn't persisted, so most "warm" fetches still go remote (E1).
6. **Stale content:** a live worker keeps serving old content for paths after a re-export.
7. **Single namespace:** images with different dependency versions overwrite each other (E5: 37 of 57 can coexist; E9).
8. **Newer Docker:** with docker 29's containerd image store, `sway export` silently uploads 0 files (compressed layers).
9. **Fixed during this work** (your commits `ced8b27`, `df882be`, `e3b26f3`, `cb8545a`): the `.json`/`.txt` filter, no writable `/tmp`, home or `/var/tmp`, the 1 GiB/128-pid limits, and `__pycache__` hidden with mtime 0.

## What failed or was re-run

- **E5:** the first run used one app because my compatibility check was too strict. Re-run; the old data is in `raw/superseded_e5_single_app/`.
- **E9:** the first run used incompatible images (see above). Re-run on compatible apps.
- **E4:** the first analysis dropped the GitHub images (a `\r` in my corpus file). Re-analysed from the same hashes, then extended with 382 Docker Hub images.
- **E8:** `reader` timed out at +40 ms (>30 min), and is recorded as such.
