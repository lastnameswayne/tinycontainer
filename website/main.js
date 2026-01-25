const ENDPOINT = "http://167.71.54.99:8444/stats";

const $ = (id) => document.getElementById(id);

const esc = (s) =>
    String(s ?? "").replace(/[&<>"']/g, (m) => ({
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#39;",
    }[m]));

const fmtDur = (ms) => (ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(2)} s`);
const rel = (iso) => dayjs(iso).fromNow();
const trunc = (s, n = 900) => {
    s = String(s ?? "");
    return s.length > n ? s.slice(0, n) + "\n…(truncated)" : s;
};

let rows = [];

function activityScore(r) {
    const mem = Number(r.memory_cache_hits ?? 0);
    const disk = Number(r.disk_cache_hits ?? 0);
    const fetch = Number(r.server_fetches ?? 0);

    const memN = Math.log1p(mem) / Math.log1p(1500);
    const diskN = Math.log1p(disk) / Math.log1p(1500);
    const fetchN = Math.log1p(fetch) / Math.log1p(60);

    return Math.max(0, Math.min(1, 0.55 * memN + 0.30 * diskN + 0.15 * fetchN));
}

function sparkBars(score01) {
    const n = 24;
    let out = "";
    for (let i = 0; i < n; i++) {
        const t = i / (n - 1);
        const h = Math.max(0, Math.min(1, score01 * 1.1 - t * 0.35));
        const height = Math.max(2, 22 * h);
        const cls = h <= 0.03 ? "bg-slate-200" : "bg-emerald-500";
        out += `<span class="inline-block w-2 rounded-sm ${cls}" style="height:${height}px"></span>`;
    }
    return out;
}

function rowHTML(r) {
    const ok = Number(r.exit_code) === 0;
    const dot = ok ? "bg-emerald-500" : "bg-red-500";
    const pill = ok
        ? "border-emerald-200 bg-emerald-50 text-emerald-700"
        : "border-red-200 bg-red-50 text-red-700";

    const startedAbs = new Date(r.started_at).toLocaleString();
    const startedRel = rel(r.started_at);

    const bars = sparkBars(activityScore(r));

    return `
    <div class="run-card rounded-2xl border border-slate-200 bg-white p-4 hover:bg-slate-50">
      <div class="grid grid-cols-1 gap-3 lg:grid-cols-3 lg:gap-4">
        <div>
          <div class="flex items-center gap-2">
            <span class="h-2.5 w-2.5 rounded-full ${dot}"></span>
            <div class="font-medium">${esc(r.filename)}</div>
            <div class="font-mono text-xs text-slate-500">#${esc(r.id)}</div>
          </div>
          <div class="mt-2 text-xs text-slate-500">
            ${esc(startedAbs)} · ${esc(startedRel)}
          </div>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <span class="rounded-full border px-2.5 py-1 text-xs ${pill}">
            ${ok ? "Succeeded" : "Failed"} · exit ${esc(r.exit_code)}
          </span>
          <span class="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs">
            ${esc(fmtDur(r.duration_ms))}
          </span>
          <span class="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs">
            <span class="font-medium">${esc(r.memory_cache_hits)}</span> mem
          </span>
          <span class="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs">
            <span class="font-medium">${esc(r.disk_cache_hits)}</span> disk
          </span>
          <span class="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs">
            <span class="font-medium">${esc(r.server_fetches)}</span> fetch
          </span>
        </div>

        <div class="flex flex-col items-end justify-between gap-2">
          <div class="text-xs text-slate-500">run · ${esc(startedRel)}</div>
          <div class="flex items-end gap-1">${bars}</div>
        </div>

        <details class="lg:col-span-3 border-t border-slate-100 pt-3">
          <summary class="cursor-pointer text-xs text-slate-500 hover:text-slate-700 select-none flex items-center justify-between">
            <span>Logs</span>
            <span class="font-mono">${(r.stderr && r.stderr.length) ? "stderr present" : "no stderr"}</span>
          </summary>

          <div class="mt-3 grid gap-2 lg:grid-cols-2">
            <div class="overflow-hidden rounded-xl border border-slate-200 bg-slate-50">
              <div class="border-b border-slate-200 px-3 py-2 text-xs text-slate-500">stdout</div>
              <pre class="max-h-56 overflow-auto p-3 font-mono text-xs whitespace-pre-wrap break-words">${esc(trunc(r.stdout) || "(empty)")}</pre>
            </div>

            <div class="overflow-hidden rounded-xl border border-slate-200 bg-slate-50">
              <div class="border-b border-slate-200 px-3 py-2 text-xs text-slate-500">stderr</div>
              <pre class="max-h-56 overflow-auto p-3 font-mono text-xs whitespace-pre-wrap break-words text-red-700">${esc(trunc(r.stderr) || "(empty)")}</pre>
            </div>
          </div>
        </details>
      </div>
    </div>
  `;
}

function render() {
    const q = $("q").value.trim().toLowerCase();
    const filtered = rows
        .filter((r) => `${r.id} ${r.filename} ${r.exit_code}`.toLowerCase().includes(q))
        .sort((a, b) => new Date(b.started_at) - new Date(a.started_at));

    $("count").textContent = `${filtered.length} run${filtered.length === 1 ? "" : "s"}`;

    $("list").innerHTML = filtered.length
        ? filtered.map(rowHTML).join("")
        : `<div class="py-10 text-center text-sm text-slate-500">No runs match your search.</div>`;
}

async function load() {
    $("status").textContent = "Loading…";
    try {
        const res = await fetch(ENDPOINT, {
            method: "POST",
            headers: { "content-type": "application/json" },
            body: "[]",
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        rows = await res.json();
        $("status").textContent = "Updated just now";
        render();
    } catch (e) {
        rows = [];
        $("status").textContent = `Failed: ${e.message}`;
        $("list").innerHTML = `
      <div class="py-10 text-center text-sm text-slate-500">
        Couldn’t reach <span class="font-mono">${esc(ENDPOINT)}</span>.<br/>
      </div>`;
        $("count").textContent = "—";
    }
}

// events
$("q").addEventListener("input", render);
$("refresh").addEventListener("click", load);

// start
load();