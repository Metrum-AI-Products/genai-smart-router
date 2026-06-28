export type ReportFilters = Record<string, string>;

export type ReportPeriod = {
  from?: string;
  to?: string;
};

export type ReportSummary = Record<string, number | string | boolean | null | undefined>;

export type ReportChartPoint = {
  label?: string;
  time?: string;
  value?: number;
  values?: Record<string, number>;
};

export type ReportChart = {
  id?: string;
  title?: string;
  unit?: string;
  kind?: string;
  series?: ReportChartPoint[];
  points?: ReportChartPoint[];
};

export type ReportResponse = {
  period?: ReportPeriod;
  report?: string;
  summary?: ReportSummary;
  rows?: ReportRow[];
  requests?: ReportRow[];
  charts?: ReportChart[];
  series?: ReportChart[];
  cache?: Record<string, unknown>;
  byToken?: ReportRow[];
  byGroup?: ReportRow[];
  byProvider?: ReportRow[];
  byStatus?: ReportRow[];
  generatedUtc?: string;
  baseline?: Record<string, unknown>;
  baselines?: Array<Record<string, unknown>>;
  byTime?: ReportRow[];
  warnings?: string[];
};

export type ReportRow = Record<string, unknown>;

export type ReportColumn = {
  key: string;
  label: string;
  unit?: string;
  description?: string;
};

export type TabSpec = {
  id: string;
  label: string;
  endpoint: string;
  columns?: ReportColumn[];
  savings?: boolean;
  overview?: boolean;
  requests?: boolean;
  security?: boolean;
};

const identityColumns: ReportColumn[] = [
  { key: "key", label: "Key" },
  { key: "secondaryKey", label: "Secondary key" },
];

const trafficColumns: ReportColumn[] = [
  { key: "requests", label: "Requests" },
  { key: "attempts", label: "Attempts" },
  { key: "streams", label: "Streams" },
  { key: "errors", label: "Errors" },
  { key: "errorRatePct", label: "Error rate", unit: "%" },
  { key: "fallbacks", label: "Fallbacks" },
  { key: "fallbackRatePct", label: "Fallback rate", unit: "%" },
];

const tokenColumns: ReportColumn[] = [
  { key: "inputTokens", label: "Input tokens" },
  { key: "outputTokens", label: "Output tokens" },
  { key: "inputImageTokens", label: "Image tokens" },
  { key: "totalTokens", label: "Total tokens" },
  { key: "input_tokens", label: "Input tokens" },
  { key: "output_tokens", label: "Output tokens" },
  { key: "total_tokens", label: "Total tokens" },
];

const costColumns: ReportColumn[] = [
  { key: "inputCostUsd", label: "Input cost", unit: "USD" },
  { key: "imageCostUsd", label: "Image cost", unit: "USD" },
  { key: "outputCostUsd", label: "Output cost", unit: "USD" },
  { key: "totalCostUsd", label: "Total cost", unit: "USD" },
  { key: "actual_cost_usd", label: "Actual cost", unit: "USD" },
  { key: "baseline_cost_usd", label: "Baseline cost", unit: "USD" },
  { key: "savings_usd", label: "Savings", unit: "USD" },
  { key: "savings_pct", label: "Savings rate", unit: "%" },
  { key: "avgCostUsd", label: "Avg cost/request", unit: "USD" },
  { key: "upstreamReportedCostUsd", label: "Upstream billed", unit: "USD" },
  { key: "baselineCostUsd", label: "Baseline cost", unit: "USD" },
  { key: "savingsUsd", label: "Savings", unit: "USD" },
  { key: "savingsPct", label: "Savings rate", unit: "%" },
];

