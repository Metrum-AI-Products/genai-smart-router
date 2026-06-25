const charts = {};
let activeTab = "groups";

const fmt = new Intl.NumberFormat("en-US", { maximumFractionDigits: 2 });
const usd = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 6 });

function qs() {
  const data = new FormData(document.querySelector("#filters"));
  const params = new URLSearchParams();
  for (const [key, value] of data.entries()) {
    if (String(value).trim()) params.set(key, String(value).trim());
  }
  return params;
}

async function load() {
  const params = qs();
  document.querySelector("#export").href = `export.md?${params}`;
  const res = await fetch(`api/summary?${params}`, { credentials: "same-origin" });
  if (!res.ok) throw new Error(`report request failed: ${res.status}`);
  const report = await res.json();
  render(report);
}

function render(report) {
  document.querySelector("#period").textContent = `${report.period.from} to ${report.period.to}`;
  const s = report.summary;
  document.querySelector("#summary").innerHTML = [
    ["Requests", fmt.format(s.requests)],
    ["Errors", fmt.format(s.errors)],
    ["Tokens", fmt.format(s.tokens)],
    ["Cost", usd.format(s.costUsd)],
    ["Avg latency", `${fmt.format(s.avgLatencyMs)} ms`],
    ["Fallbacks", fmt.format(s.fallbacks)]
  ].map(([label, value]) => `<div class="metric"><strong>${value}</strong><span>${label}</span></div>`).join("");
  chart("requestsChart", report.series.map(x => x.timeUtc), [{ label: "Requests", data: report.series.map(x => x.requests), borderColor: "#1f6feb" }]);
  chart("costChart", report.series.map(x => x.timeUtc), [{ label: "Cost", data: report.series.map(x => x.costUsd), borderColor: "#d97706" }]);
  chart("latencyChart", report.series.map(x => x.timeUtc), [{ label: "Latency", data: report.series.map(x => x.latencyMs), borderColor: "#7c3aed" }, { label: "TTFB", data: report.series.map(x => x.ttfbMs), borderColor: "#059669" }]);
  chart("cacheChart", report.series.map(x => x.timeUtc), [{ label: "Hits", data: report.series.map(x => x.cacheHits), borderColor: "#059669" }, { label: "Misses", data: report.series.map(x => x.cacheMisses), borderColor: "#d97706" }, { label: "Bypass", data: report.series.map(x => x.cacheBypass), borderColor: "#6b7280" }]);
  chart("providerChart", report.byProvider.map(x => x.key), [{ label: "Tokens", data: report.byProvider.map(x => x.tokens), borderColor: "#0f766e" }]);
  chart("errorChart", report.series.map(x => x.timeUtc), [{ label: "Errors", data: report.series.map(x => x.errors), borderColor: "#dc2626" }, { label: "Fallbacks", data: report.series.map(x => x.fallbacks), borderColor: "#d97706" }]);
  renderTable(report);
}

function chart(id, labels, datasets) {
  if (charts[id]) charts[id].destroy();
  charts[id] = new Chart(document.getElementById(id), { type: "line", data: { labels, datasets } });
}

function renderTable(report) {
  const source = activeTab === "providers" ? report.byProvider : activeTab === "tokens" ? report.byToken : activeTab === "requests" ? report.requests : report.byGroup;
  const rows = activeTab === "requests" ? requestRows(source) : aggregateRows(source);
  document.querySelector("#tables").innerHTML = `<div class="tablewrap"><table>${rows}</table></div>`;
}

function aggregateRows(rows) {
  return `<thead><tr><th>Key</th><th>Requests</th><th>Errors</th><th>Tokens</th><th>Cost</th><th>Attempts</th><th>Fallbacks</th><th>Avg latency</th></tr></thead><tbody>` +
    rows.map(r => `<tr><td>${esc(r.key)}</td><td>${r.requests}</td><td>${r.errors}</td><td>${r.tokens}</td><td>${usd.format(r.costUsd)}</td><td>${r.attempts}</td><td>${r.fallbacks}</td><td>${r.avgLatencyMs} ms</td></tr>`).join("") +
    `</tbody>`;
}

function requestRows(rows) {
  return `<thead><tr><th>Time</th><th>Request</th><th>User</th><th>Project</th><th>Group</th><th>Provider</th><th>Model</th><th>Status</th><th>Tokens</th><th>Cost</th></tr></thead><tbody>` +
    rows.slice(-100).reverse().map(r => `<tr><td>${esc(r.timeUtc)}</td><td>${esc(r.requestId)}</td><td>${esc(r.callerUser)}</td><td>${esc(r.project)}</td><td>${esc(r.modelGroup)}</td><td>${esc(r.provider)}</td><td>${esc(r.model)}</td><td>${r.status}</td><td>${r.tokens}</td><td>${usd.format(r.costUsd)}</td></tr>`).join("") +
    `</tbody>`;
}

function esc(value) {
  return String(value ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

document.querySelector("#filters").addEventListener("submit", event => {
  event.preventDefault();
  load().catch(err => document.querySelector("#tables").innerHTML = `<div class="error">${esc(err.message)}</div>`);
});
document.querySelectorAll(".tabs button").forEach(button => button.addEventListener("click", () => {
  document.querySelectorAll(".tabs button").forEach(b => b.classList.remove("active"));
  button.classList.add("active");
  activeTab = button.dataset.tab;
  load().catch(err => document.querySelector("#tables").innerHTML = `<div class="error">${esc(err.message)}</div>`);
}));
load().catch(err => document.querySelector("#tables").innerHTML = `<div class="error">${esc(err.message)}</div>`);
