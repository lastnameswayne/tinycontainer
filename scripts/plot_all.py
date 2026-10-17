#!/usr/bin/env python3
"""One plot per experiment from the CSVs in results/, written to results/plots/ as PNG
and SVG. Plots whose input CSV is missing are skipped."""
import os
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402
import numpy as np  # noqa: E402
import pandas as pd  # noqa: E402

REPO = Path(os.environ.get("REPO", Path(__file__).resolve().parents[1]))
RESULTS = Path(os.environ.get("RESULTS", REPO / "results"))
PLOTS = RESULTS / "plots"
APPS = os.environ.get("APPS", "testapp2 testapp3 testappscipy testappplotting ml-inference").split()
CONDITIONS = ["docker_pull_run", "cold", "warm", "hot"]
EXIT_METRIC_APPS = {"reader"}  # see analyze.py: plotted by exit time, marked with *
LABELS = {"docker_pull_run": "docker pull + run", "cold": "cold (L3: fileserver)",
          "warm": "warm (L2: worker disk)", "hot": "hot (L1: memory)"}
TIERS = ["mem", "disk", "remote"]

plt.rcParams.update({"axes.grid": False, "axes.spines.top": False, "axes.spines.right": False,
                     "figure.dpi": 120, "savefig.bbox": "tight"})


def save(fig, name):
    PLOTS.mkdir(parents=True, exist_ok=True)
    for ext in ("png", "svg"):
        fig.savefig(PLOTS / f"{name}.{ext}")
    plt.close(fig)
    print(f"plots/{name}.png")


def read(name):
    p = RESULTS / name
    return pd.read_csv(p) if p.exists() else None


def ecdf(ax, values, **kw):
    v = np.sort(np.asarray(values, dtype=float))
    v = v[~np.isnan(v)]
    if len(v):
        ax.step(v, np.arange(1, len(v) + 1) / len(v), where="post", **kw)
    return v


def e1():
    df = read("e1_coldstart.csv")
    if df is None:
        return
    df = df[df.exit_code == 0].copy()
    df["t_first_output_ms"] = df.t_exit_ms.where(df.app.isin(EXIT_METRIC_APPS), df.t_first_output_ms)
    # order apps by their median cold start, fastest first
    cold = df[df.condition == "cold"].groupby("app").t_first_output_ms.median()
    apps = [a for a in cold.sort_values().index if a in set(APPS) or not APPS]
    nbytes = read("e1_bytes.csv")
    width_in = max(10, 0.32 * len(apps))  # ~50 apps need room
    fig, axes = plt.subplots(2 if nbytes is not None else 1, 1, figsize=(width_in, 8 if nbytes is not None else 5),
                             squeeze=False, sharex=True,
                             gridspec_kw={"height_ratios": [2.2, 1]} if nbytes is not None else None)
    ax = axes[0][0]
    width = 0.8 / len(CONDITIONS)
    x0 = np.arange(len(apps))
    for i, cond in enumerate(CONDITIONS):
        sel = [df[(df.app == a) & (df.condition == cond)].t_first_output_ms for a in apps]
        med = [v.median() for v in sel]
        p90 = [v.quantile(0.9) for v in sel]
        x = x0 + (i - (len(CONDITIONS) - 1) / 2) * width
        ax.bar(x, med, width, label=LABELS[cond])
        ax.errorbar(x, med, yerr=[np.zeros(len(apps)), np.subtract(p90, med)], fmt="none", ecolor="black",
                    capsize=2, lw=0.8)
    cached = [df[(df.app == a) & (df.condition == "docker_run_cached")].t_first_output_ms.median() for a in apps]
    first_x = x0 - (len(CONDITIONS) - 1) / 2 * width
    ax.scatter(first_x, cached, marker="_", s=300, color="black", zorder=3,
               label="docker run, image already present")
    rot = 90 if len(apps) > 10 else 0
    ticks = [a + "*" if a in EXIT_METRIC_APPS else a for a in apps]
    ax.set_xticks(x0, ticks, rotation=rot, ha="center")
    ax.set_xlim(-0.6, len(apps) - 0.4)
    ax.set_yscale("log")
    ax.set_ylabel("time to first output, ms\n(median; whisker = p90; * = time to exit)")
    ax.legend(frameon=False, ncol=3, loc="lower center", bbox_to_anchor=(0.5, 1.0), fontsize=8)
    if nbytes is not None:
        bx = axes[1][0]
        b = nbytes.set_index("app").reindex(apps)
        bx.bar(x0 - 0.2, b.docker_pulled_bytes / 1e6, 0.4, label="docker pull (compressed image)")
        bx.bar(x0 + 0.2, b.tc_cold_wire_bytes / 1e6, 0.4, label="tinycontainer cold (on the wire)")
        bx.set_xticks(x0, ticks, rotation=rot, ha="center")
        ax.tick_params(labelbottom=False)
        bx.set_yscale("log")
        bx.set_ylabel("MB transferred")
        bx.set_xlabel("app")
        bx.legend(frameon=False, fontsize=8)
    else:
        ax.set_xlabel("app")
    save(fig, "e1_coldstart")