const latencyColumns: ReportColumn[] = [
  { key: "avgLatencyMs", label: "Avg latency", unit: "ms" },
  { key: "maxLatencyMs", label: "Max latency", unit: "ms" },
  { key: "avgTtfbMs", label: "Avg TTFB", unit: "ms" },
  { key: "maxTtfbMs", label: "Max TTFB", unit: "ms" },
  { key: "avgUpstreamMs", label: "Avg upstream", unit: "ms" },
  { key: "maxUpstreamMs", label: "Max upstream", unit: "ms" },
  { key: "avgDownstreamMs", label: "Avg downstream", unit: "ms" },
  { key: "maxDownstreamMs", label: "Max downstream", unit: "ms" },
];

const throughputColumns: ReportColumn[] = [
  { key: "avgUpstreamTokensPerSec", label: "Upstream tokens/sec", unit: "tok/s" },
  { key: "avgUpstreamOutputTokensPerSec", label: "Upstream output tok/s", unit: "tok/s" },
  { key: "avgUpstreamTotalTokensPerSec", label: "Upstream total tok/s", unit: "tok/s" },
  { key: "avgDownstreamTokensPerSec", label: "Downstream tokens/sec", unit: "tok/s" },
  { key: "avgDownstreamWriteOutputTokensPerSec", label: "Downstream output tok/s", unit: "tok/s" },
  { key: "avgDownstreamWriteTotalTokensPerSec", label: "Downstream total tok/s", unit: "tok/s" },
];

const cacheColumns: ReportColumn[] = [
  { key: "cacheHits", label: "Cache hits" },
  { key: "cacheMisses", label: "Cache misses" },
  { key: "cacheBypass", label: "Cache bypass" },
  { key: "cacheHitRatePct", label: "Cache hit rate", unit: "%" },
  { key: "latestCacheItems", label: "Latest cache items" },
  { key: "latestCacheBytes", label: "Latest cache bytes" },
  { key: "latestCacheMaxBytes", label: "Cache max bytes" },
  { key: "latestCacheOccupancyPct", label: "Cache occupancy", unit: "%" },
];

const defaultScalarColumns: ReportColumn[] = [
  ...identityColumns,
  ...trafficColumns,
  ...tokenColumns,
  ...costColumns,
  ...latencyColumns,
  ...throughputColumns,
];

const savingsColumns: ReportColumn[] = [
  { key: "key", label: "Bucket" },
  { key: "requests", label: "Requests" },
  { key: "input_tokens", label: "Input tokens" },
  { key: "output_tokens", label: "Output tokens" },
  { key: "total_tokens", label: "Total tokens" },
  { key: "actual_cost_usd", label: "Actual cost", unit: "USD" },
  { key: "baseline_cost_usd", label: "Baseline cost", unit: "USD" },
  { key: "savings_usd", label: "Savings", unit: "USD" },
  { key: "savings_pct", label: "Savings rate", unit: "%" },
];

const requestColumns: ReportColumn[] = [
  { key: "timeUtc", label: "Time" },
  { key: "requestId", label: "Request ID" },
  { key: "callerId", label: "Caller" },
  { key: "callerUser", label: "User" },
  { key: "project", label: "Project" },
  { key: "environment", label: "Environment" },
  { key: "callerIp", label: "Caller IP" },
  { key: "tokenId", label: "Token ID" },
  { key: "client", label: "Client" },
  { key: "requestedModel", label: "Requested model" },
  { key: "modelGroup", label: "Model group" },
  { key: "provider", label: "Provider" },
  { key: "model", label: "Target model" },
  { key: "dialect", label: "Dialect" },
  { key: "status", label: "Status" },
  { key: "cache", label: "Cache" },
  { key: "attempts", label: "Attempts" },
  { key: "fallback", label: "Fallback" },
  { key: "inputTokens", label: "Input tokens" },
  { key: "outputTokens", label: "Output tokens" },
  { key: "totalTokens", label: "Total tokens" },
  { key: "costUsd", label: "Cost", unit: "USD" },
  { key: "totalCostUsd", label: "Total cost", unit: "USD" },
  { key: "latencyMs", label: "Latency", unit: "ms" },
  { key: "ttfbMs", label: "TTFB", unit: "ms" },
  { key: "upstreamMs", label: "Upstream", unit: "ms" },
  { key: "downstreamMs", label: "Downstream", unit: "ms" },
  { key: "error", label: "Error" },
];

