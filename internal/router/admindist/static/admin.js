const charts = {};
let activeTab = "groups";
let lastReport = null;
let lastSavings = null;

const themeKey = "metrum-admin-reports-theme";
const fmt = new Intl.NumberFormat("en-US", { maximumFractionDigits: 2 });
const compact = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 2 });
const usd = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 6 });
const usdCompact = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", notation: "compact", maximumFractionDigits: 2 });

function storedTheme() {
  try {
    const value = window.localStorage.getItem(themeKey);
    return value === "light" || value === "dark" ? value : "";
  } catch {
    return "";
  }
}

function systemTheme() {
  return window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function currentTheme() {
  return document.documentElement.dataset.theme || storedTheme() || systemTheme();
}

function setStoredTheme(theme) {
  try {
    window.localStorage.setItem(themeKey, theme);
  } catch {
    // Theme persistence is best effort; the selected theme still applies for this page.
  }
}

function applyTheme(theme, persist) {
  document.documentElement.dataset.theme = theme;
  if (persist) setStoredTheme(theme);
  const toggle = document.querySelector("#themeToggle");
  if (toggle) {
    const isDark = theme === "dark";
    toggle.textContent = isDark ? "Light" : "Dark";
    toggle.setAttribute("aria-pressed", String(isDark));
    toggle.setAttribute("aria-label", isDark ? "Switch to light theme" : "Switch to dark theme");
  }
}

function cssVar(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

function palette() {
  return {
    red: cssVar("--metrum-red"),
    pink: cssVar("--metrum-pink"),
    magenta: cssVar("--metrum-magenta"),
    purple: cssVar("--metrum-purple"),
    violet: cssVar("--metrum-violet"),
    blue: cssVar("--metrum-blue"),
    success: cssVar("--success"),
    warning: cssVar("--warning"),
    grid: cssVar("--chart-grid"),
    text: cssVar("--chart-text")
  };
}

function colorFor(key) {
  const c = palette();
  return c[key] || c.magenta;
}

function formatUnit(value, unit, exact) {
  const n = Number(value) || 0;
  if (unit === "usd") return exact ? usd.format(n) : usdCompact.format(n);
  if (unit === "tokens" || unit === "count") return exact ? fmt.format(n) : compact.format(n);
  if (unit === "ms") return n >= 1000 ? `${fmt.format(n / 1000)} s` : `${fmt.format(n)} ms`;
  if (unit === "seconds") return `${fmt.format(n)} s`;
  if (unit === "tok/s") return `${fmt.format(n)} tok/s`;
  if (unit === "percent") return `${fmt.format(n)}%`;
  return fmt.format(n);
}

function qs() {
  const data = new FormData(document.querySelector("#filters"));
  const params = new URLSearchParams();
  for (const [key, value] of data.entries()) {
    if (String(value).trim()) params.set(key, String(value).trim());
  }
  return params;
}

function savingsQS() {
  const params = qs();
  const data = new FormData(document.querySelector("#savingsFilters"));
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

async function loadSavings() {
  const params = savingsQS();
  const res = await fetch(`api/savings?${params}`, { credentials: "same-origin" });
  if (!res.ok) throw new Error(`savings request failed: ${res.status}`);
  const report = await res.json();
  renderSavings(report);
}

function render(report) {
  lastReport = report;
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
  renderCharts(report);
  renderTable(report);
}

function renderCharts(report) {
  if (Array.isArray(report.charts) && report.charts.length) {
    renderChartSpecs(report.charts);
    return;
  }
  const c = palette();
  chart("requestsChart", report.series.map(x => x.timeUtc), [{ label: "Requests", data: report.series.map(x => x.requests), borderColor: c.magenta }], c);
  chart("costChart", report.series.map(x => x.timeUtc), [{ label: "Cost", data: report.series.map(x => x.costUsd), borderColor: c.red }], c);
  chart("latencyChart", report.series.map(x => x.timeUtc), [{ label: "Latency", data: report.series.map(x => x.latencyMs), borderColor: c.violet }, { label: "TTFB", data: report.series.map(x => x.ttfbMs), borderColor: c.blue }], c);
  chart("cacheChart", report.series.map(x => x.timeUtc), [{ label: "Hits", data: report.series.map(x => x.cacheHits), borderColor: c.success }, { label: "Misses", data: report.series.map(x => x.cacheMisses), borderColor: c.warning }, { label: "Bypass", data: report.series.map(x => x.cacheBypass), borderColor: c.text }], c);
  chart("providerChart", report.byProvider.map(x => x.key), [{ label: "Tokens", data: report.byProvider.map(x => x.tokens), borderColor: c.purple }], c);
  chart("errorChart", report.series.map(x => x.timeUtc), [{ label: "Errors", data: report.series.map(x => x.errors), borderColor: c.red }, { label: "Fallbacks", data: report.series.map(x => x.fallbacks), borderColor: c.warning }], c);
}

function renderChartSpecs(specs) {
  const byID = Object.fromEntries(specs.map(spec => [spec.chart_id, spec]));
  chartFromSpec("requestsChart", byID.requests);
  chartFromSpec("costChart", byID.cost);
  chartFromSpec("latencyChart", byID.latency);
  chartFromSpec("cacheChart", byID.cache);
  chartFromSpec("providerChart", byID.provider_tokens);
  chartFromSpec("errorChart", byID.errors_fallbacks);
}

function renderSavingsChartSpecs(specs) {
  const byID = Object.fromEntries(specs.map(spec => [spec.chart_id, spec]));
  chartFromSpec("savingsCostChart", byID.savings_cost);
  chartFromSpec("savingsUsdChart", byID.savings_usd);
  chartFromSpec("savingsPctChart", byID.savings_pct);
}

function chartFromSpec(id, spec) {
  if (!spec) return;
  const canvas = document.getElementById(id);
  const figure = canvas.closest("figure");
  const caption = figure && figure.querySelector("figcaption");
  if (caption) caption.textContent = spec.title;
  const labels = (((spec.series || [])[0] || {}).points || []).map(point => point.x);
  const datasets = (spec.series || []).map(item => ({
    label: item.name,
    data: (item.points || []).map(point => point.y),
    borderColor: colorFor(item.color_key),
    backgroundColor: colorFor(item.color_key),
    unit: item.unit,
    colorKey: item.color_key
  }));
  chart(id, labels, datasets, palette(), spec);
}

function chart(id, labels, datasets, colors, spec) {
  if (charts[id]) charts[id].destroy();
  charts[id] = new Chart(document.getElementById(id), {
    type: "line",
    data: { labels, datasets },
    options: {
      gridColor: colors.grid,
      textColor: colors.text,
      xAxis: spec ? spec.x_axis : { label: "Time", unit: "UTC hour" },
      yAxis: spec ? spec.y_axis : { label: "", unit: "" },
      formatValue: formatUnit
    }
  });
}

function renderTable(report) {
  const savingsPanel = document.querySelector("#savingsPanel");
  savingsPanel.hidden = activeTab !== "savings";
  if (activeTab === "savings") {
    document.querySelector("#tables").innerHTML = "";
    loadSavings().catch(err => document.querySelector("#tables").innerHTML = `<div class="error">${esc(err.message)}</div>`);
    return;
  }
  const source = activeTab === "providers" ? report.byProvider : activeTab === "tokens" ? report.byToken : activeTab === "requests" ? report.requests : report.byGroup;
  const rows = activeTab === "requests" ? requestRows(source) : aggregateRows(source);
  document.querySelector("#tables").innerHTML = `<div class="tablewrap"><table>${rows}</table></div>`;
}

function renderSavings(report) {
  lastSavings = report;
  populateBaselines(report.baselines || [], report.baseline && report.baseline.baseline_id);
  const s = report.summary || {};
  document.querySelector("#savingsSummary").innerHTML = [
    ["Actual cost", usd.format(s.actual_cost_usd || 0)],
    ["Baseline cost", usd.format(s.baseline_cost_usd || 0)],
    ["Savings", usd.format(s.savings_usd || 0)],
    ["Savings rate", formatUnit(s.savings_pct || 0, "percent", true)],
    ["Input tokens", fmt.format(s.input_tokens || 0)],
    ["Output tokens", fmt.format(s.output_tokens || 0)]
  ].map(([label, value]) => `<div class="metric"><strong>${value}</strong><span>${label}</span></div>`).join("");
  renderSavingsChartSpecs(report.charts || []);
  const warnings = report.warnings || [];
  document.querySelector("#savingsWarnings").innerHTML = warnings.length ? `<div class="warnings">${warnings.map(w => `<div class="warning">${esc(w)}</div>`).join("")}</div>` : "";
  document.querySelector("#tables").innerHTML = `<div class="tablewrap"><table>${savingsRows(report.byGroup || [])}</table></div>`;
}

function populateBaselines(baselines, selected) {
  const select = document.querySelector("#baselineSelect");
  if (select.options.length && select.dataset.loaded === "true") {
    select.value = selected || select.value;
    return;
  }
  select.innerHTML = baselines.map(b => `<option value="${esc(b.baseline_id)}">${esc(b.baseline_name)}</option>`).join("") +
    `<option value="custom">Custom session baseline</option>`;
  select.dataset.loaded = "true";
  select.value = selected || "gpt-5.5";
}

function savingsRows(rows) {
  return `<thead><tr><th>Model group</th><th>Requests</th><th>Input</th><th>Output</th><th>Total tokens</th><th>Actual cost</th><th>Baseline cost</th><th>Savings</th><th>Savings %</th></tr></thead><tbody>` +
    rows.map(r => `<tr><td>${esc(r.key)}</td><td>${r.requests}</td><td>${r.input_tokens}</td><td>${r.output_tokens}</td><td>${r.total_tokens}</td><td>${usd.format(r.actual_cost_usd)}</td><td>${usd.format(r.baseline_cost_usd)}</td><td>${usd.format(r.savings_usd)}</td><td>${formatUnit(r.savings_pct, "percent", true)}</td></tr>`).join("") +
    `</tbody>`;
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

document.querySelector("#themeToggle").addEventListener("click", () => {
  const next = currentTheme() === "dark" ? "light" : "dark";
  applyTheme(next, true);
  if (lastReport) renderCharts(lastReport);
  if (lastSavings) renderSavingsChartSpecs(lastSavings.charts || []);
});

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
document.querySelector("#savingsFilters").addEventListener("submit", event => {
  event.preventDefault();
  loadSavings().catch(err => document.querySelector("#tables").innerHTML = `<div class="error">${esc(err.message)}</div>`);
});

applyTheme(storedTheme() || systemTheme(), false);
load().catch(err => document.querySelector("#tables").innerHTML = `<div class="error">${esc(err.message)}</div>`);
