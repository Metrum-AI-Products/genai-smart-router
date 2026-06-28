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

export type TabSpec = {
  id: string;
  label: string;
  endpoint: string;
  savings?: boolean;
  overview?: boolean;
  requests?: boolean;
  security?: boolean;
};

export const tabSpecs: TabSpec[] = [
  { id: "overview", label: "Overview", endpoint: "summary", overview: true },
  { id: "groups", label: "Groups", endpoint: "summary" },
  { id: "providers", label: "Providers", endpoint: "summary" },
  { id: "tokens", label: "Keys", endpoint: "summary" },
  { id: "savings", label: "Savings", endpoint: "savings", savings: true },
  { id: "savings-by-user", label: "Savings by user", endpoint: "savings-by-user", savings: true },
  { id: "savings-by-key", label: "Savings by key", endpoint: "savings-by-key", savings: true },
  { id: "savings-by-group", label: "Savings by group", endpoint: "savings-by-group", savings: true },
  { id: "savings-by-project", label: "Savings by project", endpoint: "savings-by-project", savings: true },
  { id: "savings-by-provider-model", label: "Savings by provider", endpoint: "savings-by-provider-model", savings: true },
  { id: "model-groups-by-user", label: "User groups", endpoint: "model-groups-by-user" },
  { id: "usage-by-key", label: "Key usage", endpoint: "usage-by-key" },
  { id: "usage-by-caller", label: "Caller usage", endpoint: "usage-by-caller" },
  { id: "requested-models", label: "Requested models", endpoint: "requested-models" },
  { id: "provider-model-mix", label: "Provider/model", endpoint: "provider-model-mix" },
  { id: "latency-throughput", label: "Latency", endpoint: "latency-throughput" },
  { id: "errors-fallbacks", label: "Errors", endpoint: "errors-fallbacks" },
  { id: "cache-report", label: "Cache", endpoint: "cache" },
  { id: "quotas-budgets", label: "Quotas", endpoint: "quotas-budgets" },
  { id: "troubleshooting-buckets", label: "Troubleshooting", endpoint: "troubleshooting-buckets" },
  { id: "routing-decisions", label: "Routing", endpoint: "routing-decisions" },
  { id: "dynamic-signals", label: "Dynamic signals", endpoint: "dynamic-signals" },
  { id: "dynamic-score-buckets", label: "Dynamic scores", endpoint: "dynamic-score-buckets" },
  { id: "dynamic-thresholds", label: "Dynamic thresholds", endpoint: "dynamic-thresholds" },
  { id: "max-token-buckets", label: "Max tokens", endpoint: "max-token-buckets" },
  { id: "input-token-buckets", label: "Input tokens", endpoint: "input-token-buckets" },
  { id: "admission-reasons", label: "Admission", endpoint: "admission-reasons" },
  { id: "provider-catalog-status", label: "Catalog status", endpoint: "provider-catalog-status" },
  { id: "retention-status", label: "Retention", endpoint: "retention-status" },
  { id: "contract-buckets", label: "Contracts", endpoint: "contract-buckets" },
  { id: "contract-workloads", label: "Workloads", endpoint: "contract-workloads" },
  { id: "target-validation", label: "Validation", endpoint: "target-validation" },
  { id: "expensive-requests", label: "Expensive", endpoint: "expensive-requests", requests: true },
  { id: "client-breakdown", label: "Clients", endpoint: "client-breakdown" },
  { id: "project-chargeback", label: "Projects", endpoint: "project-chargeback" },
  { id: "capability-usage", label: "Capabilities", endpoint: "capability-usage" },
  { id: "anomalies", label: "Anomalies", endpoint: "anomalies" },
  { id: "security-events", label: "Security", endpoint: "security/events", security: true },
  { id: "requests", label: "Requests", endpoint: "summary", requests: true },
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
