import assert from "node:assert/strict";
import test from "node:test";

import {taskDetailSections, toggleExpandedTask} from "./task-details.js";

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
