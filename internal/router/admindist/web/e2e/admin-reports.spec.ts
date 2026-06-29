import { expect, test, type Page, type Route } from "@playwright/test";
import { navGroups, validateNavGroups } from "../src/lib/navGroups";
import { tabSpecs, type ReportChart, type ReportRow } from "../src/lib/reports";

const generatedUtc = "2026-06-28T12:00:00Z";
let apiRequests: string[] = [];

test.beforeEach(async ({ page }) => {
  apiRequests = [];
  await installAdminApiMocks(page, apiRequests);
});

test("admin report nav groups cover every tab exactly once", async () => {
  const result = validateNavGroups(navGroups, tabSpecs);
  expect(result.duplicateTabs).toEqual([]);
  expect(result.missingTabs).toEqual([]);
  expect(result.unknownTabs).toEqual([]);
  expect(result.uniqueGroupIds).toBe(true);
  expect(result.nonEmptyLabels).toBe(true);
  expect(navGroups.map((group) => group.id)).toEqual([
    "overview",
    "usage",
    "savings",
    "performance",
    "traffic-shaping",
    "routing-decisions",
    "provider-catalog",
    "security",
    "request-drilldown",
    "system-status",
  ]);
});

test("admin reports shell renders every tab with mocked report APIs", async ({ page }) => {
  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });

  await page.goto("/");

  await expect(page.getByRole("heading", { name: "Admin Reports" })).toBeVisible();
  await expect(page.getByText("v2026.e2e")).toBeVisible();
  await expect(page.getByText("Commit e2ecommit123")).toBeVisible();

  const reportNav = page.getByRole("navigation", { name: "Report sections" });
  await expect(reportNav.locator("[data-report-tab]")).toHaveCount(tabSpecs.length);
  const horizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
  expect(horizontalOverflow).toBe(false);

  for (const spec of tabSpecs) {
    await reportNav.locator(`[data-report-tab="${spec.id}"]`).click();
    await expect(page.getByRole("heading", { name: spec.label, exact: true })).toBeVisible();
    await expect(page.locator("main canvas, main table").first()).toBeVisible();
  }

  expect(consoleErrors).toEqual([]);
});

test("sidebar groups collapse and expand on click", async ({ page }) => {
  await page.goto("/");

  const reportNav = page.getByRole("navigation", { name: "Report sections" });
  for (const group of navGroups) {
    await expect(reportNav.locator(`[data-nav-group="${group.id}"] > button`).first()).toBeVisible();
  }

  await reportNav.getByRole("button", { name: "Usage", exact: true }).click();
  await expect(reportNav.getByRole("button", { name: "Groups", exact: true })).toBeHidden();

  await reportNav.getByRole("button", { name: "Usage", exact: true }).click();
  await expect(reportNav.getByRole("button", { name: "Groups", exact: true })).toBeVisible();

  await reportNav.getByRole("button", { name: "Usage", exact: true }).click();
  await page.reload();
  const reloadedNav = page.getByRole("navigation", { name: "Report sections" });
  await expect(reloadedNav.getByRole("button", { name: "Groups", exact: true })).toBeHidden();
});

test("sidebar keyboard navigation reaches every group", async ({ page }) => {
  await page.goto("/");

  const reportNav = page.getByRole("navigation", { name: "Report sections" });
  await reportNav.locator('[data-nav-group="overview"] button').first().focus();

  const reachedGroups = new Set<string>();
  for (let index = 0; index < tabSpecs.length + navGroups.length + 8; index += 1) {
    const groupId = await page.evaluate(() => {
      const focused = document.activeElement;
      if (!(focused instanceof HTMLElement) || !focused.hasAttribute("data-report-tab")) return "";
      return focused.closest("[data-nav-group]")?.getAttribute("data-nav-group") || "";
    });
    if (groupId) reachedGroups.add(groupId);
    await page.keyboard.press("Tab");
  }

  expect(Array.from(reachedGroups).sort()).toEqual(navGroups.map((group) => group.id).sort());
});

