#!/usr/bin/env python3
"""Derives the per-experiment datasets from the worker's runs.db, the image manifests
and the raw run logs. Safe to re-run; every output is rewritten.

  image_files.csv      manifest of each test-app image (image, path, size_bytes, sha1)
  e1_tier_check.csv    per app and condition: share of file fetches served by each tier
  e1_bytes.csv         bytes moved per app: docker (compressed image) vs tinycontainer cold
  e2_workingset.csv    working set vs image size, per app (E1 cold runs)
  e2_scatter.csv       touched bytes vs cold first-output time, with the docker reference
  e3_timeline_<app>.csv cumulative bytes fetched over a representative cold run
  e3_order_stability.csv Jaccard similarity of the first 200 fetched paths, KEY_APP cold runs
  e5_tiers.csv         per-run share of reads (count and bytes) by the tier that loaded the file
  e5_lookup_tiers.csv  per-run share of FUSE file lookups by tier
  e6_read_latency.csv  per-event latency for file fetches (lookup) and reads, E5 runs
  e6_negative.csv      negative lookups: count and latency, cached vs asked the fileserver
  e7_summary.csv       hedging on/off: per-fetch p50/p90/p99, extra requests/bytes, cold start
  e8_summary.csv       median/p90 per app, condition and added RTT; e8_crossover.csv
  e9_hit_rates.csv     per first-time image: share of file fetches served locally vs remote
  e10_summary.csv      .pyc on vs off: median first output per app and cache state, speedup
"""
import itertools
import os
import sqlite3
import sys
from pathlib import Path

import numpy as np
import pandas as pd

REPO = Path(os.environ.get("REPO", Path(__file__).resolve().parents[1]))
RESULTS = Path(os.environ.get("RESULTS", REPO / "results"))
WORKER_DIR = Path(os.environ.get("WORKER_DIR", "/opt/tc-exp/worker"))
APPS = os.environ.get("APPS", "testapp2 testapp3 testappscipy testappplotting ml-inference").split()
# Apps whose real work happens after their first line of output (reader prints right after
# importing os, then reads the whole image): their headline time is exit, not first output.
EXIT_METRIC_APPS = {"reader"}


def primary_ms(df):
    """Time to first output, or time to exit for EXIT_METRIC_APPS."""
    return df.t_exit_ms.where(df.app.isin(EXIT_METRIC_APPS), df.t_first_output_ms)


def db():
    return sqlite3.connect(WORKER_DIR / "runs.db")


def events(run_ids, where=""):
    ids = [int(i) for i in run_ids if pd.notna(i)]
    if not ids:
        return pd.DataFrame(columns=["run_id", "ts_us", "op", "path", "size_bytes", "tier", "latency_us", "hit", "hedge"])
    q = f"SELECT * FROM file_events WHERE run_id IN ({','.join(map(str, ids))}) {where}"
    with db() as con:
        return pd.read_sql_query(q, con)


def runs_table(run_ids):
    ids = [int(i) for i in run_ids if pd.notna(i)]
    with db() as con:
        return pd.read_sql_query(f"SELECT * FROM runs WHERE id IN ({','.join(map(str, ids))})", con)


def to_image_path(key):
    """Fileserver key ('app/usr/lib/x') -> path inside the image ('/usr/lib/x')."""
    return "/" + (key[4:] if key.startswith("app/") else key)


def pct(a, q):
    return float(np.percentile(a, q)) if len(a) else float("nan")


def write(df, name):
    df.to_csv(RESULTS / name, index=False)
    print(f"wrote {name} ({len(df)} rows)")


def manifests():
    out = {}
    for app in APPS:
        p = RESULTS / "raw" / "manifests" / f"app-{app}.csv"
        if p.exists():
            out[app] = pd.read_csv(p, keep_default_na=False)
    return out


def image_files(mans):
    frames = []
    for app, m in mans.items():
        regular = m[m.link_target == ""]
        frames.append(pd.DataFrame({"image": app, "path": regular.path,
                                    "size_bytes": regular.size_bytes, "sha1": regular.sha1}))
    if frames:
        write(pd.concat(frames), "image_files.csv")


