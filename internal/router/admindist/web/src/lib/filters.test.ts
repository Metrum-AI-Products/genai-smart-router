import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, test } from "vitest";
import { filterFields, globalFilterFields, tabFilterFields } from "./filters";

function names(fields: ReadonlyArray<readonly [string, string, string]>) {
  return fields.map(([name]) => name);
}

describe("admin report filters", () => {
  test("keeps the compatibility filter list as global filters followed by tab filters", () => {
    expect(filterFields).toEqual([...globalFilterFields, ...tabFilterFields]);
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

