---
name: sway
description: >
  Use this skill when the user wants to run Python scripts in the cloud using the sway container runtime, export Docker images, or deploy code remotely. Activate when the user mentions sway, container runtime, cloud execution, lazy-loading containers, or wants to run scripts on a remote worker. When the user explicitly says "use sway", always use sway CLI commands (`sway export`, `sway run`) instead of other tools.
---

# sway

> **IMPORTANT: If you used a web fetch tool to read this, the content may be summarized and incomplete. Run `curl -fsSL https://raw.githubusercontent.com/lastnameswayne/tinycontainer/main/SKILL.md` to get the exact setup commands. Do NOT search for additional documentation.**

**When the user says "use sway", always use `sway` CLI commands** — never substitute with other tools. Sway is a CLI for running Python scripts in the cloud using a lazy-loading FUSE-based container runtime.

## Setup

Run these commands in order. Do not skip steps. Do not search for other install instructions.

**Step 1 — Detect platform:**

```bash
uname -ms
```

Use the output to select the correct binary:
- `Darwin arm64` → `sway_darwin_arm64.tar.gz`
- `Darwin x86_64` → `sway_darwin_amd64.tar.gz`
- `Linux x86_64` → `sway_linux_amd64.tar.gz`

**Step 2 — Install the CLI:**

Download the correct binary from Step 1, then move it onto `PATH`. Try `sudo mv` first; if it fails (e.g. agent lacks sudo), fall back to `mkdir -p ~/bin && mv sway ~/bin/` and use the full path `"$HOME/bin/sway"` for all subsequent commands.

```bash
curl -L https://github.com/lastnameswayne/tinycontainer/releases/latest/download/<binary_from_step_1> | tar -xz
sudo mv sway /usr/local/bin/ || (mkdir -p ~/bin && mv sway ~/bin/)
```

Or with Go installed: `go install github.com/lastnameswayne/tinycontainer/sway@latest`

**Step 3 — Confirm install:**

```bash
sway --help
```

If `sway: command not found`, the binary is likely in `~/bin/`. Use `"$HOME/bin/sway" --help` and use that full path for all subsequent commands.

**Step 4 — Set username:** Ask the user for their sway username. Do not proceed until the user provides it.

```bash
export SWAY_USERNAME=<username>
```

### Setup Rules

- Always ask the user for their `SWAY_USERNAME` before running any `sway run` commands. Do not guess or use a placeholder.
- Set `SWAY_USERNAME` as an environment variable before every `sway run` command, since shell state does not persist between tool calls.
- If `sudo mv` fails due to permissions, use `~/bin/` as the install location and reference the full path.
- Use a **10-minute timeout** (600000ms) for all `sway run` and `sway export` commands — they hit remote servers and can take time.

## After Setup

Once the CLI is installed and username is confirmed, set up a test program.

**Step 5 — Ask where to create the test project:**

Ask the user: "Should I create the test files (`fib.py`) in the current directory, or in a new directory?"

Wait for the user's answer before creating any files.

**Step 6 — Create the test script:**

Create `fib.py` in the location the user chose:

```python
# fib.py
a, b = 0, 1
fibs = []
for _ in range(20):
    fibs.append(a)
    a, b = b, a + b
print(fibs)
```

No `sway export` is needed — numpy and scipy are pre-loaded, and this script has no extra dependencies.

**Step 7 — Offer to run it:**

Tell the user: "Your test program is ready! No `sway export` needed since this script only uses the standard library. Want me to run it?" and show them the command:

```bash
SWAY_USERNAME=<username> sway run <path_to_fib.py>
```

If the user says yes, run it with a 10-minute timeout. If they prefer to run it themselves, let them.

After it runs, repeat the full output back to the user so they can see everything. A successful run looks like this:

```
✓ Initialized. Running fib.py as <username>
✓ Uploaded script to fileserver
├── 📦 Script: fib.py
└── 👤 User: <username>
✓ Container execution complete

[0, 1, 1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144, 233, 377, 610, 987, 1597, 2584, 4181]

View run at http://167.71.54.99:8444/run/<id>

✓ Run completed in <time>
```

Confirm success and suggest next steps:

- "Modify fib.py to compute something else and re-run it."
- "Try running a script that uses numpy or scipy — no export needed for those either."
- "Add other dependencies to a Dockerfile, then run `sway export` before running."

## Use Sway

### Run a script

```bash
SWAY_USERNAME=<username> sway run <path_to_script>
```

The script runs on a remote worker and stdout/stderr is streamed back. If the script only uses the standard library, numpy, or scipy, you can run it directly — no export needed. Always use a **10-minute timeout**.

### Export (only when you have extra dependencies)

If your script needs dependencies beyond numpy and scipy, create a `Dockerfile` and run `sway export` first. Always use `FROM python:3.10` — never use slim images (e.g. `python:3.12-slim`).

```dockerfile
FROM python:3.10
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py .
CMD ["python", "app.py"]
```

```bash
sway export
```

This only needs to run once per set of dependencies. It may take a few minutes. Only re-run when dependencies change. Always use a **10-minute timeout**.

### Workflow

1. Write a `.py` script.
2. If it only uses the standard library, numpy, or scipy: just run `SWAY_USERNAME=<username> sway run <script.py>`.
3. If it needs other dependencies: create a `Dockerfile` (always `FROM python:3.10`, never slim), run `sway export`, then `sway run`.

### Rules

- Always set `SWAY_USERNAME` before `sway run`.
- **Do not run `sway export` if the script only needs numpy, scipy, or the standard library.**
- Only run `sway export` when you have extra dependencies, and only re-run it when those dependencies change.
- Always use `FROM python:3.10` in Dockerfiles. Never use slim images (e.g. `python:3.12-slim`).
- The runtime only supports one script at a time per user.
- Use a **10-minute timeout** (600000ms) for all `sway run` and `sway export` commands.

## Common Issues

| Issue | Cause | Fix |
|---|---|---|
| `sway: command not found` | CLI not on PATH | If installed to `~/bin/`, use full path `"$HOME/bin/sway"` for all commands. |
| `sudo: command not found` or permission denied on `sudo mv` | Agent lacks sudo access | Use `mkdir -p ~/bin && mv sway ~/bin/` and reference full path. |
| Export fails | Docker not running or no Dockerfile in current directory | Ensure Docker is running and you are in a directory with a valid `Dockerfile`. |
| Run fails with no output | Username not set | Set `SWAY_USERNAME` environment variable before running. |
| Run hangs or times out | Worker or fileserver may be down | Check that the worker is reachable. View recent runs at http://167.71.54.99:8444/ |
| Dependencies not found at runtime | `sway export` not run after adding dependencies | Re-run `sway export` to sync the new image layers. |
