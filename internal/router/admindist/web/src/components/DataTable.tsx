import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type { ReportRow } from "@/lib/reports";
import { formatValue, titleize } from "@/lib/utils";

type Props = {
  rows: ReportRow[];
  onRefresh?: () => void;
};

const preferredColumns = [
  "key",
  "secondaryKey",
  "requestId",
  "timeUtc",
  "provider",
  "model",
  "requests",
  "errors",
  "tokens",
  "totalTokens",
  "totalCostUsd",
  "costUsd",
  "savingsUsd",
  "savingsPct",
  "avgLatencyMs",
  "avgTtfbMs",
  "fallbacks",
  "status",
  "error",
];

export function DataTable({ rows, onRefresh }: Props) {
  const [search, setSearch] = useState("");
  const [pageSize, setPageSize] = useState(50);
  const [sortKey, setSortKey] = useState("");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");

  const columns = useMemo(() => {
    const seen = new Set<string>();
    for (const row of rows) {
      for (const key of Object.keys(row)) seen.add(key);
    }
    return Array.from(seen).sort((a, b) => {
      const ai = preferredColumns.indexOf(a);
      const bi = preferredColumns.indexOf(b);
      if (ai === -1 && bi === -1) return a.localeCompare(b);
      if (ai === -1) return 1;
      if (bi === -1) return -1;
      return ai - bi;
    });
  }, [rows]);

  const visibleRows = useMemo(() => {
    const needle = search.trim().toLowerCase();
    let out = needle
      ? rows.filter((row) => Object.values(row).some((value) => String(value ?? "").toLowerCase().includes(needle)))
      : rows;
    if (sortKey) {
      out = [...out].sort((a, b) => {
        const av = a[sortKey];
        const bv = b[sortKey];
        const an = typeof av === "number" ? av : Number.NaN;
        const bn = typeof bv === "number" ? bv : Number.NaN;
        const cmp = Number.isFinite(an) && Number.isFinite(bn) ? an - bn : String(av ?? "").localeCompare(String(bv ?? ""));
        return sortDir === "asc" ? cmp : -cmp;
      });
    }
    return out.slice(0, pageSize);
  }, [pageSize, rows, search, sortDir, sortKey]);

  function sort(column: string) {
    if (sortKey === column) {
      setSortDir((dir) => (dir === "asc" ? "desc" : "asc"));
      return;
    }
    setSortKey(column);
    setSortDir("desc");
  }

  function exportCsv() {
    const header = columns.join(",");
    const body = visibleRows
      .map((row) => columns.map((column) => JSON.stringify(row[column] ?? "")).join(","))
      .join("\n");
    const blob = new Blob([header, "\n", body], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "admin-report.csv";
    a.click();
    URL.revokeObjectURL(url);
  }

  function renderCell(row: ReportRow, column: string) {
    const value = row[column];
    const formatted = formatValue(value, column);
    if ((column === "requestId" || column === "request_id") && value) {
      return (
        <a className="font-mono text-metrum-pink underline decoration-metrum-pink/40 underline-offset-4 hover:text-white" href={`api/request/${encodeURIComponent(String(value))}`}>
          {formatted}
        </a>
      );
    }
    return formatted;
  }

  if (columns.length === 0) {
    return <div className="rounded-lg border border-white/10 p-6 text-sm text-white/62">No rows for the selected filters.</div>;
  }

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Input className="max-w-xs" placeholder="Search visible rows" value={search} onChange={(event) => setSearch(event.target.value)} />
        <Select value={String(pageSize)} onChange={(event) => setPageSize(Number(event.target.value))}>
          <option value="25">25 rows</option>
          <option value="50">50 rows</option>
          <option value="100">100 rows</option>
        </Select>
        <Button type="button" variant="outline" onClick={onRefresh}>
          Refresh
        </Button>
        <Button type="button" variant="outline" onClick={exportCsv}>
          CSV
        </Button>
      </div>
      <div className="overflow-auto rounded-lg border border-white/10">
        <table className="w-full min-w-[920px] border-collapse text-left text-sm">
          <thead className="bg-white/[0.06] text-xs uppercase text-white/60">
            <tr>
              {columns.map((column) => (
                <th key={column} className="whitespace-nowrap px-3 py-2">
                  <button type="button" className="text-left hover:text-white" onClick={() => sort(column)}>
                    {titleize(column)}
                  </button>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {visibleRows.map((row, index) => (
              <tr key={index} className="border-t border-white/10 odd:bg-white/[0.025]">
                {columns.map((column) => (
                  <td key={column} className="max-w-[360px] truncate px-3 py-2 text-white/82" title={formatValue(row[column], column)}>
                    {renderCell(row, column)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-white/45">
        Showing {visibleRows.length} of {rows.length} rows.
      </p>
    </section>
  );
}