def e2():
    df = read("e2_scatter.csv")
    if df is None:
        return
    fig, ax = plt.subplots(figsize=(6.5, 4.5))
    x = df.touched_bytes / 1e6
    ax.scatter(x, df.cold_t_first_output_ms_median, zorder=3, label="tinycontainer cold start (median)")
    for xi, ref, app, y in zip(x, df.docker_pull_plus_run_ms, df.app, df.cold_t_first_output_ms_median):
        ax.hlines(ref, xi / 1.25, xi * 1.25, colors="black", lw=1.5,
                  label="docker pull + run, same app (median)" if app == df.app.iloc[0] else None)
        ax.annotate(app, (xi, y), textcoords="offset points", xytext=(5, 5), fontsize=8)
    ax.set_xscale("log")
    ax.set_yscale("log")
    ax.set_xlabel("bytes touched by the process, MB")
    ax.set_ylabel("time to first output, ms")
    ax.legend(frameon=False, fontsize=8)
    save(fig, "e2_workingset")


def e3():
    fig, ax = plt.subplots(figsize=(7, 4.5))
    n = 0
    for app in APPS:
        df = read(f"e3_timeline_{app}.csv")
        if df is None or df.empty:
            continue
        n += 1
        line, = ax.plot(df.ts_ms, df.cumulative_bytes / 1e6, label=app)
        first = df.worker_first_output_ms.iloc[0]
        y = np.interp(first, df.ts_ms, df.cumulative_bytes / 1e6)
        ax.plot([first], [y], marker="|", markersize=14, color=line.get_color())
    if not n:
        plt.close(fig)
        return
    ax.set_xlabel("ms since the run request reached the worker")
    ax.set_ylabel("cumulative MB fetched from the fileserver")
    if n > 1:
        ax.legend(frameon=False, title="| = first output")
    save(fig, "e3_timeline")


def e4():
    df = read("e4_dedup.csv")
    if df is None:
        return
    s = df[(df.ordering == "sorted") & (df.order > 0)]  # the first image is 100% new by construction
    fig, ax = plt.subplots(figsize=(6, 4.5))
    for scheme, g in s.groupby("scheme", sort=False):
        ecdf(ax, g.pct_unique, label=scheme)
    ax.set_xlabel("% of the image's bytes new to the store")
    ax.set_ylabel("cumulative fraction of images")
    ax.set_xlim(0, 100)
    ax.legend(frameon=False)
    save(fig, "e4_dedup_cdf")

    fig, ax = plt.subplots(figsize=(6, 4.5))
    for scheme, g in df[df.ordering == "sorted"].groupby("scheme", sort=False):
        ax.plot(g.order + 1, g.store_bytes / 1e9, label=scheme)
    ax.set_xlabel("images added")
    ax.set_ylabel("store size, GB")
    ax.legend(frameon=False)
    save(fig, "e4_store_growth")

    fig, ax = plt.subplots(figsize=(6, 4.5))
    for ordering, g in df[df.order > 0].groupby("ordering"):
        ecdf(ax, g[g.scheme == "file"].pct_unique, label=f"file, {ordering}")
    ax.set_xlabel("% of the image's bytes new to the store")
    ax.set_ylabel("cumulative fraction of images")
    ax.set_xlim(0, 100)
    ax.legend(frameon=False)
    save(fig, "e4_dedup_orderings")