const securityColumns: ReportColumn[] = [
  { key: "timeUtc", label: "Time" },
  { key: "requestId", label: "Request ID" },
  { key: "eventType", label: "Event type" },
  { key: "surface", label: "Surface" },
  { key: "method", label: "Method" },
  { key: "path", label: "Path" },
  { key: "status", label: "Status" },
  { key: "outcome", label: "Outcome" },
  { key: "reason", label: "Reason" },
  { key: "authSubject", label: "Auth subject" },
  { key: "authSource", label: "Auth source" },
  { key: "callerId", label: "Caller" },
  { key: "callerUser", label: "User" },
  { key: "project", label: "Project" },
  { key: "adminSubject", label: "Admin subject" },
  { key: "client", label: "Client" },
  { key: "userAgentFamily", label: "User agent" },
  { key: "ipAddress", label: "IP address" },
  { key: "ipSource", label: "IP source" },
  { key: "modelGroup", label: "Model group" },
  { key: "requestedModel", label: "Requested model" },
  { key: "resolvedGroup", label: "Resolved group" },
  { key: "inputTokens", label: "Input tokens" },
  { key: "outputTokens", label: "Output tokens" },
  { key: "totalTokens", label: "Total tokens" },
];

const retentionColumns: ReportColumn[] = [
  { key: "dataClass", label: "Data class" },
  { key: "tableName", label: "Table" },
  { key: "rollupType", label: "Rollup type" },
  { key: "status", label: "Status" },
  { key: "retentionDays", label: "Retention days" },
  { key: "candidateRows", label: "Candidate rows" },
  { key: "eligibleRows", label: "Eligible rows" },
  { key: "heldRows", label: "Held rows" },
  { key: "blockedRows", label: "Blocked rows" },
  { key: "deletedRows", label: "Deleted rows" },
  { key: "sourceRequestCount", label: "Source requests" },
  { key: "dailyRows", label: "Daily rows" },
  { key: "rollupRows", label: "Rollup rows" },
  { key: "windowStart", label: "Window start" },
  { key: "windowEnd", label: "Window end" },
  { key: "completedAt", label: "Completed" },
  { key: "message", label: "Message" },
];

const catalogColumns: ReportColumn[] = [
  { key: "source", label: "Source" },
  { key: "provider", label: "Provider" },
  { key: "modelRef", label: "Model ref" },
  { key: "model", label: "Model" },
  { key: "dialect", label: "Dialect" },
  { key: "activeGroups", label: "Active groups" },
  { key: "activeTargetCount", label: "Active targets" },
  { key: "validationStatus", label: "Validation" },
  { key: "validationWorkload", label: "Workload" },
  { key: "validationAgeBucket", label: "Validation age" },
  { key: "qualityScore", label: "Quality score" },
  { key: "passRate", label: "Pass rate", unit: "%" },
  { key: "contextTokens", label: "Context tokens" },
  { key: "inputModalities", label: "Input modalities" },
  { key: "outputModalities", label: "Output modalities" },
  { key: "toolSupport", label: "Tool support" },
  { key: "inputPricePerMillionUsd", label: "Input price/M", unit: "USD" },
  { key: "outputPricePerMillionUsd", label: "Output price/M", unit: "USD" },
  { key: "pricingSource", label: "Pricing source" },
  { key: "pricingUpdatedAt", label: "Pricing updated" },
  { key: "pricingMissing", label: "Pricing missing" },
];

