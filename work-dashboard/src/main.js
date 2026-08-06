import * as vg from "@uwdata/vgplot";
import {detailList, taskDetailSections, toggleExpandedTask} from "./task-details.js";
import "./style.css";

const STATUS_ORDER = ["blocked", "in_progress", "ready", "pending", "done", "cancelled"];
const PRIORITY_ORDER = {critical: 0, high: 1, normal: 2, low: 3};
const app = document.querySelector("#app");

function parseNdjson(text, source) {
  return text.split(/\r?\n/).filter(line => line.trim()).map((line, index) => {
    try {
      return JSON.parse(line);
    } catch (error) {
      throw new Error(`${source} line ${index + 1}: ${error.message}`);
    }
  });
}

function scheduleBucket(task) {
  if (["done", "cancelled"].includes(task.status)) return "closed";
  if (!task.due_date) return "unscheduled";
  const today = new Date();
  const utcToday = Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate());
  const due = Date.parse(`${task.due_date}T00:00:00Z`);
  const days = Math.round((due - utcToday) / 86_400_000);
  if (days < 0) return "overdue";
  if (days === 0) return "due_today";
  if (days <= 7) return "due_next_7_days";
  return "scheduled_later";
}


function reconcile(baseline, events) {
  const baselineTasks = baseline.filter(record => record.kind === "task");
  const tasks = new Map();
  const revisions = new Map();
  const normalizedTaskIds = new Map();
  const grouped = new Map();
  const conflicts = [];
  const eventIds = new Set();

  for (const task of baselineTasks) {
    const normalized = String(task.id).toLocaleLowerCase();
    if (normalizedTaskIds.has(normalized)) {
      conflicts.push({
        type: "duplicate_task_id",
        message: `${task.id} conflicts with existing task ID ${normalizedTaskIds.get(normalized)}`,
      });
      continue;
    }
    normalizedTaskIds.set(normalized, task.id);
    tasks.set(task.id, structuredClone(task));
    revisions.set(task.id, 0);
  }

  for (const event of events) {
    if (!event.event_id || eventIds.has(event.event_id)) {
      conflicts.push({type: "duplicate_event", message: `Duplicate or missing event ID: ${event.event_id ?? "unknown"}`});
      continue;
    }
    eventIds.add(event.event_id);
    if (!grouped.has(event.task_id)) grouped.set(event.task_id, []);
    grouped.get(event.task_id).push(event);
  }

  for (const [taskId, taskEvents] of grouped) {
    const seenRevisions = new Set();
    let current = tasks.get(taskId);
    let revision = 0;
    let conflicted = false;
    for (const event of taskEvents.sort((a, b) => a.revision - b.revision || a.event_id.localeCompare(b.event_id))) {
      if (seenRevisions.has(event.revision)) {
        conflicts.push({type: "duplicate_revision", message: `${taskId} has multiple events at revision ${event.revision}`});
        conflicted = true;
        continue;
      }
      seenRevisions.add(event.revision);
      if (event.base_revision !== revision || event.revision !== revision + 1) {
        conflicts.push({type: "revision_gap", message: `${taskId} expected base ${revision} / revision ${revision + 1}, received ${event.base_revision} / ${event.revision}`});
        conflicted = true;
        continue;
      }
      if (event.event_type === "task.created") {
        const existingId = normalizedTaskIds.get(taskId.toLocaleLowerCase());
        if (current || existingId) {
          conflicts.push({type: "create_existing", message: `${taskId} creation conflicts with existing task ID ${existingId ?? taskId}`});
          conflicted = true;
          continue;
        }
      }
      if (event.event_type === "task.updated" && !current) {
        conflicts.push({type: "update_missing", message: `${taskId} update has no prior task`});
        conflicted = true;
        continue;
      }
      if (!event.task || event.task.id !== taskId) {
        conflicts.push({type: "invalid_snapshot", message: `${taskId} event ${event.event_id} has no matching task snapshot`});
        conflicted = true;
        continue;
      }
      if (event.event_type === "task.created") {
        normalizedTaskIds.set(taskId.toLocaleLowerCase(), taskId);
      }
      current = structuredClone(event.task);
      revision = event.revision;
    }
    if (!conflicted && current) {
      tasks.set(taskId, current);
      revisions.set(taskId, revision);
    }
  }

  const rows = [...tasks.values()].map(task => {
    const hasDetail = Boolean(
      task.description?.trim()
      || detailList(task.actions).length
      || detailList(task.acceptance).length
      || detailList(task.evidence).length
      || detailList(task.commands).length
    );
    if (!hasDetail) {
      conflicts.push({type: "missing_drilldown_detail", message: `${task.id} has no drilldown detail`});
    }
    return {
      id: task.id,
      stream: task.stream,
      phase: task.phase,
      title: task.title,
      description: task.description ?? null,
      actions: detailList(task.actions),
      acceptance: detailList(task.acceptance),
      evidence: detailList(task.evidence),
      commands: detailList(task.commands),
      requires: task.requires ?? [],
      references: task.references ?? [],
      status: task.status,
      due_date: task.due_date,
      due_bucket: scheduleBucket(task),
      priority: task.priority,
      assignee: task.assignee,
      revision: revisions.get(task.id) ?? 0,
      source: (revisions.get(task.id) ?? 0) > 0 ? "event" : "baseline",
      created_at: task.created_at,
      updated_at: task.updated_at,
      started_at: task.started_at,
      completed_at: task.completed_at,
    };
  });
  rows.sort((a, b) => a.status.localeCompare(b.status) || String(a.due_date ?? "9999").localeCompare(String(b.due_date ?? "9999")) || a.id.localeCompare(b.id));
  return {tasks: rows, conflicts, baselineTaskCount: baselineTasks.length};
}