test("mobile viewport shows drawer toggle and hidden sidebar", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 800 });
  await page.goto("/");

  await expect(page.locator("[data-report-sidebar]:visible")).toHaveCount(0);
  await page.getByRole("button", { name: "Open report navigation" }).click();

  const reportNav = page.getByRole("navigation", { name: "Report sections" });
  await expect(reportNav).toBeVisible();
  await expect(reportNav.locator("[data-report-tab]")).toHaveCount(tabSpecs.length);
  await reportNav.getByRole("button", { name: "Requests", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Requests", exact: true })).toBeVisible();
  await expect(page.locator("[data-report-sidebar]:visible")).toHaveCount(0);
});

test("deep link requests tab opens active sidebar item", async ({ page }) => {
  await page.goto("/?tab=requests");

  const reportNav = page.getByRole("navigation", { name: "Report sections" });
  const requestsTab = reportNav.getByRole("button", { name: "Requests", exact: true });
  await expect(requestsTab).toHaveAttribute("aria-current", "page");
  await expect(page.getByRole("heading", { name: "Requests", exact: true })).toBeVisible();
});

test("filter URL state and CSV export remain usable", async ({ page }) => {
  await page.goto("/");

  await page.getByLabel("Caller").fill("alice");
  await page.getByLabel("IP").fill("203.0.113.10");
  await page.getByLabel("Status").fill("200");
  await page.getByLabel("Client").fill("codex-cli");
  await page.getByRole("button", { name: "Apply" }).click();

  await expect(page).toHaveURL(/caller_id=alice/);
  await expect(page).toHaveURL(/caller_ip=203\.0\.113\.10/);
  await expect(page).toHaveURL(/status=200/);
  await expect(page).toHaveURL(/client=codex-cli/);

  await page.getByPlaceholder("Search visible rows").fill("mock");
  await expect(page.locator("tbody tr")).toHaveCount(1);

  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "CSV" }).click();
  await expect((await download).suggestedFilename()).toBe("admin-report.csv");
});

test("deep-linked savings URLs preserve baseline and sorting params", async ({ page }) => {
  await page.goto("/?tab=savings-by-key&since=6d&limit=50&baseline=gpt-5.5&sort=savingsUsd&direction=desc");

  await expect(page.getByRole("heading", { name: "Savings by key", exact: true })).toBeVisible();
  await expect(page.getByLabel("Baseline")).toHaveValue("gpt-5.5");
  await expect(page.getByLabel("Sort")).toHaveValue("savingsUsd");
  await expect(page.getByLabel("Direction")).toHaveValue("desc");
  await expect(page.getByRole("columnheader", { name: "Savings(USD)" })).toBeVisible();
  await expect(page.locator("tbody tr").first()).toContainText("rtr_mock_public");
  await expect(page.locator("main canvas").first()).toBeVisible();

  const savingsRequest = apiRequests.find((url) => url.includes("/api/savings-by-key?"));
  expect(savingsRequest).toContain("baseline=gpt-5.5");
  expect(savingsRequest).toContain("sort=savingsUsd");
  expect(savingsRequest).toContain("direction=desc");
});

async function installAdminApiMocks(page: Page, requestedUrls?: string[]) {
  await page.route("**/api/**", async (route) => {
    requestedUrls?.push(route.request().url());
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(responseForRoute(route)),
    });
  });
}

