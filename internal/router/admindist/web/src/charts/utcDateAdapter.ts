// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

import { _adapters } from "chart.js";

type TimeUnit = "millisecond" | "second" | "minute" | "hour" | "day" | "week" | "month" | "quarter" | "year";
type TimeFormat = TimeUnit | "datetime";

const formats: Record<TimeFormat, string> = {
	datetime: "MMM d, yyyy HH:mm:ss",
	millisecond: "HH:mm:ss.SSS",
	second: "HH:mm:ss",
	minute: "HH:mm",
	hour: "HH:mm",
	day: "MMM d",
	week: "MMM d",
	month: "MMM yyyy",
  quarter: "MMM yyyy",
  year: "yyyy",
};

const unitFormatters: Record<TimeUnit, Intl.DateTimeFormat> = {
	millisecond: new Intl.DateTimeFormat("en-US", { timeZone: "UTC", hour: "2-digit", minute: "2-digit", second: "2-digit", fractionalSecondDigits: 3, hour12: false }),
	second: new Intl.DateTimeFormat("en-US", { timeZone: "UTC", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false }),
	minute: new Intl.DateTimeFormat("en-US", { timeZone: "UTC", hour: "2-digit", minute: "2-digit", hour12: false }),
	hour: new Intl.DateTimeFormat("en-US", { timeZone: "UTC", hour: "2-digit", minute: "2-digit", hour12: false }),
  day: new Intl.DateTimeFormat("en-US", { timeZone: "UTC", month: "short", day: "numeric" }),
  week: new Intl.DateTimeFormat("en-US", { timeZone: "UTC", month: "short", day: "numeric" }),
  month: new Intl.DateTimeFormat("en-US", { timeZone: "UTC", month: "short", year: "numeric" }),
  quarter: new Intl.DateTimeFormat("en-US", { timeZone: "UTC", month: "short", year: "numeric" }),
  year: new Intl.DateTimeFormat("en-US", { timeZone: "UTC", year: "numeric" }),
};

_adapters._date.override({
  formats: () => formats,
  parse(value: unknown) {
    if (typeof value === "number" && Number.isFinite(value)) return value;
    if (value instanceof Date) return value.getTime();
    if (typeof value === "string") {
      const parsed = Date.parse(value);
      return Number.isFinite(parsed) ? parsed : null;
    }
    return null;
  },
  format(value: number, format: string) {
    const unit = timeUnitForFormat(format);
    return unitFormatters[unit].format(new Date(value));
  },
  add(value: number, amount: number, unit: TimeUnit) {
    const date = new Date(value);
    switch (unit) {
      case "millisecond":
        return value + amount;
      case "second":
        return value + amount * 1000;
      case "minute":
        return value + amount * 60 * 1000;
      case "hour":
        return value + amount * 60 * 60 * 1000;
      case "day":
        return value + amount * 24 * 60 * 60 * 1000;
      case "week":
        return value + amount * 7 * 24 * 60 * 60 * 1000;
      case "month":
        date.setUTCMonth(date.getUTCMonth() + amount);
        return date.getTime();
      case "quarter":
        date.setUTCMonth(date.getUTCMonth() + amount * 3);
        return date.getTime();
      case "year":
        date.setUTCFullYear(date.getUTCFullYear() + amount);
        return date.getTime();
    }
  },
  diff(max: number, min: number, unit: TimeUnit) {
    const delta = max - min;
    switch (unit) {
      case "millisecond":
        return delta;
      case "second":
        return delta / 1000;
      case "minute":
        return delta / (60 * 1000);
      case "hour":
        return delta / (60 * 60 * 1000);
      case "day":
        return delta / (24 * 60 * 60 * 1000);
      case "week":
        return delta / (7 * 24 * 60 * 60 * 1000);
      case "month":
        return (new Date(max).getUTCFullYear() - new Date(min).getUTCFullYear()) * 12 + new Date(max).getUTCMonth() - new Date(min).getUTCMonth();
      case "quarter":
        return this.diff(max, min, "month") / 3;
      case "year":
        return new Date(max).getUTCFullYear() - new Date(min).getUTCFullYear();
    }
  },
  startOf(value: number, unit: TimeUnit, weekday?: number) {
    const date = new Date(value);
    switch (unit) {
      case "second":
        date.setUTCMilliseconds(0);
        break;
      case "minute":
        date.setUTCSeconds(0, 0);
        break;
      case "hour":
        date.setUTCMinutes(0, 0, 0);
        break;
      case "day":
        date.setUTCHours(0, 0, 0, 0);
        break;
      case "week": {
        date.setUTCHours(0, 0, 0, 0);
        const startDay = typeof weekday === "number" ? weekday : 0;
        const diff = (date.getUTCDay() - startDay + 7) % 7;
        date.setUTCDate(date.getUTCDate() - diff);
        break;
      }
      case "month":
        date.setUTCDate(1);
        date.setUTCHours(0, 0, 0, 0);
        break;
      case "quarter": {
        const quarterStart = Math.floor(date.getUTCMonth() / 3) * 3;
        date.setUTCMonth(quarterStart, 1);
        date.setUTCHours(0, 0, 0, 0);
        break;
      }
      case "year":
        date.setUTCMonth(0, 1);
        date.setUTCHours(0, 0, 0, 0);
        break;
    }
    return date.getTime();
  },
  endOf(value: number, unit: TimeUnit) {
    return this.add(this.startOf(value, unit), 1, unit) - 1;
  },
});

function isTimeUnit(value: string): value is TimeUnit {
  return value in unitFormatters;
}

function timeUnitForFormat(format: string): TimeUnit {
  if (isTimeUnit(format)) return format;
  const matchingUnit = (Object.entries(formats) as Array<[TimeFormat, string]>).find(([, value]) => value === format)?.[0];
  return matchingUnit && isTimeUnit(matchingUnit) ? matchingUnit : "day";
}