async function loadSnapshot() {
  const [baselineResponse, eventsResponse, versionResponse] = await Promise.all([
    fetch("/data/work-items.ndjson", {cache: "no-store"}),
    fetch("/data/work-item-events.ndjson", {cache: "no-store"}),
    fetch("/data/version", {cache: "no-store"}),
  ]);
  for (const response of [baselineResponse, eventsResponse, versionResponse]) {
    if (!response.ok) throw new Error(`Dashboard data request failed: HTTP ${response.status}`);
  }
  const [baselineText, eventsText, version] = await Promise.all([
    baselineResponse.text(),
    eventsResponse.text(),
    versionResponse.json(),
  ]);
  const baseline = parseNdjson(baselineText, "work-items.ndjson");
  const events = parseNdjson(eventsText, "work-item-events.ndjson");
  const reconciled = reconcile(baseline, events);
  return {
    healthy: true,
    version: version.version,
    generated_at: new Date().toISOString(),
    reconciliation: {
      baseline_task_count: reconciled.baselineTaskCount,
      event_count: events.length,
      current_task_count: reconciled.tasks.length,
      conflict_count: reconciled.conflicts.length,
    },
    tasks: reconciled.tasks,
    recent_events: events
      .map(({event_id, event_type, task_id, revision, occurred_at, actor, status, due_date, changed_fields}) => ({
        event_id, event_type, task_id, revision, occurred_at, actor, status, due_date, changed_fields,
      }))
      .sort((a, b) => b.occurred_at.localeCompare(a.occurred_at) || b.event_id.localeCompare(a.event_id))
      .slice(0, 25),
    conflicts: reconciled.conflicts,
  };
}

const snapshot = await loadSnapshot();

