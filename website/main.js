(async () => {
    const tbody = document.getElementById("runs-body");
    if (!tbody) return;

    const esc = (s) =>
        String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

    const fmtTime = (iso) => {
        const d = new Date(iso);
        return isNaN(d) ? "—" : d.toISOString().replace("T", " ").replace(/\.\d{3}Z$/, "Z");
    };

    try {
        const res = await fetch("/stats", { headers: { "Accept": "application/json" } });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const runs = await res.json();
        if (!Array.isArray(runs)) throw new Error("expected array");

        runs.sort((a, b) => new Date(b.started_at) - new Date(a.started_at));

        const rows = runs.map((r) => `
        <tr>
          <td>${esc(r.id)}</td>
          <td class="mono">${esc(fmtTime(r.started_at))}</td>
          <td>${esc(r.filename ?? "")}</td>
          <td class="mono">${esc(r.duration_ms ?? "—")}</td>
          <td class="mono">${esc(r.exit_code ?? "—")}</td>
          <td class="mono">${esc(r.memory_cache_hits ?? "—")}</td>
          <td class="mono">${esc(r.disk_cache_hits ?? "—")}</td>
          <td class="mono">${esc(r.server_fetches ?? "—")}</td>
        </tr>
      `).join("");

        tbody.innerHTML = rows || `<tr><td colspan="8">No runs</td></tr>`;
    } catch (e) {
        tbody.innerHTML = `<tr><td colspan="8">Failed to load runs</td></tr>`;
    }
})();