def e2(mans, e1):
    rows, scatter = [], []
    for app in APPS:
        cold = e1[(e1.app == app) & (e1.condition == "cold") & (e1.exit_code == 0)]
        if cold.empty or app not in mans:
            continue
        m = mans[app]
        canon = dict(zip(m.path, [t or p for p, t in zip(m.path, m.link_target)]))
        size = dict(zip(m.path, m.size_bytes))
        regular = m[m.link_target == ""]
        ev = events(cold.run_id)
        per_run = []
        for run_id, g in ev.groupby("run_id"):
            reads = {canon.get(to_image_path(p)) for p in g[g.op == "read"].path.unique()} - {None}
            remote = g[(g.op == "lookup") & (g.tier == "remote")]
            fetched_paths = {canon.get(to_image_path(p)) for p in remote[remote.size_bytes > 0].path} - {None}
            per_run.append({
                "touched_files": len(reads),
                "touched_bytes": sum(size[p] for p in reads),
                "fetched_files": len(fetched_paths),
                "fetched_bytes": int(remote.size_bytes.sum()),
                "negative_lookups": int((g.tier == "negative").sum()),
                "remote_dir_listings": int(((g.op == "readdir") & (g.tier == "remote")).sum()),
            })
        pr = pd.DataFrame(per_run)
        image_bytes = int(regular.size_bytes.sum())
        row = {"app": app, "image_bytes": image_bytes, "image_files": len(regular), "cold_runs": len(pr)}
        for c in pr.columns:
            row[c] = int(pr[c].median())
        row["pct_touched"] = round(100 * row["touched_bytes"] / image_bytes, 3)
        row["pct_fetched"] = round(100 * row["fetched_bytes"] / image_bytes, 3)
        rows.append(row)

        docker = e1[(e1.app == app) & (e1.condition == "docker_pull_run") & (e1.exit_code == 0)]
        scatter.append({"app": app, "touched_bytes": row["touched_bytes"], "fetched_bytes": row["fetched_bytes"],
                        "metric": "exit" if app in EXIT_METRIC_APPS else "first_output",
                        "cold_t_first_output_ms_median": primary_ms(cold).median(),
                        "cold_t_first_output_ms_p90": pct(primary_ms(cold).dropna(), 90),
                        "docker_pull_ms_median": docker.pull_ms.median(),
                        "docker_pull_plus_run_ms": primary_ms(docker).median()})
    if rows:
        write(pd.DataFrame(rows), "e2_workingset.csv")
        write(pd.DataFrame(scatter), "e2_scatter.csv")


def tier_shares(ev):
    """Share of file fetches (lookups that returned content) by tier, by count and bytes."""
    files = ev[(ev.op == "lookup") & ev.tier.isin(["mem", "disk", "remote"]) & (ev.size_bytes > 0)]
    out = {}
    n, b = len(files), files.size_bytes.sum()
    for tier in ("mem", "disk", "remote"):
        sel = files[files.tier == tier]
        out[f"{tier}_files_pct"] = round(100 * len(sel) / n, 2) if n else np.nan
        out[f"{tier}_bytes_pct"] = round(100 * sel.size_bytes.sum() / b, 2) if b else np.nan
    out["file_fetches"] = n
    out["fetched_bytes"] = int(b)
    return out


def e1_checks(mans, e1):
    sway = e1[e1.condition.isin(["cold", "warm", "hot"]) & (e1.exit_code == 0)]
    rows = []
    for (app, cond), g in sway.groupby(["app", "condition"]):
        ev = events(g.run_id)
        per_run = pd.DataFrame([tier_shares(e) for _, e in ev.groupby("run_id")])
        if per_run.empty:
            continue
        row = {"app": app, "condition": cond, "runs": len(per_run)}
        row.update(per_run.median(numeric_only=True).round(2).to_dict())
        rows.append(row)
    if rows:
        write(pd.DataFrame(rows), "e1_tier_check.csv")

    exports = RESULTS / "raw" / "exports.csv"
    ex = pd.read_csv(exports) if exports.exists() else pd.DataFrame(columns=["app", "registry_compressed_bytes"])
    rows = []
    for app in APPS:
        cold = e1[(e1.app == app) & (e1.condition == "cold") & (e1.exit_code == 0)]
        if cold.empty:
            continue
        reg = ex[ex.app == app].registry_compressed_bytes.dropna()
        image = mans[app][mans[app].link_target == ""].size_bytes.sum() if app in mans else np.nan
        rows.append({"app": app,
                     "docker_pulled_bytes": int(reg.iloc[-1]) if len(reg) else np.nan,
                     "image_uncompressed_bytes": image,
                     "tc_cold_file_bytes": cold.fs_file_value_bytes.median(),
                     "tc_cold_wire_bytes": cold.fs_response_bytes.median(),
                     "tc_cold_files_fetched": cold.fs_file_fetches.median()})
    if rows:
        write(pd.DataFrame(rows), "e1_bytes.csv")


