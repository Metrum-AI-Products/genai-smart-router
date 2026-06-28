import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DataTable } from "@/components/DataTable";
import { MetricGrid } from "@/components/MetricGrid";
import type { ReportResponse, TabSpec } from "@/lib/reports";
import { rowsForTab } from "@/lib/reports";

type Props = {
  tab: TabSpec;
  report?: ReportResponse;
  loading: boolean;
  error?: string;
  onRefresh: () => void;
};

export function ReportPanel({ tab, report, loading, error, onRefresh }: Props) {
  const rows = report ? rowsForTab(tab, report) : [];
  return (
    <main className="space-y-4">
      <div>
        <p className="font-mono text-xs uppercase text-metrum-red">{tab.endpoint}</p>
        <h2 className="font-display text-2xl text-white">{tab.label}</h2>
        <p className="text-sm text-white/58">
          {report?.period?.from && report?.period?.to ? `${report.period.from} to ${report.period.to}` : report?.generatedUtc ? `Generated ${report.generatedUtc}` : "Current report"}
        </p>
      </div>
      {error ? <div className="rounded-lg border border-metrum-red/40 bg-metrum-red/10 p-4 text-sm text-white">{error}</div> : null}
      {loading ? <div className="rounded-lg border border-white/10 p-6 text-sm text-white/62">Loading report data...</div> : null}
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
      <DataTable rows={rows} onRefresh={onRefresh} />
    </main>
  );
}
