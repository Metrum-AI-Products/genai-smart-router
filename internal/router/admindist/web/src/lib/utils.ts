// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export const numberFmt = new Intl.NumberFormat("en-US", { maximumFractionDigits: 2 });
export const compactFmt = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 2,
});
export const usdFmt = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  maximumFractionDigits: 6,
});
export const usdCompactFmt = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  notation: "compact",
  maximumFractionDigits: 2,
});

export function formatValue(value: unknown, key = ""): string {
  if (value === null || value === undefined || value === "") return "-";
  if (typeof value === "boolean") return value ? "yes" : "no";
  if (typeof value === "number") {
    const lower = key.toLowerCase();
    if (lower.includes("usd") || lower.includes("cost")) return usdFmt.format(value);
    if (lower.includes("pct") || lower.includes("rate")) return `${numberFmt.format(value)}%`;
    if (lower.includes("latency") || lower.includes("ttfb") || lower.endsWith("ms")) return `${numberFmt.format(value)} ms`;
    if (lower.includes("tokenspersec")) return `${numberFmt.format(value)} tok/s`;
    return numberFmt.format(value);
  }
  if (Array.isArray(value)) return value.join(", ");
  return String(value);
}

export function titleize(input: string): string {
  return input
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (m) => m.toUpperCase());
}

export function isNumeric(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}
