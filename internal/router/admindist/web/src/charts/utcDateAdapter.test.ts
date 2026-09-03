// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

import { _adapters } from "chart.js";
import { describe, expect, test } from "vitest";
import "./utcDateAdapter";

const adapter = new _adapters._date();
const timestamp = Date.UTC(2026, 6, 1, 8, 0, 0);

describe("UTC Chart.js date adapter", () => {
	test("parses epoch milliseconds, Date values, and RFC3339 strings", () => {
		expect(adapter.parse(timestamp)).toBe(timestamp);
		expect(adapter.parse(new Date(timestamp))).toBe(timestamp);
		expect(adapter.parse("2026-07-01T08:00:00.000Z")).toBe(timestamp);
		expect(adapter.parse(null)).toBeNull();
		expect(adapter.parse("not a date")).toBeNull();
	});

	test("formats supported UTC units", () => {
		expect(adapter.format(timestamp, "hour")).toBe("08:00");
		expect(adapter.format(timestamp, "day")).toBe("Jul 1");
		expect(adapter.format(timestamp, "month")).toBe("Jul 2026");
		expect(adapter.format(timestamp, "year")).toBe("2026");
	});

	test("adds and diffs in UTC units", () => {
		expect(adapter.add(timestamp, 1, "hour")).toBe(timestamp + 60 * 60 * 1000);
		expect(adapter.diff(timestamp + 2 * 24 * 60 * 60 * 1000, timestamp, "day")).toBe(2);
	});

	test("starts periods in UTC", () => {
		const late = Date.UTC(2026, 6, 1, 23, 59, 58, 123);
		expect(adapter.startOf(late, "day")).toBe(Date.UTC(2026, 6, 1, 0, 0, 0, 0));
		expect(adapter.startOf(late, "month")).toBe(Date.UTC(2026, 6, 1, 0, 0, 0, 0));
	});
});