def e8():
    p = RESULTS / "e8_rtt.csv"
    if not p.exists():
        return
    df = pd.read_csv(p)
    df = df[df.exit_code == 0].assign(t_ms=lambda d: primary_ms(d))
    g = df.groupby(["app", "condition", "added_rtt_ms"]).t_ms
    summ = pd.DataFrame({"runs": g.size(), "median_ms": g.median(), "p90_ms": g.quantile(0.9)}).reset_index()
    write(summ, "e8_summary.csv")
    rows = []
    for app, a in summ.groupby("app"):
        piv = a.pivot(index="added_rtt_ms", columns="condition", values="median_ms").sort_index()
        if {"cold", "docker_pull_run"} - set(piv.columns):
            continue
        docker_wins = piv[piv.docker_pull_run <= piv.cold]
        rows.append({"app": app, "first_added_rtt_where_docker_wins_ms":
                     docker_wins.index.min() if len(docker_wins) else None,
                     "max_added_rtt_tested_ms": piv.index.max()})
    if rows:
        write(pd.DataFrame(rows), "e8_crossover.csv")


def e9():
    p = RESULTS / "raw" / "e9_runs.csv"
    if not p.exists():
        return
    runs = pd.read_csv(p)
    runs = runs[(runs.exit_code == 0) & runs.run_id.notna()]
    ev = events(runs.run_id)
    rows = []
    meta = runs.set_index("run_id")
    for run_id, g in ev.groupby("run_id"):
        r = {"run_id": run_id, "seed": meta.loc[run_id, "seed"], "seq": meta.loc[run_id, "seq"],
             "image": meta.loc[run_id, "image"]}
        r.update(tier_shares(g))
        r["local_files_pct"] = r["mem_files_pct"] + r["disk_files_pct"]
        r["local_bytes_pct"] = r["mem_bytes_pct"] + r["disk_bytes_pct"]
        rows.append(r)
    if rows:
        write(pd.DataFrame(rows).sort_values(["seed", "seq"]), "e9_hit_rates.csv")


def e1_breakdown(e1):
    """Where a cold start's time goes, per app (median over its E1 cold runs). Python imports
    run on one thread, so a run's fetches happen one after another and their latencies add up
    to time the program spends waiting:
      fetch_ms    remote lookups that returned a file or directory
      notfound_ms remote lookups that came back 404
      listing_ms  remote directory listings (readdir)
      local_ms    lookups served from the worker's memory or disk
      other_ms    everything else up to first output: runc start, Python running, the kernel
    """
    cold = e1[(e1.condition == "cold") & (e1.exit_code == 0)]
    ev = events(cold.run_id)
    first = runs_table(cold.run_id).set_index("id").first_output_ms
    rows = []
    for run_id, g in ev.groupby("run_id"):
        lk = g[g.op == "lookup"]
        part = {
            "fetch_ms": lk[lk.tier == "remote"].latency_us.sum() / 1000,
            "notfound_ms": lk[(lk.tier == "negative") & (lk.hit == 0)].latency_us.sum() / 1000,
            "listing_ms": g[g.op == "readdir"].latency_us.sum() / 1000,
            "local_ms": lk[lk.tier.isin(["mem", "disk"])].latency_us.sum() / 1000,
        }
        total = float(first.get(run_id, np.nan))
        part["other_ms"] = max(total - sum(part.values()), 0)
        rows.append(dict(part, run_id=run_id, app=cold.set_index("run_id").app.get(run_id), first_output_ms=total))
    df = pd.DataFrame(rows)
    out = df.groupby("app")[["fetch_ms", "notfound_ms", "listing_ms", "local_ms", "other_ms", "first_output_ms"]].median().round(1)
    write(out.reset_index(), "e1_time_breakdown.csv")


