import { useEffect, useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type { ReportColumn, ReportRow } from "@/lib/reports";
import { formatValue } from "@/lib/utils";

type Props = {
  rows: ReportRow[];
  columns: ReportColumn[];
  initialSortKey?: string;
  initialSortDir?: "asc" | "desc";
  limit?: string;
  onLimitChange?: (limit: string) => void;
  onRefresh?: () => void;
};

export function DataTable({ rows, columns, initialSortKey = "", initialSortDir = "desc", limit = "50", onLimitChange, onRefresh }: Props) {
  const [search, setSearch] = useState("");
  const [pageSize, setPageSize] = useState(50);
  const [sortKey, setSortKey] = useState(initialSortKey);
  const [sortDir, setSortDir] = useState<"asc" | "desc">(initialSortDir);

  useEffect(() => {
    setSortKey(initialSortKey);
    setSortDir(initialSortDir);
  }, [initialSortDir, initialSortKey]);

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
    const header = columns.map((column) => csvCell(column.label)).join(",");
    const body = visibleRows
      .map((row) => columns.map((column) => csvCell(row[column.key] ?? "")).join(","))
      .join("\n");
    const blob = new Blob([header, "\n", body], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "admin-report.csv";
    a.click();
    URL.revokeObjectURL(url);
  }

  function csvCell(value: unknown) {
    const text = String(value ?? "");
    const trimmed = text.trimStart();
    const safe = trimmed && ["=", "+", "-", "@"].includes(trimmed[0]) ? `'${text}` : text;
    return JSON.stringify(safe);
  }

  function renderCell(row: ReportRow, column: ReportColumn) {
    const value = row[column.key];
    const formatted = formatValue(value, column.key);
    if ((column.key === "requestId" || column.key === "request_id") && value) {
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
        <label className="grid gap-1 font-mono text-[0.68rem] uppercase text-white/58">
          Rows
          <Select value={limit || "50"} onChange={(event) => onLimitChange?.(event.target.value)}>
            <option value="25">25 rows</option>
            <option value="50">50 rows</option>
            <option value="100">100 rows</option>
            {limit && !["25", "50", "100"].includes(limit) ? <option value={limit}>{limit} rows</option> : null}
          </Select>
        </label>
        <Select aria-label="Page size" value={String(pageSize)} onChange={(event) => setPageSize(Number(event.target.value))}>
          <option value="25">25 visible</option>
          <option value="50">50 visible</option>
          <option value="100">100 visible</option>
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
                <th key={column.key} className="whitespace-nowrap px-3 py-2">
                  <button type="button" className="text-left hover:text-white" onClick={() => sort(column.key)} title={column.description}>
                    {column.label}
                    {column.unit ? <span className="ml-1 normal-case text-white/40">({column.unit})</span> : null}
                  </button>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {visibleRows.map((row, index) => (
              <tr key={index} className="border-t border-white/10 odd:bg-white/[0.025]">
                {columns.map((column) => (
                  <td key={column.key} className="max-w-[360px] truncate px-3 py-2 text-white/82" title={formatValue(row[column.key], column.key)}>
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
