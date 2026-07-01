import { useState } from "react";
import { Bar, Line } from "react-chartjs-2";
import type { ChartData, ChartOptions } from "chart.js";
import type { ReportChart, ReportChartAxis, ReportChartPoint, ReportChartSeries } from "@/lib/reports";
import { buildShortLabelMap, type ShortLabelMap } from "@/lib/shortLabels";
import { formatUtcTimestamp, pickTimeUnit, timeRange, type TimeRange } from "@/lib/timeAxis";
import { compactFmt, usdCompactFmt } from "@/lib/utils";

type Props = {
  charts?: ReportChart[];
};

const palette: Record<string, string> = {
  blue: "#465cda",
  magenta: "#ee0089",
  pink: "#fe005f",
  purple: "#cc28af",
  red: "#ff3132",
  success: "#22c55e",
  text: "#c7c7d1",
  violet: "#9948cb",
  warning: "#f59e0b",
};

export function ReportCharts({ charts }: Props) {
  const normalized = (charts || []).map(normalizeChart).filter((chart) => chart.series.some((series) => series.points.length > 0));
  if (normalized.length === 0) {
    return <div className="rounded-lg border border-white/10 p-6 text-sm text-white/62">No chart data for the selected filters.</div>;
  }
  return (
    <section className="grid gap-4 xl:grid-cols-2">
      {normalized.map((chart) => (
        <div key={chart.id} className="rounded-lg border border-white/10 bg-white/[0.035] p-4">
          <div className="mb-3">
            <h3 className="font-display text-lg text-white">{chart.title}</h3>
            <p className="text-xs text-white/48">
              {chart.xAxis.label || "Bucket"} by {chart.yAxis.label || chart.yAxis.unit || "value"}
            </p>
          </div>
          <div className="h-72">
            {chart.kind === "line" ? (
              <Line data={chartData(chart) as ChartData<"line">} options={chartOptions(chart) as ChartOptions<"line">} />
            ) : (
              <Bar data={chartData(chart) as ChartData<"bar">} options={chartOptions(chart) as ChartOptions<"bar">} />
            )}
          </div>
          <ChartLegend entries={chart.legendEntries} />
        </div>
      ))}
    </section>
  );
}

type NormalizedChart = {
  id: string;
  title: string;
  kind: "bar" | "line";
  xAxis: ReportChartAxis;
  yAxis: ReportChartAxis;
  labels: string[];
  displayLabels: string[];
  legendEntries: ShortLabelMap[];
  hasNumericTimeAxis: boolean;
  timeRange: TimeRange | null;
  series: Array<Required<Pick<ReportChartSeries, "name" | "points">> & { unit?: string; colorKey?: string }>;
};

function normalizeChart(chart: ReportChart): NormalizedChart {
  const xAxis = chart.x_axis || chart.xAxis || {};
  const yAxis = chart.y_axis || chart.yAxis || { unit: chart.unit };
  const rawSeries = Array.isArray(chart.series) ? chart.series : [];
  const series = rawSeries.length > 0 && isBackendSeries(rawSeries[0])
    ? (rawSeries as ReportChartSeries[]).map((item, index) => ({
        name: item.name || `Series ${index + 1}`,
        unit: item.unit,
        colorKey: item.color_key || item.colorKey,
        points: item.points || [],
      }))
    : [
        {
          name: chart.title || "Value",
          unit: chart.unit,
          colorKey: "magenta",
          points: legacyPoints(rawSeries as ReportChartPoint[], chart.points),
        },
      ];
  const kind = chart.kind === "line" || xAxis.type === "time" ? "line" : "bar";
  const hasNumericTimeAxis = kind === "line" && series.every((item) => item.points.every((point) => typeof point.x_unix_ms === "number" && Number.isFinite(point.x_unix_ms)));
  const labels = Array.from(new Set(series.flatMap((item) => item.points.map((point) => point.x || ""))));
  const legendEntries = kind === "bar" ? buildShortLabelMap(labels) : [];
  const fullToShort = new Map(legendEntries.map((entry) => [entry.full, entry.short]));
  const range = hasNumericTimeAxis ? timeRange(series.flatMap((item) => item.points.map((point) => point.x_unix_ms as number))) : null;
  return {
    id: chart.chart_id || chart.chartId || chart.id || chart.title || "chart",
    title: chart.title || "Report chart",
    kind,
    xAxis,
    yAxis,
    labels,
    displayLabels: kind === "bar" ? labels.map((label) => fullToShort.get(label) || label) : labels,
    legendEntries,
    hasNumericTimeAxis,
    timeRange: range,
    series,
  };
}

function isBackendSeries(value: ReportChartPoint | ReportChartSeries): value is ReportChartSeries {
  return "points" in value || "color_key" in value || "colorKey" in value || "name" in value;
}