function responseForRoute(route: Route) {
  const url = new URL(route.request().url());
  const endpoint = url.pathname.split("/api/")[1] || "";
  if (endpoint === "version") {
    return {
      version: "2026.e2e",
      commit: "e2ecommit123456",
      build_date: generatedUtc,
      go_version: "go1.26.0",
      goos: "linux",
      goarch: "amd64",
      license_compile_mode: "release",
    };
  }
  if (endpoint === "provider-catalog-status") {
    return commonResponse("provider-catalog-status", {
      rows: [
        {
          source: "active_target",
          provider: "mock",
          model: "mock-model",
          dialect: "openai",
          activeGroups: ["default"],
          activeTargetCount: 1,
          validationStatus: "passed",
          pricingMissing: false,
        },
      ],
    });
  }
  if (endpoint === "retention-status") {
    return {
      generatedUtc,
      enabled: true,
      dryRun: true,
      tables: [{ dataClass: "usage_detail", tableName: "request_usage", status: "dry_run", candidateRows: 2, eligibleRows: 2, heldRows: 0, blockedRows: 0, deletedRows: 0 }],
      rollups: [{ rollupType: "daily", status: "finalized", sourceRequestCount: 2, dailyRows: 1, rollupRows: 1, windowStart: "2026-06-27T00:00:00Z", windowEnd: "2026-06-28T00:00:00Z" }],
      charts: [chart("retention-status")],
    };
  }
  if (endpoint === "summary") {
    return commonResponse("summary", {
      byGroup: [row("default")],
      byProvider: [row("mock/mock-model")],
      byToken: [row("rtr_mock_public")],
      byStatus: [row("200")],
      requests: [requestRow()],
    });
  }
  if (endpoint === "security/events") {
    return commonResponse("security/events", {
      rows: [
        {
          timeUtc: generatedUtc,
          eventType: "admin_report",
          surface: "admin_reports",
          method: "GET",
          path: "/admin/reports/api/summary",
          status: 200,
          outcome: "allowed",
          reason: "authorized",
          authSubject: "basic:admin",
          authSource: "basic",
        },
      ],
    });
  }
  if (endpoint === "savings") {
    return {
      ...commonResponse("savings", {
        byTime: [savingsRow("2026-06-28T12:00:00Z")],
        byGroup: [savingsRow("default")],
      }),
      baseline: {
        baseline_id: "gpt-5.5",
        baseline_name: "GPT-5.5",
        pricing_source: "https://example.invalid/pricing",
        pricing_updated_at: "2026-06-25",
      },
      baselines: [],
    };
  }
  if (endpoint === "savings-by-key") {
    return commonResponse(endpoint, {
      baseline: {
        baseline_id: "gpt-5.5",
        baseline_name: "GPT-5.5",
      },
      rows: [
        {
          ...row("rtr_mock_public"),
          baselineCostUsd: 0.25,
          savingsUsd: 0.2,
          savingsPct: 80,
        },
      ],
    });
  }
  if (endpoint === "expensive-requests") {
    return commonResponse(endpoint, { requests: [requestRow()] });
  }
  return commonResponse(endpoint, { rows: [row(endpoint)] });
}

function commonResponse(report: string, extra: Record<string, unknown>) {
  return {
    period: { from: "2026-06-28T11:00:00Z", to: generatedUtc },
    generatedUtc,
    report,
    summary: {
      requests: 2,
      errors: 0,
      totalTokens: 345,
      inputTokens: 123,
      outputTokens: 222,
      totalCostUsd: 0.0123,
      avgLatencyMs: 245,
    },
    charts: [chart(report)],
    ...extra,
  };
}

function row(key: string): ReportRow {
  return {
    key,
    secondaryKey: "mock",
    requests: 2,
    attempts: 2,
    streams: 1,
    errors: 0,
    errorRatePct: 0,
    fallbacks: 0,
    fallbackRatePct: 0,
    inputTokens: 123,
    outputTokens: 222,
    totalTokens: 345,
    inputCostUsd: 0.001,
    outputCostUsd: 0.0113,
    totalCostUsd: 0.0123,
    avgLatencyMs: 245,
    avgUpstreamTokensPerSec: 31.5,
  };
}

function savingsRow(key: string): ReportRow {
  return {
    key,
    requests: 2,
    input_tokens: 123,
    output_tokens: 222,
    total_tokens: 345,
    actual_cost_usd: 0.0123,
    baseline_cost_usd: 0.015,
    savings_usd: 0.0027,
    savings_pct: 18,
  };
}

function requestRow(): ReportRow {
  return {
    timeUtc: generatedUtc,
    requestId: "req_e2e_123",
    callerId: "alice",
    callerIp: "203.0.113.10",
    tokenId: "rtr_mock_public",
    client: "codex-cli",
    requestedModel: "default",
    modelGroup: "default",
    provider: "mock",
    model: "mock-model",
    dialect: "openai",
    status: 200,
    cache: "miss",
    attempts: 1,
    fallback: false,
    inputTokens: 123,
    outputTokens: 222,
    totalTokens: 345,
    totalCostUsd: 0.0123,
    latencyMs: 245,
  };
}

function chart(id: string): ReportChart {
  return {
    chart_id: `${id}-requests`,
    title: `${id} requests`,
    x_axis: { label: "Time", type: "time" },
    y_axis: { label: "Requests", unit: "requests" },
    series: [
      {
        name: "Requests",
        unit: "requests",
        color_key: "magenta",
        points: [
          { x: "2026-06-28T11:00:00Z", y: 1 },
          { x: generatedUtc, y: 2 },
        ],
      },
    ],
  };
}
