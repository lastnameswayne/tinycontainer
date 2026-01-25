const ENDPOINT = "/stats";

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
const truncLines = (s, maxLines = 80) => {
    s = String(s ?? "");
    const lines = s.split("\n");
    return lines.length > maxLines ? lines.slice(0, maxLines).join("\n") + "\n…(truncated)" : s;
};

let rows = [];

function rowHTML(r) {
    const ok = Number(r.exit_code) === 0;
    const dot = ok ? "bg-emerald-500" : "bg-red-500";
    const pill = ok
        ? "border-emerald-200 bg-emerald-50 text-emerald-700"
        : "border-red-200 bg-red-50 text-red-700";

    const startedAbs = new Date(r.started_at).toLocaleString();
    const startedRel = rel(r.started_at);
    const username = r.username || r.filename.split('_')[0];

    return `
    <div class="run-card rounded-2xl border border-slate-200 bg-white p-4 hover:bg-slate-50">
      <div class="grid grid-cols-1 gap-3 lg:grid-cols-3 lg:gap-4">
        <div>
          <div class="flex items-center gap-2">
            <span class="h-2.5 w-2.5 rounded-full ${dot}"></span>
            <div class="font-medium">${esc(username)}</div>
            <span class="rounded border border-blue-200 bg-blue-50 px-1.5 py-0.5 text-xs text-blue-700 font-mono">.py</span>
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
          <span class="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs cursor-help" data-tooltip="Times fetched from memory">
            <span class="font-medium">${esc(r.memory_cache_hits)}</span> mem
          </span>
          <span class="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs cursor-help" data-tooltip="Times fetched from disk">
            <span class="font-medium">${esc(r.disk_cache_hits)}</span> disk
          </span>
          <span class="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs cursor-help" data-tooltip="Times fetched from server">
            <span class="font-medium">${esc(r.server_fetches)}</span> server
          </span>
        </div>

        <details class="lg:col-span-3 border-t border-slate-100 pt-3">
          <summary class="cursor-pointer text-xs text-slate-500 hover:text-slate-700 select-none flex items-center justify-between">
            <span>Logs</span>
            <span class="font-mono">${(r.stderr && r.stderr.length) ? "stderr present" : "no stderr"}</span>
          </summary>

          <div class="mt-3 grid gap-2 lg:grid-cols-2">
            <div class="overflow-hidden rounded-xl border border-slate-200 bg-slate-50">
              <div class="border-b border-slate-200 px-3 py-2 text-xs text-slate-500">stdout</div>
              <pre class="max-h-56 overflow-auto p-3 font-mono text-xs whitespace-pre">${esc(truncLines(r.stdout) || "(empty)")}</pre>
            </div>

            <div class="overflow-hidden rounded-xl border border-slate-200 bg-slate-50">
              <div class="border-b border-slate-200 px-3 py-2 text-xs text-slate-500">stderr</div>
              <pre class="max-h-56 overflow-auto p-3 font-mono text-xs whitespace-pre-wrap break-words text-red-700">${esc(truncLines(r.stderr) || "(empty)")}</pre>
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