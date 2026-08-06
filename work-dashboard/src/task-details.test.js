import assert from "node:assert/strict";
import test from "node:test";

import {
  linkedTextParts,
  referenceHref,
  taskDetailSections,
  toggleExpandedTask,
} from "./task-details.js";

test("task detail sections preserve commands and omit empty sections", () => {
  const sections = taskDetailSections({
    actions: ["Run the smoke"],
    acceptance: [],
    evidence: null,
    commands: "rtk npm run build",
    requires: ["task.package"],
    references: [],
  });

  assert.deepEqual(sections, [
    {label: "Actions", values: ["Run the smoke"]},
    {label: "Commands", values: ["rtk npm run build"]},
    {label: "Dependencies", values: ["task.package"]},
  ]);
});

test("tasks expand and collapse independently", () => {
  const expandedTaskIds = new Set();

  assert.equal(toggleExpandedTask(expandedTaskIds, "task.first"), true);
  assert.equal(toggleExpandedTask(expandedTaskIds, "task.second"), true);
  assert.deepEqual([...expandedTaskIds], ["task.first", "task.second"]);

  assert.equal(toggleExpandedTask(expandedTaskIds, "task.first"), false);
  assert.deepEqual([...expandedTaskIds], ["task.second"]);
});

test("GitHub and external references become safe link targets", () => {
  assert.equal(
    referenceHref("#765"),
    "https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/765",
  );
  assert.equal(
    referenceHref("PR#766"),
    "https://github.com/sysadmin-metrum-ai/genai-smart-router/pull/766",
  );
  assert.equal(referenceHref("https://example.com/runbook"), "https://example.com/runbook");
  assert.equal(referenceHref("task.package"), null);

  assert.deepEqual(
    linkedTextParts("Resolve #765 with PR#766; see https://example.com/runbook."),
    [
      {text: "Resolve ", href: null},
      {text: "#765", href: "https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/765"},
      {text: " with ", href: null},
      {text: "PR#766", href: "https://github.com/sysadmin-metrum-ai/genai-smart-router/pull/766"},
      {text: "; see ", href: null},
      {text: "https://example.com/runbook", href: "https://example.com/runbook"},
      {text: ".", href: null},
    ],
  );
});