def e5():
    df = read("e5_tiers.csv")
    if df is None:
        return
    tot = df.groupby("run_id").reads.transform("sum")
    df = df.assign(pct=np.where(tot > 0, 100 * df.reads / tot, np.nan))
    caps = sorted(df.cache_cap_bytes.unique(), reverse=True)
    fig, axes = plt.subplots(1, len(caps), figsize=(4.5 * len(caps), 4.5), sharey=True, squeeze=False)
    for ax, cap in zip(axes[0], caps):
        d = df[df.cache_cap_bytes == cap]
        ax.boxplot([d[d.tier == t].pct.dropna() for t in TIERS], tick_labels=TIERS)
        ax.set_title(f"disk cache cap {cap / 2**30:g} GiB", fontsize=10)
        ax.set_xlabel("tier that served the read")
    axes[0][0].set_ylabel("% of reads in the run")
    save(fig, "e5_tiers")

    fig, ax = plt.subplots(figsize=(7, 4))
    for cap in caps:
        d = df[(df.cache_cap_bytes == cap) & (df.tier == "remote")].sort_values("seq")
        ax.plot(d.seq, d.pct, marker=".", lw=0.8, label=f"cap {cap / 2**30:g} GiB")
    ax.set_xlabel("run number in the workload")
    ax.set_ylabel("% of reads served by a remote fetch")
    if len(caps) > 1:
        ax.legend(frameon=False)
    save(fig, "e5_remote_fraction")


def e6():
    df = read("e6_read_latency.csv")
    if df is None:
        return
    fetch = df[df.op == "lookup"]
    fig, ax = plt.subplots(figsize=(6.5, 4.5))
    v = ecdf(ax, fetch.latency_us / 1000, color="black")
    for q in (50, 90, 99):
        x = np.percentile(v, q)
        ax.axvline(x, color="grey", lw=0.6, ls=":")
        ax.annotate(f"p{q} {x:.2f} ms", (x, q / 100), textcoords="offset points", xytext=(4, -10), fontsize=8)
    ax.set_xscale("log")
    ax.set_xlabel("FUSE lookup latency incl. fetching the file, ms")
    ax.set_ylabel("cumulative fraction of file lookups")
    save(fig, "e6_read_latency")

    fig, ax = plt.subplots(figsize=(6.5, 4.5))
    for tier in TIERS:
        ecdf(ax, fetch[fetch.tier == tier].latency_us / 1000, label=tier)
    ax.set_xscale("log")
    ax.set_xlabel("FUSE lookup latency incl. fetching the file, ms")
    ax.set_ylabel("cumulative fraction of file lookups")
    ax.legend(frameon=False)
    save(fig, "e6_read_latency_by_tier")

    remote = fetch[(fetch.tier == "remote") & (fetch.size_bytes > 0)]
    if len(remote):
        fig, ax = plt.subplots(figsize=(6.5, 4.5))
        hb = ax.hexbin(remote.size_bytes, remote.latency_us / 1000, xscale="log", yscale="log", gridsize=40,
                       mincnt=1, bins="log", cmap="viridis")
        fig.colorbar(hb, ax=ax, label="fetches")
        ax.set_xlabel("file size, bytes")
        ax.set_ylabel("remote fetch latency, ms")
        save(fig, "e6_size_vs_latency")


