package router

import "testing"

func TestAdminSavingsBreakdownChartsExposeSavingsSeries(t *testing.T) {
	baseline := 2.50
	savings := -0.75
	savingsPct := -30.0
	rows := []adminScalarReportRow{{
		Key:             "alice",
		Requests:        3,
		CostUSD:         3.25,
		TotalCostUSD:    3.25,
		BaselineCostUSD: &baseline,
		SavingsUSD:      &savings,
		SavingsPct:      &savingsPct,
	}}
	for _, endpoint := range []string{
		"/api/savings-by-user",
		"/api/savings-by-key",
		"/api/savings-by-group",
		"/api/savings-by-project",
		"/api/savings-by-provider-model",
	} {
		t.Run(endpoint, func(t *testing.T) {
			spec, ok := adminScalarEndpointSpecs(endpoint)
			if !ok {
				t.Fatalf("missing spec for %s", endpoint)
			}
			charts := adminScalarCharts(adminReportFilters{}, "2026-06-30T00:00:00Z", spec, rows)
			if len(charts) != 4 {
				t.Fatalf("charts=%d: %#v", len(charts), charts)
			}
			cost := chartByID(t, charts, spec.Report+"_cost")
			assertChartSeriesNames(t, cost, "Actual cost", "Baseline cost")
			if cost.Title != "Actual vs baseline cost" || cost.YAxis.Unit != "usd" {
				t.Fatalf("cost chart=%#v", cost)
			}
			savingsChart := chartByID(t, charts, spec.Report+"_savings")
			assertChartSeriesNames(t, savingsChart, "Savings")
			if got := savingsChart.Series[0].Points[0].Y; got != savings {
				t.Fatalf("savings point=%v, want %v", got, savings)
			}
			rate := chartByID(t, charts, spec.Report+"_savings_pct")
			assertChartSeriesNames(t, rate, "Savings rate")
			if rate.YAxis.Unit != "percent" {
				t.Fatalf("savings pct unit=%q", rate.YAxis.Unit)
			}
			if got := rate.Series[0].Points[0].Y; got != savingsPct {
				t.Fatalf("savings pct point=%v, want %v", got, savingsPct)
			}
		})
	}
}

func TestAdminScalarChartsDoNotAddSavingsSeriesToCostReports(t *testing.T) {
	spec, ok := adminScalarEndpointSpecs("/api/provider-model-mix")
	if !ok {
		t.Fatal("missing provider-model-mix spec")
	}
	rows := []adminScalarReportRow{{Key: "provider/model", Requests: 1, CostUSD: 1.25, TotalCostUSD: 1.25}}
	charts := adminScalarCharts(adminReportFilters{}, "2026-06-30T00:00:00Z", spec, rows)
	if len(charts) != 2 {
		t.Fatalf("charts=%d: %#v", len(charts), charts)
	}
	for _, chart := range charts {
		for _, series := range chart.Series {
			switch series.Name {
			case "Actual cost", "Baseline cost", "Savings", "Savings rate":
				t.Fatalf("non-savings report gained savings series: %#v", chart)
			}
		}
	}
}

func chartByID(t *testing.T, charts []adminReportChart, id string) adminReportChart {
	t.Helper()
	for _, chart := range charts {
		if chart.ChartID == id {
			return chart
		}
	}
	t.Fatalf("missing chart %s in %#v", id, charts)
	return adminReportChart{}
}

func assertChartSeriesNames(t *testing.T, chart adminReportChart, names ...string) {
	t.Helper()
	if len(chart.Series) != len(names) {
		t.Fatalf("series count=%d want %d in %#v", len(chart.Series), len(names), chart)
	}
	for i, name := range names {
		if chart.Series[i].Name != name {
			t.Fatalf("series[%d]=%q want %q in %#v", i, chart.Series[i].Name, name, chart)
		}
	}
}