app.innerHTML = `
  <header class="topbar">
    <div class="brand-lockup">
      <div class="mark" aria-hidden="true"><span></span><span></span><span></span></div>
      <div>
        <p class="eyebrow">GENAI SMART ROUTER</p>
        <h1>Work Registry</h1>
      </div>
    </div>
    <div class="live-state" id="live-state"><span class="live-dot"></span><span>Connecting</span></div>
  </header>

  <section class="hero">
    <div>
      <p class="section-kicker">OPERATIONS / RECONCILED STATE</p>
      <h2>One view of every moving part.</h2>
      <p class="lede">Baseline records and append-only events, reconciled live. Charts run on Mosaic and DuckDB-WASM in your browser.</p>
    </div>
    <div class="refresh-meta">
      <span>Snapshot</span>
      <strong id="snapshot-version"></strong>
      <small id="refresh-time"></small>
    </div>
  </section>

  <section class="alert hidden" id="conflict-alert" role="alert">
    <div class="alert-icon">!</div>
    <div><strong>Reconciliation blocked</strong><p id="conflict-message"></p></div>
  </section>

  <section class="metrics" aria-label="Registry summary">
    <article><span>Current tasks</span><strong id="metric-tasks">—</strong><small>reconciled</small></article>
    <article><span>Open work</span><strong id="metric-open">—</strong><small>not done or cancelled</small></article>
    <article><span>Event journal</span><strong id="metric-events">—</strong><small>append-only records</small></article>
    <article><span>Conflicts</span><strong id="metric-conflicts">—</strong><small>revision integrity</small></article>
  </section>

  <section class="chart-grid">
    <article class="panel chart-panel">
      <div class="panel-heading"><div><span class="panel-index">01</span><h3>Status distribution</h3></div><span class="tech-badge">MOSAIC</span></div>
      <div class="chart" id="status-chart"><div class="chart-loading">Initializing DuckDB-WASM…</div></div>
    </article>
    <article class="panel chart-panel">
      <div class="panel-heading"><div><span class="panel-index">02</span><h3>Work by stream</h3></div><span class="tech-badge">MOSAIC</span></div>
      <div class="chart" id="stream-chart"><div class="chart-loading">Preparing query…</div></div>
    </article>
    <article class="panel chart-panel wide-chart">
      <div class="panel-heading"><div><span class="panel-index">03</span><h3>Schedule pressure</h3></div><span class="tech-badge">MOSAIC</span></div>
      <div class="chart" id="due-chart"><div class="chart-loading">Preparing query…</div></div>
    </article>
  </section>

  <section class="panel registry-panel">
    <div class="panel-heading registry-heading">
      <div><span class="panel-index">04</span><h3>Task registry</h3></div>
      <div class="row-count" id="row-count"></div>
    </div>
    <div class="filters">
      <label class="search-control"><span>Search</span><input id="search" type="search" placeholder="ID, title, assignee…" autocomplete="off" /></label>
      <label><span>Status</span><select id="status-filter"><option value="">All statuses</option></select></label>
      <label><span>Stream</span><select id="stream-filter"><option value="">All streams</option></select></label>
      <label><span>Priority</span><select id="priority-filter"><option value="">All priorities</option></select></label>
    </div>
    <div class="table-wrap">
      <table>
        <thead><tr><th>Status</th><th>Task</th><th>Stream</th><th>Due</th><th>Priority</th><th>Owner</th><th>Rev</th></tr></thead>
        <tbody id="task-rows"></tbody>
      </table>
      <div class="empty hidden" id="empty-state">No tasks match these filters.</div>
    </div>
  </section>

  <section class="panel activity-panel">
    <div class="panel-heading"><div><span class="panel-index">05</span><h3>Recent journal activity</h3></div><span class="read-only">READ ONLY</span></div>
    <div class="event-list" id="event-list"></div>
  </section>

  <footer><span>Source: work-items.ndjson + work-item-events.ndjson</span><span>Browser reconciliation · live version polling</span></footer>
`;

const byId = id => document.getElementById(id);
const reconciliation = snapshot.reconciliation;
byId("snapshot-version").textContent = snapshot.version;
byId("refresh-time").textContent = new Date(snapshot.generated_at).toLocaleString();
byId("metric-tasks").textContent = reconciliation.current_task_count;
byId("metric-open").textContent = snapshot.tasks.filter(task => !["done", "cancelled"].includes(task.status)).length;
byId("metric-events").textContent = reconciliation.event_count;
byId("metric-conflicts").textContent = reconciliation.conflict_count;

if (!snapshot.healthy || snapshot.conflicts.length) {
  byId("conflict-alert").classList.remove("hidden");
  byId("conflict-message").textContent = snapshot.conflicts.map(conflict => conflict.message).join(" · ");
}

function addOptions(select, values) {
  values.forEach(value => {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = value.replaceAll("_", " ");
    select.append(option);
  });
}

const tasks = snapshot.tasks;
addOptions(byId("status-filter"), STATUS_ORDER.filter(status => tasks.some(task => task.status === status)));
addOptions(byId("stream-filter"), [...new Set(tasks.map(task => task.stream))].sort());
addOptions(byId("priority-filter"), ["critical", "high", "normal", "low"].filter(priority => tasks.some(task => task.priority === priority)));

function cell(text, className = "") {
  const td = document.createElement("td");
  td.textContent = text ?? "—";
  if (className) td.className = className;
  return td;
}
const expandedTaskIds = new Set();

function appendDetailSection(container, label, values) {
  const section = document.createElement("section");
  section.className = "task-detail-section";
  const heading = document.createElement("h5");
  heading.textContent = label;
  const list = document.createElement("ul");
  values.forEach(value => {
    const item = document.createElement("li");
    item.textContent = value;
    list.append(item);
  });
  section.append(heading, list);
  container.append(section);
}