def e7():
    df = read("e7_hedging.csv")
    if df is None:
        return
    df = df[df.phase == "main"]
    fig, ax = plt.subplots(figsize=(6.5, 4.5))
    for hedge, g in df.groupby("hedge"):
        label = "hedged" if hedge == "on" else "not hedged"
        if hedge == "on":
            label += f" (after {int(g.hedge_ms.iloc[0])} ms)"
        ecdf(ax, g.latency_us / 1000, label=label)
    ax.set_xscale("log")
    ax.set_xlabel("per-fetch latency, ms")
    ax.set_ylabel("cumulative fraction of remote fetches")
    ax.legend(frameon=False)
    save(fig, "e7_hedging")


def e8():
    df = read("e8_summary.csv")
    if df is None:
        return
    fig, ax = plt.subplots(figsize=(7, 4.5))
    for i, (app, g) in enumerate(df.groupby("app", sort=False)):
        color = f"C{i}"
        for cond, ls in (("cold", "-"), ("docker_pull_run", "--")):
            c = g[g.condition == cond].sort_values("added_rtt_ms")
            ax.plot(c.added_rtt_ms, c.median_ms, ls, color=color, marker="o", ms=3,
                    label=app if cond == "cold" else None)
    ax.plot([], [], "-", color="grey", label="tinycontainer cold")
    ax.plot([], [], "--", color="grey", label="docker pull + run")
    ax.set_yscale("log")
    ax.set_xlabel("added round-trip time between worker and store, ms")
    ax.set_ylabel("time to first output, ms (median)")
    ax.legend(frameon=False, fontsize=8, ncol=2)
    save(fig, "e8_rtt")


def e9():
    df = read("e9_hit_rates.csv")
    if df is None:
        return
    df = df[df.seq > 0]  # the first image of each order meets an empty cache by construction
    fig, ax = plt.subplots(figsize=(6, 4.5))
    ecdf(ax, df.local_bytes_pct, label="by bytes")
    ecdf(ax, df.local_files_pct, label="by files")
    ax.set_xlabel("% of a first-time image's fetches already cached on the worker")
    ax.set_ylabel("cumulative fraction of images")
    ax.set_xlim(0, 100)
    ax.legend(frameon=False)
    save(fig, "e9_first_run_hits")

    fig, ax = plt.subplots(figsize=(5, 4.5))
    ax.boxplot([df[f"{t}_bytes_pct"].dropna() for t in TIERS], tick_labels=TIERS)
    ax.set_xlabel("tier that served the fetch")
    ax.set_ylabel("% of bytes fetched by a first-time image")
    save(fig, "e9_first_run_tiers")


def e10():
    df = read("e10_summary.csv")
    if df is None or "speedup_off_over_on" not in df:
        return
    apps = list(dict.fromkeys(df.app))
    fig, ax = plt.subplots(figsize=(max(7, 0.8 * len(apps)), 4.5))
    x0 = np.arange(len(apps))
    bars = [("cold", "on"), ("cold", "off"), ("hot", "on"), ("hot", "off")]
    width = 0.8 / len(bars)
    for i, (state, pyc) in enumerate(bars):
        d = df[(df.state == state) & (df.pyc == pyc)].set_index("app").reindex(apps)
        ax.bar(x0 + (i - 1.5) * width, d.median_first_output_ms, width, label=f"{state}, .pyc {pyc}")
    ax.set_xticks(x0, apps, rotation=45 if len(apps) > 6 else 0, ha="right" if len(apps) > 6 else "center")
    ax.set_yscale("log")
    ax.set_ylabel("time to first output, ms (median)")
    ax.legend(frameon=False, ncol=2, fontsize=8)
    save(fig, "e10_pyc")


if __name__ == "__main__":
    for f in (e1, e2, e3, e4, e5, e6, e7, e8, e9, e10):
        f()
