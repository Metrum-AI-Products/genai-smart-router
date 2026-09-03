// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { buildShortLabelMap, heuristicShort } from "./shortLabels";

describe("short chart labels", () => {
  test("shortens email labels to local-part prefixes", () => {
    expect(heuristicShort("alice@metrum.ai", 6)).toBe("alice…");
  });

  test("shortens provider paths from the last segment", () => {
    expect(heuristicShort("fireworks/accounts/fireworks/models/deepseek-v4-flash", 16)).toBe("deepseek-v4-fla…");
  });

  test("shortens NUL-joined composite buckets from the final bucket component", () => {
    expect(heuristicShort("openai\u0000gpt-5.4-nano", 16)).toBe("gpt-5.4-nano");
    expect(heuristicShort("fireworks\u0000accounts/fireworks/models/deepseek-v4-flash", 16)).toBe("deepseek-v4-fla…");
  });

  test("width-caps long opaque strings", () => {
    expect(heuristicShort("averyverylongopaqueidentifier", 10)).toBe("averyvery…");
  });

  test("preserves input order and removes empty or duplicate labels", () => {
    expect(buildShortLabelMap(["", " alpha ", "beta", "alpha"])).toEqual([
      { short: "alpha", full: "alpha" },
      { short: "beta", full: "beta" },
    ]);
  });

  test("suffixes collisions without exceeding the width cap", () => {
    const labels = buildShortLabelMap(["prefix-one", "prefix-two"], { width: 7 });
    expect(labels).toEqual([
      { short: "prefix…", full: "prefix-one" },
      { short: "pref…-2", full: "prefix-two" },
    ]);
    expect(labels.every((label) => Array.from(label.short).length <= 7)).toBe(true);
  });

  test("does not split unicode code points at the truncation boundary", () => {
    expect(heuristicShort("αβγδεζηθ", 5)).toBe("αβγδ…");
  });
});
