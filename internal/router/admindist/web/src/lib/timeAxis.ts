export type TimeUnit = "minute" | "hour" | "day" | "week" | "month" | "year";

export type TimeRange = {
  min: number;
  max: number;
};

const minuteMs = 60 * 1000;
const hourMs = 60 * minuteMs;
const dayMs = 24 * hourMs;

export function timeRange(values: number[]): TimeRange | null {
  const finite = values.filter(Number.isFinite);
  if (finite.length === 0) return null;
  return {
    min: Math.min(...finite),
    max: Math.max(...finite),
  };
}

export function pickTimeUnit(range: TimeRange | null | undefined): TimeUnit {
	if (!range) return "day";
	const span = Math.max(0, range.max - range.min);
	if (span <= 2 * dayMs) return "hour";
	if (span <= 14 * dayMs) return "day";
	if (span <= 90 * dayMs) return "week";
	if (span <= 730 * dayMs) return "month";
	return "year";
}

export function formatUtcTimestamp(ms: number): string {
  if (!Number.isFinite(ms)) return "";
  return `${new Date(ms).toISOString()} UTC`;
}