function createTaskDetailRow(task, detailId) {
  const detailRow = document.createElement("tr");
  detailRow.id = detailId;
  detailRow.className = "task-detail-row";
  detailRow.hidden = !expandedTaskIds.has(task.id);

  const detailCell = document.createElement("td");
  detailCell.colSpan = 7;
  const panel = document.createElement("div");
  panel.className = "task-detail-panel";

  const heading = document.createElement("h4");
  heading.textContent = `${task.title} details`;
  panel.append(heading);

  if (task.description) {
    const description = document.createElement("section");
    description.className = "task-detail-description";
    const label = document.createElement("h5");
    label.textContent = "Description";
    const copy = document.createElement("p");
    copy.textContent = task.description;
    description.append(label, copy);
    panel.append(description);
  }

  const sections = document.createElement("div");
  sections.className = "task-detail-sections";
  taskDetailSections(task).forEach(section => {
    appendDetailSection(sections, section.label, section.values);
  });
  panel.append(sections);

  const metadata = document.createElement("p");
  metadata.className = "task-detail-metadata";
  metadata.textContent = [
    `Status: ${task.status.replaceAll("_", " ")}`,
    `Priority: ${task.priority}`,
    `Revision: ${task.revision}`,
    `Created: ${task.created_at}`,
    `Updated: ${task.updated_at}`,
  ].join(" · ");
  panel.append(metadata);
  detailCell.append(panel);
  detailRow.append(detailCell);
  return detailRow;
}
function updateExpansion(button, summaryRow, detailRow, task, expanded) {
  button.setAttribute("aria-expanded", String(expanded));
  button.setAttribute("aria-label", `${expanded ? "Collapse" : "Expand"} details for ${task.title}`);
  button.textContent = expanded ? "−" : "+";
  summaryRow.classList.toggle("expanded", expanded);
  detailRow.hidden = !expanded;
}



function renderRows() {
  const query = byId("search").value.trim().toLowerCase();
  const status = byId("status-filter").value;
  const stream = byId("stream-filter").value;
  const priority = byId("priority-filter").value;
  const visible = tasks
    .filter(task => !status || task.status === status)
    .filter(task => !stream || task.stream === stream)
    .filter(task => !priority || task.priority === priority)
    .filter(task => !query || [task.id, task.title, task.assignee, task.stream].some(value => String(value ?? "").toLowerCase().includes(query)))
    .sort((a, b) => {
      const statusDiff = STATUS_ORDER.indexOf(a.status) - STATUS_ORDER.indexOf(b.status);
      if (statusDiff) return statusDiff;
      const dueDiff = String(a.due_date ?? "9999").localeCompare(String(b.due_date ?? "9999"));
      if (dueDiff) return dueDiff;
      return (PRIORITY_ORDER[a.priority] ?? 9) - (PRIORITY_ORDER[b.priority] ?? 9);
    });

  const body = byId("task-rows");
  body.replaceChildren();
  visible.forEach(task => {
    const row = document.createElement("tr");
    row.className = "task-summary-row";
    const statusCell = document.createElement("td");
    const statusPill = document.createElement("span");
    statusPill.className = `status-pill status-${task.status}`;
    statusPill.textContent = task.status.replaceAll("_", " ");
    statusCell.append(statusPill);
    row.append(statusCell);

    const detailId = `task-detail-${task.id}`;
    const detailRow = createTaskDetailRow(task, detailId);
    const taskCell = document.createElement("td");
    taskCell.className = "task-summary";
    const title = document.createElement("strong");
    title.textContent = task.title;
    const id = document.createElement("small");
    id.textContent = task.id;
    const expandButton = document.createElement("button");
    expandButton.type = "button";
    expandButton.className = "task-expand";
    expandButton.setAttribute("aria-controls", detailId);
    updateExpansion(expandButton, row, detailRow, task, expandedTaskIds.has(task.id));
    expandButton.addEventListener("click", () => {
      updateExpansion(expandButton, row, detailRow, task, toggleExpandedTask(expandedTaskIds, task.id));
    });
    expandButton.addEventListener("keydown", event => {
      if (event.key !== "Escape" || !expandedTaskIds.has(task.id)) return;
      event.preventDefault();
      expandedTaskIds.delete(task.id);
      updateExpansion(expandButton, row, detailRow, task, false);
    });
    taskCell.append(title, id, expandButton);
    row.append(taskCell);
    row.append(cell(task.stream, "mono-cell"));
    row.append(cell(task.due_date ?? "Unscheduled", task.due_bucket === "overdue" ? "due-overdue" : ""));
    row.append(cell(task.priority, `priority priority-${task.priority}`));
    row.append(cell(task.assignee ?? "Unassigned"));
    row.append(cell(String(task.revision), "revision-cell"));
    body.append(row, detailRow);
  });
  byId("row-count").textContent = `${visible.length} / ${tasks.length} tasks`;
  byId("empty-state").classList.toggle("hidden", visible.length !== 0);
}

