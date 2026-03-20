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

**Step 1 — Install the CLI:**

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

**Step 2 — Set username:** Ask the user for their sway username, then set it:

```bash
export SWAY_USERNAME=<username>
```

**Step 3 — Confirm readiness:**

```bash
sway --help
```

### Setup Rules

- Always ask the user for their `SWAY_USERNAME` before running any commands. Do not guess or use a placeholder.
- Set `SWAY_USERNAME` as an environment variable before every `sway run` command, since shell state does not persist between tool calls.

## After Setup

Provide:

- Confirmation that the CLI is installed (`sway --help` succeeds).
- The configured username.
- 2-3 simple starter prompts tailored to running scripts, for example:

  - "Create a Python script that computes the SVD of a random matrix and run it in the cloud."
  - "Write a script that fetches the current Bitcoin price and run it remotely."
  - "Run a Python one-liner that prints the first 20 Fibonacci numbers."

## Use Sway

### Export (first time or when dependencies change)

Before running scripts, the user needs a directory with a `Dockerfile` and their Python script. Export populates the fileserver with the container image layers:

```bash
sway export
```

This only needs to run once per set of dependencies. It may take a few minutes.

### Run a script

```bash
SWAY_USERNAME=<username> sway run <path_to_script>
```

The script runs on a remote worker and stdout/stderr is streamed back.

### Workflow

1. Ensure the working directory has a `Dockerfile` and a `.py` script.
2. Run `sway export` if this is the first run or dependencies changed.
3. Run `SWAY_USERNAME=<username> sway run <script.py>` to execute remotely.

### Rules

- Always set `SWAY_USERNAME` before `sway run`.
- Always run `sway export` before the first `sway run` in a new project.
- Only re-run `sway export` when dependencies in the Dockerfile change.
- The runtime only supports one script at a time per user.

## Common Issues

| Issue | Cause | Fix |
|---|---|---|
| `sway: command not found` | CLI not installed | Run the install commands from Step 1, then retry. |
| Export fails | Docker not running or no Dockerfile in current directory | Ensure Docker is running and you are in a directory with a valid `Dockerfile`. |
| Run fails with no output | Username not set | Set `SWAY_USERNAME` environment variable before running. |
| Run hangs or times out | Worker or fileserver may be down | Check that the worker is reachable. View recent runs at http://167.71.54.99:8444/ |
| Dependencies not found at runtime | `sway export` not run after adding dependencies | Re-run `sway export` to sync the new image layers. |