def e10():
    p = RESULTS / "e10_pyc.csv"
    if not p.exists():
        return
    df = pd.read_csv(p)
    df = df[df.exit_code == 0]
    g = df.groupby(["app", "state", "pyc"])
    summ = pd.DataFrame({"runs": g.size(), "median_first_output_ms": g.t_first_output_ms.median(),
                         "median_files_fetched": g.fs_file_fetches.median(),
                         "median_bytes_fetched": g.fs_file_value_bytes.median()}).reset_index()
    wide = summ.pivot_table(index=["app", "state"], columns="pyc", values="median_first_output_ms").reset_index()
    if {"on", "off"} <= set(wide.columns):
        wide["speedup_off_over_on"] = (wide["off"] / wide["on"]).round(3)
        summ = summ.merge(wide[["app", "state", "speedup_off_over_on"]], on=["app", "state"])
    write(summ, "e10_summary.csv")


def e3(e1):
    for app in APPS:
        cold = e1[(e1.app == app) & (e1.condition == "cold") & (e1.exit_code == 0)].dropna(subset=["t_exit_ms"])
        if cold.empty:
            continue
        # the run with the median t_exit_ms (lower median for an even count)
        rep = cold.sort_values("t_exit_ms").iloc[(len(cold) - 1) // 2]
        run = runs_table([rep.run_id]).iloc[0]
        ev = events([rep.run_id], "AND op = 'lookup' AND tier = 'remote'").sort_values("ts_us")
        t0 = run.handler_start_us
        out = pd.DataFrame({
            "ts_ms": ((ev.ts_us + ev.latency_us) - t0) / 1000,  # when each fetch completed
            "cumulative_bytes": ev.size_bytes.cumsum(),
            "cumulative_files": np.arange(1, len(ev) + 1),
        })
        out["run_id"] = int(rep.run_id)
        out["worker_first_output_ms"] = run.first_output_ms
        out["worker_exit_ms"] = run.exit_ms
        write(out, f"e3_timeline_{app}.csv")

    key = os.environ.get("KEY_APP", "testappscipy")
    cold = e1[(e1.app == key) & (e1.condition == "cold") & (e1.exit_code == 0)]
    if len(cold) >= 2:
        ev = events(cold.run_id, "AND op = 'lookup' AND tier = 'remote'").sort_values(["run_id", "ts_us"])
        firsts = {r: set(g.path.head(200)) for r, g in ev.groupby("run_id")}
        pairs = [len(a & b) / len(a | b) for a, b in itertools.combinations(firsts.values(), 2)]
        write(pd.DataFrame([{"app": key, "runs": len(firsts), "pairs": len(pairs),
                             "jaccard_mean": np.mean(pairs), "jaccard_min": np.min(pairs),
                             "jaccard_median": np.median(pairs)}]), "e3_order_stability.csv")


def e5_e6(e5):
    ok = e5[e5.run_id.notna()]
    if ok.empty:
        return
    tiers, lookups, lat = [], [], []
    for cap, runs in ok.groupby("cache_cap_bytes"):
        ev = events(runs.run_id)
        meta = runs.set_index("run_id")
        for run_id, g in ev.groupby("run_id"):
            app = meta.loc[run_id, "app"]
            seq = meta.loc[run_id, "seq"]
            files = g[(g.op == "lookup") & (g.tier.isin(["mem", "disk", "remote"])) & (g.size_bytes > 0)]
            # tier that loaded each file in this run; files with no lookup this run were
            # already in worker memory (kernel still held the inode)
            loaded_by = files.sort_values("ts_us").drop_duplicates("path").set_index("path").tier
            reads = g[g.op == "read"]
            read_tier = reads.path.map(loaded_by).fillna("mem")
            for tier in ("mem", "disk", "remote"):
                sel = reads[read_tier == tier]
                tiers.append({"run_id": run_id, "seq": seq, "app": app, "cache_cap_bytes": cap, "tier": tier,
                              "reads": len(sel), "bytes": int(sel.size_bytes.sum())})
            all_lookups = g[(g.op == "lookup") & g.tier.isin(["mem", "disk", "remote"])]
            for tier in ("mem", "disk", "remote"):
                sel = all_lookups[all_lookups.tier == tier]
                lookups.append({"run_id": run_id, "seq": seq, "app": app, "cache_cap_bytes": cap, "tier": tier,
                                "lookups": len(sel), "bytes": int(sel.size_bytes.sum())})
            for op in ("lookup", "read"):
                sel = files if op == "lookup" else reads.assign(tier=read_tier)
                lat.append(pd.DataFrame({"latency_us": sel.latency_us, "tier": sel.tier, "size_bytes": sel.size_bytes,
                                         "app": app, "op": op, "cache_cap_bytes": cap, "run_id": run_id}))
    write(pd.DataFrame(tiers), "e5_tiers.csv")
    write(pd.DataFrame(lookups), "e5_lookup_tiers.csv")
    write(pd.concat(lat), "e6_read_latency.csv")

    neg = events(ok.run_id, "AND tier = 'negative'")
    rows = []
    for label, sel in [("all", neg), ("cached (no fileserver request)", neg[neg.hit == 1]),
                       ("fileserver 404", neg[neg.hit == 0])]:
        rows.append({"negative_lookups": label, "count": len(sel),
                     "per_run_median": float(sel.groupby("run_id").size().median()) if len(sel) else 0,
                     "latency_us_p50": pct(sel.latency_us, 50), "latency_us_p90": pct(sel.latency_us, 90),
                     "latency_us_p99": pct(sel.latency_us, 99)})
    write(pd.DataFrame(rows), "e6_negative.csv")


def e7():
    p = RESULTS / "e7_hedging.csv"
    if not p.exists():
        return
    f = pd.read_csv(p)
    f = f[f.phase == "main"]
    runs = pd.read_csv(RESULTS / "raw" / "e7_runs.csv")
    runs = runs[runs.phase == "main"]
    base = runs[runs.hedge == "off"]
    rows = []
    for hedge, g in f.groupby("hedge"):
        r = runs[runs.hedge == hedge]
        rows.append({
            "hedge": hedge, "hedge_ms": int(g.hedge_ms.iloc[0]), "runs": len(r), "fetches": len(g),
            "p50_us": pct(g.latency_us, 50), "p90_us": pct(g.latency_us, 90),
            "p99_us": pct(g.latency_us, 99), "p999_us": pct(g.latency_us, 99.9),
            "hedges_fired_pct": round(100 * (g.hedge_state > 0).mean(), 3),
            "hedge_won_pct": round(100 * (g.hedge_state == 2).mean(), 3),
            "extra_requests_pct": round(100 * (r.fs_file_fetches.median() / base.fs_file_fetches.median() - 1), 3),
            "extra_bytes_pct": round(100 * (r.fs_response_bytes.median() / base.fs_response_bytes.median() - 1), 3),
            "median_t_first_output_ms": r.t_first_output_ms.median(),
            "p90_t_first_output_ms": pct(r.t_first_output_ms.dropna(), 90),
            "median_t_exit_ms": r.t_exit_ms.median(),
        })
    out = pd.DataFrame(rows)
    off = out[out.hedge == "off"].median_t_first_output_ms.iloc[0]
    out["t_first_output_delta_ms"] = out.median_t_first_output_ms - off
    write(out, "e7_summary.csv")


def main():
    RESULTS.mkdir(exist_ok=True)
    mans = manifests()
    image_files(mans)
    e1p = RESULTS / "raw" / "e1_runs.csv"
    if e1p.exists():
        e1 = pd.read_csv(e1p)
        sway = e1[~e1.run_id.astype(str).str.startswith("docker")].copy()
        sway["run_id"] = pd.to_numeric(sway.run_id)
        e1 = pd.concat([sway, e1[e1.run_id.astype(str).str.startswith("docker")]])
        e1_checks(mans, sway)
        e1_breakdown(sway)
        e2(mans, e1)
        e3(sway)
    e5p = RESULTS / "raw" / "e5_runs.csv"
    if e5p.exists():
        e5_e6(pd.read_csv(e5p))
    e7()
    e8()
    e9()
    e10()


if __name__ == "__main__":
    sys.exit(main())
