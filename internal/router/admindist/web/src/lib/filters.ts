export type FilterField<Name extends string = string> = readonly [name: Name, label: string, defaultValue: string];

export type GlobalFilterName =
  | "since"
  | "caller_id"
  | "caller_user"
  | "token_id"
  | "caller_ip"
  | "caller_project"
  | "caller_environment"
  | "requested_model"
  | "resolved_group"
  | "provider"
  | "target_model"
  | "dialect"
  | "client";

export type TabFilterName = "baseline" | "status" | "cache" | "sort" | "direction" | "traffic_shape_bucket" | "traffic_shape_scope" | "limit";

export type ReportFilterName = GlobalFilterName | TabFilterName;

export const globalFilterFields = [
  ["since", "Since", "24h"],
  ["caller_id", "Caller", ""],
  ["caller_user", "User", ""],
  ["token_id", "Token ID", ""],
  ["caller_ip", "IP", ""],
  ["caller_project", "Project", ""],
  ["caller_environment", "Environment", ""],
  ["requested_model", "Requested", ""],
  ["resolved_group", "Group", ""],
  ["provider", "Provider", ""],
  ["target_model", "Target", ""],
  ["dialect", "Dialect", ""],
  ["client", "Client", ""],
] as const satisfies ReadonlyArray<FilterField<GlobalFilterName>>;

export const tabFilterFields = [
  ["baseline", "Baseline", ""],
  ["status", "Status", ""],
  ["cache", "Cache", ""],
  ["traffic_shape_bucket", "Shape bucket", ""],
  ["traffic_shape_scope", "Shape scope", ""],
  ["sort", "Sort", ""],
  ["direction", "Direction", ""],
  ["limit", "Rows", "50"],
] as const satisfies ReadonlyArray<FilterField<TabFilterName>>;

export const filterFields = [...globalFilterFields, ...tabFilterFields] as const satisfies ReadonlyArray<FilterField<ReportFilterName>>;

export function validateFilterModel() {
  const names = filterFields.map(([name]) => name);
  const globalNames = globalFilterFields.map(([name]) => name);
  const tabNames = tabFilterFields.map(([name]) => name);
  return {
    duplicateNames: names.filter((name, index) => names.indexOf(name) !== index),
    overlappingNames: globalNames.filter((name) => tabNames.includes(name as TabFilterName)),
    hasEveryGlobal: globalNames.every((name) => names.includes(name)),
    hasEveryTab: tabNames.every((name) => names.includes(name)),
  };
}