export const tabSpecs: TabSpec[] = [
  { id: "overview", label: "Overview", endpoint: "summary", overview: true },
  { id: "groups", label: "Groups", endpoint: "summary", columns: defaultScalarColumns },
  { id: "providers", label: "Providers", endpoint: "summary", columns: defaultScalarColumns },
  { id: "tokens", label: "Keys", endpoint: "summary", columns: defaultScalarColumns },
  { id: "savings", label: "Savings", endpoint: "savings", columns: savingsColumns, savings: true },
  { id: "savings-by-user", label: "Savings by user", endpoint: "savings-by-user", columns: defaultScalarColumns, savings: true },
  { id: "savings-by-key", label: "Savings by key", endpoint: "savings-by-key", columns: defaultScalarColumns, savings: true },
  { id: "savings-by-group", label: "Savings by group", endpoint: "savings-by-group", columns: defaultScalarColumns, savings: true },
  { id: "savings-by-project", label: "Savings by project", endpoint: "savings-by-project", columns: defaultScalarColumns, savings: true },
  { id: "savings-by-provider-model", label: "Savings by provider", endpoint: "savings-by-provider-model", columns: defaultScalarColumns, savings: true },
  { id: "model-groups-by-user", label: "User groups", endpoint: "model-groups-by-user", columns: defaultScalarColumns },
  { id: "usage-by-key", label: "Key usage", endpoint: "usage-by-key", columns: defaultScalarColumns },
  { id: "usage-by-caller", label: "Caller usage", endpoint: "usage-by-caller", columns: defaultScalarColumns },
  { id: "requested-models", label: "Requested models", endpoint: "requested-models", columns: defaultScalarColumns },
  { id: "provider-model-mix", label: "Provider/model", endpoint: "provider-model-mix", columns: defaultScalarColumns },
  { id: "latency-throughput", label: "Latency", endpoint: "latency-throughput", columns: [...identityColumns, ...trafficColumns, ...latencyColumns, ...throughputColumns, ...tokenColumns] },
  { id: "errors-fallbacks", label: "Errors", endpoint: "errors-fallbacks", columns: [...identityColumns, ...trafficColumns, ...latencyColumns] },
  { id: "cache-report", label: "Cache", endpoint: "cache", columns: [...identityColumns, ...trafficColumns, ...cacheColumns] },
  { id: "quotas-budgets", label: "Quotas", endpoint: "quotas-budgets", columns: defaultScalarColumns },
  { id: "troubleshooting-buckets", label: "Troubleshooting", endpoint: "troubleshooting-buckets", columns: defaultScalarColumns },
  { id: "routing-decisions", label: "Routing", endpoint: "routing-decisions", columns: defaultScalarColumns },
  { id: "dynamic-signals", label: "Dynamic signals", endpoint: "dynamic-signals", columns: defaultScalarColumns },
  { id: "dynamic-score-buckets", label: "Dynamic scores", endpoint: "dynamic-score-buckets", columns: defaultScalarColumns },
  { id: "dynamic-thresholds", label: "Dynamic thresholds", endpoint: "dynamic-thresholds", columns: defaultScalarColumns },
  { id: "max-token-buckets", label: "Max tokens", endpoint: "max-token-buckets", columns: defaultScalarColumns },
  { id: "input-token-buckets", label: "Input tokens", endpoint: "input-token-buckets", columns: defaultScalarColumns },
  { id: "admission-reasons", label: "Admission", endpoint: "admission-reasons", columns: defaultScalarColumns },
  { id: "provider-catalog-status", label: "Catalog status", endpoint: "provider-catalog-status", columns: catalogColumns },
  { id: "retention-status", label: "Retention", endpoint: "retention-status", columns: retentionColumns },
  { id: "contract-buckets", label: "Contracts", endpoint: "contract-buckets", columns: defaultScalarColumns },
  { id: "contract-workloads", label: "Workloads", endpoint: "contract-workloads", columns: defaultScalarColumns },
  { id: "target-validation", label: "Validation", endpoint: "target-validation", columns: defaultScalarColumns },
  { id: "expensive-requests", label: "Expensive", endpoint: "expensive-requests", columns: requestColumns, requests: true },
  { id: "client-breakdown", label: "Clients", endpoint: "client-breakdown", columns: defaultScalarColumns },
  { id: "project-chargeback", label: "Projects", endpoint: "project-chargeback", columns: defaultScalarColumns },
  { id: "capability-usage", label: "Capabilities", endpoint: "capability-usage", columns: defaultScalarColumns },
  { id: "anomalies", label: "Anomalies", endpoint: "anomalies", columns: defaultScalarColumns },
  { id: "security-events", label: "Security", endpoint: "security/events", columns: securityColumns, security: true },
  { id: "requests", label: "Requests", endpoint: "summary", columns: requestColumns, requests: true },
];

