// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { formatUtcTimestamp, pickTimeUnit, timeRange } from "./timeAxis";

const base = Date.UTC(2026, 5, 28, 12, 0, 0);
const hour = 60 * 60 * 1000;
const day = 24 * hour;

describe("time axis helpers", () => {
  test("computes finite time ranges", () => {
    expect(timeRange([base + day, Number.NaN, base, Infinity])).toEqual({ min: base, max: base + day });
    expect(timeRange([Number.NaN, Infinity])).toBeNull();
  });

	test("picks compact units for short ranges", () => {
		expect(pickTimeUnit({ min: base, max: base + 24 * hour })).toBe("hour");
		expect(pickTimeUnit({ min: base, max: base + 48 * hour })).toBe("hour");
		expect(pickTimeUnit({ min: base, max: base + 49 * hour })).toBe("day");
		expect(pickTimeUnit({ min: base, max: base + 14 * day })).toBe("day");
	});

	test("picks broader units for long ranges", () => {
		expect(pickTimeUnit({ min: base, max: base + 15 * day })).toBe("week");
		expect(pickTimeUnit({ min: base, max: base + 90 * day })).toBe("week");
		expect(pickTimeUnit({ min: base, max: base + 91 * day })).toBe("month");
		expect(pickTimeUnit({ min: base, max: base + 365 * day })).toBe("month");
		expect(pickTimeUnit({ min: base, max: base + 2 * 365 * day })).toBe("month");
		expect(pickTimeUnit({ min: base, max: base + 3 * 365 * day })).toBe("year");
	});

  test("formats tooltip timestamps as explicit UTC", () => {
    expect(formatUtcTimestamp(Date.UTC(2026, 5, 28, 12, 30, 5))).toBe("2026-06-28T12:30:05.000Z UTC");
  });
});
