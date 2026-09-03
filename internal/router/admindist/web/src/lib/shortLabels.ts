// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

export type ShortLabelMap = {
  short: string;
  full: string;
};

const defaultWidth = 16;

export function buildShortLabelMap(values: string[], opts: { prefix?: string; width?: number } = {}): ShortLabelMap[] {
  const width = Math.max(4, opts.width ?? defaultWidth);
  const seenFull = new Set<string>();
  const usedShort = new Map<string, number>();
  const entries: ShortLabelMap[] = [];

  for (const raw of values) {
    const full = raw.trim();
    if (!full || seenFull.has(full)) continue;
    seenFull.add(full);

    const base = opts.prefix ? `${opts.prefix}${entries.length + 1}` : heuristicShort(full, width);
    const short = uniqueShort(base, width, usedShort);
    entries.push({ short, full });
  }

  return entries;
}

export function heuristicShort(s: string, width: number = defaultWidth): string {
  const value = s.trim();
  if (!value) return "";
  const compositeParts = value.split("\u0000").map((part) => part.trim()).filter(Boolean);
  if (compositeParts.length > 1) return heuristicShort(compositeParts[compositeParts.length - 1], width);
  if (charLength(value) <= width) return value;

  if (value.includes("@")) {
    const localPart = value.split("@", 1)[0] || value;
    return markOmitted(localPart, width);
  }

  if (value.includes("/")) {
    const lastSegment = value.split("/").filter(Boolean).pop() || value;
    return truncateEnd(lastSegment, width);
  }

  return truncateEnd(value, width);
}

function uniqueShort(base: string, width: number, usedShort: Map<string, number>): string {
  const count = usedShort.get(base) || 0;
  usedShort.set(base, count + 1);
  if (count === 0) return base;

  let suffixIndex = count + 1;
  let candidate = withSuffix(base, suffixIndex, width);
  while (usedShort.has(candidate)) {
    suffixIndex += 1;
    candidate = withSuffix(base, suffixIndex, width);
  }
  usedShort.set(candidate, 1);
  return candidate;
}

function withSuffix(value: string, index: number, width: number): string {
  const suffix = `-${index}`;
  const headWidth = Math.max(1, width - charLength(suffix));
  return `${truncateEnd(value, headWidth)}${suffix}`;
}

function truncateEnd(value: string, width: number): string {
  if (charLength(value) <= width) return value;
  if (width <= 1) return "…";
  return `${Array.from(value).slice(0, width - 1).join("")}…`;
}

function markOmitted(value: string, width: number): string {
  if (width <= 1) return "…";
  const chars = Array.from(value);
  if (chars.length < width) return `${value}…`;
  return `${chars.slice(0, width - 1).join("")}…`;
}

function charLength(value: string): number {
  return Array.from(value).length;
}