export const filterFields = [
  ["since", "Since", "24h"],
  ["caller_id", "Caller", ""],
  ["caller_ip", "IP", ""],
  ["caller_project", "Project", ""],
  ["requested_model", "Requested", ""],
  ["resolved_group", "Group", ""],
  ["provider", "Provider", ""],
  ["target_model", "Target", ""],
  ["dialect", "Dialect", ""],
  ["status", "Status", ""],
  ["cache", "Cache", ""],
  ["client", "Client", ""],
] as const;

export async function fetchReport(endpoint: string, filters: ReportFilters): Promise<ReportResponse> {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value.trim()) params.set(key, value.trim());
  }
  const res = await fetch(`api/${endpoint}?${params}`, { credentials: "same-origin" });
  if (!res.ok) throw new Error(`${endpoint} failed with HTTP ${res.status}`);
  return (await res.json()) as ReportResponse;
}

export function rowsForTab(tab: TabSpec, report: ReportResponse): ReportRow[] {
  if (tab.id === "groups") return report.byGroup || [];
  if (tab.id === "providers") return report.byProvider || [];
  if (tab.id === "tokens") return report.byToken || [];
  if (tab.requests || tab.id === "requests") return report.requests || [];
  if (tab.id === "retention-status") {
    const retention = report as ReportResponse & { tables?: ReportRow[]; rollups?: ReportRow[] };
    return [...(retention.tables || []), ...(retention.rollups || [])];
  }
  return report.rows || report.byTime || [];
}

export function columnsForTab(tab: TabSpec, rows: ReportRow[]): ReportColumn[] {
  const schema = tab.columns?.length ? tab.columns : inferColumns(rows);
  const available = new Set<string>();
  for (const row of rows) {
    for (const key of Object.keys(row)) {
      if (hasVisibleValue(row[key])) available.add(key);
    }
  }
  const selected = schema.filter((column) => available.has(column.key));
  const seen = new Set<string>();
  const deduped = selected.filter((column) => {
    if (seen.has(column.key)) return false;
    seen.add(column.key);
    return true;
  });
  return suppressRedundantAliases(deduped, rows);
}

function inferColumns(rows: ReportRow[]): ReportColumn[] {
  const seen = new Set<string>();
  for (const row of rows) {
    for (const key of Object.keys(row)) seen.add(key);
  }
  return Array.from(seen)
    .sort((a, b) => a.localeCompare(b))
    .map((key) => ({ key, label: key }));
}

function hasVisibleValue(value: unknown): boolean {
  if (value === null || value === undefined || value === "") return false;
  if (Array.isArray(value)) return value.length > 0;
  return true;
}

function suppressRedundantAliases(columns: ReportColumn[], rows: ReportRow[]): ReportColumn[] {
  const keys = new Set(columns.map((column) => column.key));
  const remove = new Set<string>();
  if (keys.has("tokens") && keys.has("totalTokens") && rowsEveryEqual(rows, "tokens", "totalTokens")) remove.add("tokens");
  if (keys.has("costUsd") && keys.has("totalCostUsd") && rowsEveryEqual(rows, "costUsd", "totalCostUsd")) remove.add("costUsd");
  return columns.filter((column) => !remove.has(column.key));
}

function rowsEveryEqual(rows: ReportRow[], left: string, right: string): boolean {
  return rows.length > 0 && rows.every((row) => String(row[left] ?? "") === String(row[right] ?? ""));
}
