// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { overviewChartSectionIds } from "@/components/ReportCharts";

describe("overview chart sections", () => {
  it("groups expanded overview charts into scan-friendly sections", () => {
    const sections = overviewChartSectionIds([
      { id: "overview_requests", title: "Requests over time" },
      { id: "overview_cost_savings", title: "Actual cost and savings" },
      { id: "overview_savings_rate", title: "Savings rate" },
      { id: "overview_latency", title: "Latency and TTFB" },
      { id: "overview_throughput", title: "Throughput" },
      { id: "overview_cache", title: "Cache over time" },
      { id: "overview_security_events", title: "Security events" },
      { id: "provider_tokens", title: "Provider token volume" },
    ]);

    expect(sections).toEqual(["health", "value", "performance", "operations", "topn"]);
  });

  it("keeps unknown backend charts visible instead of failing the overview", () => {
    const sections = overviewChartSectionIds([{ id: "custom_chart", title: "Custom deployment chart" }]);

    expect(sections).toEqual(["other"]);
  });
});
