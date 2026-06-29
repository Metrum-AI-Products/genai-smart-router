import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, test } from "vitest";
import { filterFields, globalFilterFields, paginationFilterFields, tabFilterFields } from "./filters";
import { columnsForTab, reportMetadataById, resolveSortKey, tabSpecs, type ReportColumn } from "./reports";

function names(fields: ReadonlyArray<readonly [string, string, string]>) {
  return fields.map(([name]) => name);
}

describe("admin report filters", () => {
  test("keeps the compatibility filter list as global filters followed by tab and pagination filters", () => {
    expect(filterFields).toEqual([...globalFilterFields, ...tabFilterFields, ...paginationFilterFields]);
  });

  test("does not duplicate keys between global and tab filters", () => {
    const globalNames = new Set(names(globalFilterFields));
    const tabNames = new Set(names(tabFilterFields));
    expect([...globalNames].filter((name) => tabNames.has(name))).toEqual([]);
    expect(new Set(names(filterFields)).size).toBe(filterFields.length);
  });

  test("keeps every global filter accepted by the Go report parser", () => {
    const parserSource = readFileSync(resolve(__dirname, "../../../../admin_reports.go"), "utf8");
    const accepted = new Set<string>(["since"]);
    for (const match of parserSource.matchAll(/q\.Get\("([^"]+)"\)/g)) {
      accepted.add(match[1]);
    }
    for (const [name] of globalFilterFields) {
      expect(accepted.has(name), `${name} must be accepted by parseAdminReportFilters`).toBe(true);
    }
  });
});

describe("admin report help metadata", () => {
  test("defines complete help metadata for every report tab", () => {
    for (const tab of tabSpecs) {
      expect(tab.metadata.shortDescription.trim(), tab.id).not.toBe("");
      expect(tab.metadata.purpose.trim(), tab.id).not.toBe("");
      expect(tab.metadata.dataSemantics.trim(), tab.id).not.toBe("");
      expect(tab.metadata.caveats.trim(), tab.id).not.toBe("");
      expect(tab.metadata.emptyState.trim(), tab.id).not.toBe("");
      expect(tab.metadata.docsPath, tab.id).toContain(`#${tab.id}`);
      expect(tab.metadata.commonFilters.length, tab.id).toBeGreaterThan(0);
      expect(tab.metadata.keyColumns.length, tab.id).toBeGreaterThan(0);
      expect(tab.metadata.relatedReports.length, tab.id).toBeGreaterThan(0);
    }
    expect(Object.keys(reportMetadataById).sort()).toEqual(tabSpecs.map((tab) => tab.id).sort());
  });

  test("adds column descriptions to tab schemas", () => {
    for (const tab of tabSpecs) {
      const columns = columnsForTab(
        tab,
        Object.fromEntries((tab.columns || []).map((column) => [column.key, column.key])) ? [Object.fromEntries((tab.columns || []).map((column) => [column.key, column.key]))] : [],
      );
      for (const column of columns) {
        expect(column.description?.trim(), `${tab.id}.${column.key}`).not.toBe("");
      }
    }
  });
});

describe("admin report sort state", () => {
  test("maps backend cost sort aliases to the visible total-cost column", () => {
    const columns: ReportColumn[] = [{ key: "totalCostUsd", label: "Total cost" }];
    const rows = [{ costUsd: 1.23, totalCostUsd: 1.23 }];
    expect(resolveSortKey("costUsd", rows, columns)).toBe("totalCostUsd");
  });
});