function legacyPoints(series?: ReportChartPoint[], points?: ReportChartPoint[]): ReportChartPoint[] {
  const input = points?.length ? points : series || [];
  return input.map((point) => ({ x: point.x || point.time || point.label || "", x_unix_ms: point.x_unix_ms, y: point.y ?? point.value ?? 0 }));
}

function chartData(chart: NormalizedChart): ChartData<"bar" | "line"> {
  return {
    labels: chart.hasNumericTimeAxis ? undefined : chart.displayLabels,
    datasets: chart.series.map((series, index) => {
      const color = palette[series.colorKey || ""] || Object.values(palette)[index % Object.keys(palette).length];
      const valuesByLabel = new Map(series.points.map((point) => [point.x || "", point.y ?? point.value ?? 0]));
      const data = chart.hasNumericTimeAxis
        ? series.points.map((point) => ({ x: point.x_unix_ms as number, y: point.y ?? point.value ?? 0 })).sort((left, right) => left.x - right.x)
        : chart.labels.map((label) => valuesByLabel.get(label) || 0);
      return {
        label: series.name,
        data,
        borderColor: color,
        backgroundColor: withAlpha(color, chart.kind === "line" ? 0.18 : 0.7),
        borderWidth: 2,
        tension: 0.28,
        fill: chart.kind === "line",
      };
    }),
  };
}

function chartOptions(chart: NormalizedChart): ChartOptions<"bar" | "line"> {
  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { intersect: false, mode: "index" },
    plugins: {
      legend: { labels: { color: "rgba(255,255,255,0.72)", boxWidth: 12, boxHeight: 12 } },
      tooltip: {
        callbacks: {
          title: (items) => {
            const item = items[0];
            if (!item) return "";
            if (chart.hasNumericTimeAxis) return formatUtcTimestamp(item.parsed.x ?? Number.NaN);
            return chart.labels[item.dataIndex] || item.label || "";
          },
          label: (item) => `${item.dataset.label}: ${formatChartNumber(chart.hasNumericTimeAxis ? item.parsed.y ?? 0 : Number(item.raw || 0), chart.yAxis.unit)}`,
        },
      },
    },
    scales: {
      x: {
        type: chart.hasNumericTimeAxis ? "time" : "category",
					time: chart.hasNumericTimeAxis
						? {
								unit: pickTimeUnit(chart.timeRange),
								displayFormats: { hour: "hour", day: "day", week: "week", month: "month", year: "year" },
								tooltipFormat: "datetime",
							}
						: undefined,
					ticks: {
						color: "rgba(255,255,255,0.58)",
						maxRotation: chart.kind === "bar" || chart.hasNumericTimeAxis ? 0 : 40,
						minRotation: 0,
						autoSkip: chart.kind === "bar" || chart.hasNumericTimeAxis,
						autoSkipPadding: 12,
						maxTicksLimit: chart.kind === "bar" ? 12 : chart.hasNumericTimeAxis ? 10 : undefined,
					},
        grid: { color: "rgba(255,255,255,0.06)" },
      },
      y: {
        beginAtZero: true,
        ticks: {
          color: "rgba(255,255,255,0.58)",
          callback: (value) => formatChartNumber(Number(value), chart.yAxis.unit),
        },
        grid: { color: "rgba(255,255,255,0.08)" },
      },
    },
  };
}

function ChartLegend({ entries }: { entries: ShortLabelMap[] }) {
  const [open, setOpen] = useState(entries.length <= 8);
  if (entries.length === 0) return null;
  return (
    <div className="mt-3 text-[0.7rem] text-white/62" data-chart-bucket-legend>
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        className="mb-2 inline-flex items-center gap-1 rounded border border-white/10 px-2 py-1 text-white/70 hover:bg-white/5 focus:outline-none focus:ring-2 focus:ring-white/30"
        aria-expanded={open}
      >
        {open ? "Hide" : "Show"} bucket legend ({entries.length})
      </button>
      {open && (
        <ul className="grid grid-cols-1 gap-x-4 gap-y-1 sm:grid-cols-2 xl:grid-cols-3">
          {entries.map((entry) => (
            <li key={entry.full} className="flex min-w-0 font-mono" title={entry.full}>
              <span className="mr-2 inline-block min-w-[4ch] shrink-0 text-white/48">{entry.short}</span>
              <span className="truncate align-bottom">{entry.full}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function formatChartNumber(value: number, unit?: string): string {
  if (unit === "usd") return usdCompactFmt.format(value);
  if (unit === "percent") return `${compactFmt.format(value)}%`;
  if (unit === "ms") return `${compactFmt.format(value)} ms`;
  if (unit === "tokens") return `${compactFmt.format(value)} tok`;
  return compactFmt.format(value);
}

function withAlpha(hex: string, alpha: number): string {
  const red = parseInt(hex.slice(1, 3), 16);
  const green = parseInt(hex.slice(3, 5), 16);
  const blue = parseInt(hex.slice(5, 7), 16);
  return `rgba(${red}, ${green}, ${blue}, ${alpha})`;
}