["search", "status-filter", "stream-filter", "priority-filter"].forEach(id => byId(id).addEventListener("input", renderRows));
renderRows();

const eventList = byId("event-list");
if (!snapshot.recent_events.length) {
  eventList.innerHTML = '<div class="empty-event">No journal events yet.</div>';
} else {
  snapshot.recent_events.slice(0, 8).forEach(event => {
    const item = document.createElement("article");
    item.className = "event-item";
    const glyph = document.createElement("div");
    glyph.className = `event-glyph ${event.event_type === "task.created" ? "created" : "updated"}`;
    glyph.textContent = event.event_type === "task.created" ? "+" : "↗";
    const content = document.createElement("div");
    const heading = document.createElement("strong");
    heading.textContent = event.task_id;
    const details = document.createElement("p");
    details.textContent = `${event.event_type.replace("task.", "")} · ${event.actor} · revision ${event.revision}`;
    const timestamp = document.createElement("time");
    timestamp.dateTime = event.occurred_at;
    timestamp.textContent = new Date(event.occurred_at).toLocaleString();
    content.append(heading, details);
    item.append(glyph, content, timestamp);
    eventList.append(item);
  });
}

async function renderMosaicCharts() {
  if (!snapshot.healthy || !tasks.length) {
    document.querySelectorAll(".chart").forEach(chart => { chart.textContent = "No reconciled data available."; });
    return;
  }
  vg.coordinator().databaseConnector(vg.wasmConnector());
  const columns = ["id", "stream", "phase", "title", "status", "due_date", "due_bucket", "priority", "assignee", "revision", "source", "updated_at"];
  const sqlValue = value => {
    if (value === null || value === undefined || value === "") return "NULL";
    if (typeof value === "number") return String(value);
    return `'${String(value).replaceAll("'", "''")}'`;
  };
  const values = tasks.map(task => `(${columns.map(column => sqlValue(task[column])).join(",")})`).join(",");
  await vg.coordinator().exec(`CREATE OR REPLACE TABLE work_items AS SELECT * FROM (VALUES ${values}) AS t(${columns.join(",")})`);

  const shared = [vg.marginLeft(118), vg.marginRight(20), vg.marginTop(16), vg.marginBottom(42), vg.height(250)];
  const statusChart = vg.plot(
    vg.barX(vg.from("work_items"), {y: "status", x: vg.count(), fill: "status", sort: {y: "-x"}, tip: true}),
    vg.width(Math.max(360, byId("status-chart").clientWidth - 4)),
    vg.xLabel("Tasks"),
    vg.yLabel(null),
    ...shared,
  );
  byId("status-chart").replaceChildren(statusChart);

  const streamChart = vg.plot(
    vg.barX(vg.from("work_items"), {y: "stream", x: vg.count(), fill: "status", sort: {y: "-x"}, tip: true}),
    vg.width(Math.max(360, byId("stream-chart").clientWidth - 4)),
    vg.xLabel("Tasks"),
    vg.yLabel(null),
    ...shared,
  );
  byId("stream-chart").replaceChildren(streamChart);

  const dueChart = vg.plot(
    vg.barY(vg.from("work_items"), {x: "due_bucket", y: vg.count(), fill: "due_bucket", sort: {x: "-y"}, tip: true}),
    vg.width(Math.max(720, byId("due-chart").clientWidth - 4)),
    vg.height(260),
    vg.marginLeft(56),
    vg.marginRight(20),
    vg.marginTop(16),
    vg.marginBottom(70),
    vg.xLabel(null),
    vg.yLabel("Tasks"),
  );
  byId("due-chart").replaceChildren(dueChart);
}

renderMosaicCharts().catch(error => {
  console.error(error);
  document.querySelectorAll(".chart").forEach(chart => {
    chart.textContent = `Mosaic query failed: ${error.message}`;
    chart.classList.add("chart-error");
  });
});

const liveState = byId("live-state");
let polling = true;

async function pollVersion() {
  try {
    const response = await fetch("/data/version", {cache: "no-store"});
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const current = await response.json();
    liveState.className = "live-state connected";
    liveState.lastElementChild.textContent = "Live";
    if (current.version !== snapshot.version) {
      polling = false;
      window.location.reload();
      return;
    }
  } catch (error) {
    console.error("Live refresh unavailable", error);
    liveState.className = "live-state disconnected";
    liveState.lastElementChild.textContent = "Reconnecting";
  }
  if (polling) window.setTimeout(pollVersion, 1000);
}

pollVersion();
