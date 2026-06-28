import { Card, CardContent } from "@/components/ui/card";
import { formatValue, titleize } from "@/lib/utils";
import type { ReportSummary } from "@/lib/reports";

const priority = ["requests", "errors", "totalTokens", "tokens", "totalCostUsd", "costUsd", "avgLatencyMs", "fallbacks"];

export function MetricGrid({ summary }: { summary?: ReportSummary }) {
  if (!summary) return null;
  const entries = Object.entries(summary)
    .filter(([, value]) => typeof value === "number" || typeof value === "string" || typeof value === "boolean")
    .sort(([a], [b]) => {
      const ai = priority.indexOf(a);
      const bi = priority.indexOf(b);
      if (ai === -1 && bi === -1) return a.localeCompare(b);
      if (ai === -1) return 1;
      if (bi === -1) return -1;
      return ai - bi;
    })
    .slice(0, 8);
  if (entries.length === 0) return null;
  return (
    <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      {entries.map(([key, value]) => (
        <Card key={key} className="panel-gradient">
          <CardContent className="p-4">
            <div className="font-display text-2xl text-white">{formatValue(value, key)}</div>
            <div className="mt-1 font-mono text-xs uppercase text-white/58">{titleize(key)}</div>
          </CardContent>
        </Card>
      ))}
    </section>
  );
}
