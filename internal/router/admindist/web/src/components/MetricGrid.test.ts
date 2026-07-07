import { describe, expect, test } from "vitest";
import { buildMetricEntries, type MetricGridConfig } from "./MetricGrid";

const savingsMetricConfig: MetricGridConfig = {
  priority: [
    "savingsUsd",
    "savingsPct",
    "actualCostUsd",
    "totalCostUsd",
    "baselineCostUsd",
    "requests",
    "totalTokens",
    "tokens",
    "avgCostUsd",
    "errors",
    "fallbacks",
  ],
  labels: {
    savingsUsd: "Total savings",
    savingsPct: "Savings rate",
    actualCostUsd: "Actual cost",
    totalCostUsd: "Actual cost",
    baselineCostUsd: "Baseline cost",
    avgCostUsd: "Avg cost/request",
  },
  hiddenKeys: ["totalCostUsd"],
  hideZeroKeys: ["errors", "fallbacks"],
  maxItems: 9,
};

describe("MetricGrid", () => {
  test("prioritizes customer-readable savings summary cards", () => {
    const entries = buildMetricEntries(
      {
        requests: 100,
        errors: 2,
        fallbacks: 1,
        avgLatencyMs: 250,
        totalTokens: 12345,
        actualCostUsd: 3.21,
        totalCostUsd: 3.21,
        baselineCostUsd: 9.87,
        savingsUsd: 6.66,
        savingsPct: 67.5,
        avgCostUsd: 0.0321,
      },
      savingsMetricConfig,
    );

    expect(entries.map((entry) => entry.key)).toEqual([
      "savingsUsd",
      "savingsPct",
      "actualCostUsd",
      "baselineCostUsd",
      "requests",
      "totalTokens",
      "avgCostUsd",
      "errors",
      "fallbacks",
    ]);
    expect(entries.map((entry) => entry.label)).toEqual([
      "Total savings",
      "Savings rate",
      "Actual cost",
      "Baseline cost",
      "Requests",
      "Total Tokens",
      "Avg cost/request",
      "Errors",
      "Fallbacks",
    ]);
  });

  test("uses totalCostUsd as the actual cost fallback and hides irrelevant error cards", () => {
    const entries = buildMetricEntries(
      {
        requests: 10,
        errors: 0,
        fallbacks: 0,
        totalTokens: 2000,
        totalCostUsd: 1.23,
        baselineCostUsd: 4.56,
        savingsUsd: 3.33,
        savingsPct: 73.0,
        avgCostUsd: 0.123,
      },
      { ...savingsMetricConfig, hiddenKeys: undefined },
    );

    expect(entries.map((entry) => entry.key)).toEqual([
      "savingsUsd",
      "savingsPct",
      "totalCostUsd",
      "baselineCostUsd",
      "requests",
      "totalTokens",
      "avgCostUsd",
    ]);
    expect(entries.find((entry) => entry.key === "totalCostUsd")?.label).toBe("Actual cost");
    expect(entries.some((entry) => entry.key === "errors")).toBe(false);
    expect(entries.some((entry) => entry.key === "fallbacks")).toBe(false);
  });
});
