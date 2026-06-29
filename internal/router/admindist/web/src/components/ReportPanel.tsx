import type { Dispatch, SetStateAction } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DataTable } from "@/components/DataTable";
import { MetricGrid } from "@/components/MetricGrid";
import { ReportCharts } from "@/components/ReportCharts";
import { TabFilterPanel } from "@/components/TabFilterPanel";
import { Button } from "@/components/ui/button";
import type { ReportFilters, ReportPageAction, ReportResponse, TabSpec } from "@/lib/reports";
import { columnsForTab, resolveSortKey, rowsForTab } from "@/lib/reports";

type Props = {
  tab: TabSpec;
  report?: ReportResponse;
  filters: ReportFilters;
  pageIndex: number | null;
  canGoBack: boolean;
  loading: boolean;
  error?: string;
  onFiltersChange: Dispatch<SetStateAction<ReportFilters>>;
  onPageChange: (action: ReportPageAction, cursor?: string) => void;
  onRefresh: () => void;
};

export function ReportPanel({ tab, report, filters, pageIndex, canGoBack, loading, error, onFiltersChange, onPageChange, onRefresh }: Props) {
  const rows = report ? rowsForTab(tab, report) : [];
  const columns = columnsForTab(tab, rows);
  const supportedSortKeys = supportedSortKeysForTab(tab);
  const sortKey = resolveSortKey(report?.pagination?.sort || filters.sort, rows, columns);
  const sortDir = (report?.pagination?.direction || filters.direction) === "asc" ? "asc" : "desc";
  return (
    <main className="space-y-4">
      <div>
        <p className="font-mono text-xs uppercase text-metrum-red">{tab.endpoint}</p>
        <h2 className="font-display text-2xl text-white">{tab.label}</h2>
        <p className="text-sm text-white/58">
          {report?.period?.from && report?.period?.to ? `${report.period.from} to ${report.period.to}` : report?.generatedUtc ? `Generated ${report.generatedUtc}` : "Current report"}
        </p>
      </div>
      {error ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-metrum-red/40 bg-metrum-red/10 p-4 text-sm text-white">
          <span>{error}</span>
          {filters.cursor ? (
            <Button type="button" variant="outline" onClick={() => onPageChange("first")}>
              Reset to first page
            </Button>
          ) : null}
        </div>
      ) : null}
      {loading ? <div className="rounded-lg border border-white/10 p-6 text-sm text-white/62">Loading report data...</div> : null}
      <TabFilterPanel tab={tab} columns={columns} supportedSortKeys={supportedSortKeys} filters={filters} onFiltersChange={onFiltersChange} />
      <MetricGrid summary={report?.summary} />
      {tab.id === "savings" && report?.warnings?.length ? (
        <Card className="border-metrum-red/30 bg-metrum-red/10">
          <CardHeader>
            <CardTitle>Warnings</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="list-disc space-y-1 pl-5 text-sm text-white/75">
              {report.warnings.map((warning) => (
                <li key={warning}>{warning}</li>
              ))}
            </ul>
          </CardContent>
        </Card>
      ) : null}
      <ReportCharts charts={report?.charts} />
      <DataTable
        rows={rows}
        columns={columns}
        initialSortKey={sortKey}
        initialSortDir={sortDir}
        limit={filters.limit}
        onLimitChange={(limit) => onFiltersChange((current) => ({ ...current, limit }))}
        pagination={report?.pagination}
        supportedSortKeys={supportedSortKeys}
        pageIndex={pageIndex}
        canGoBack={canGoBack}
        loading={loading}
        onPageChange={onPageChange}
        onSortChange={(sort, direction) => onFiltersChange((current) => ({ ...current, sort, direction }))}
        onRefresh={onRefresh}
      />
    </main>
  );
}

function supportedSortKeysForTab(tab: TabSpec): ReadonlySet<string> | undefined {
  if (tab.security) return new Set(["timeUtc", "status", "outcome", "surface", "reason"]);
  if (tab.requests) return new Set(["timeUtc", "costUsd", "totalCostUsd", "latencyMs", "status", "requestId"]);
  if (!tab.filters?.includes("sort")) return undefined;
  return new Set([
    "key",
    "requests",
    "costUsd",
    "totalCostUsd",
    "tokens",
    "totalTokens",
    "latencyMs",
    "avgLatencyMs",
    "errors",
    "fallbacks",
    "savingsUsd",
    "inputImageCount",
  ]);
}
