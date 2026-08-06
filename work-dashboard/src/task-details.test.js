import assert from "node:assert/strict";
import test from "node:test";

import {
  associateTaskDetail,
  collapseExpandedTaskOnEscape,
  detailList,
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

test("command values normalize without losing structured command entries", () => {
  const commands = ["rtk npm test", "rtk npm run build"];

  assert.deepEqual(detailList(commands), commands);
  assert.deepEqual(
    taskDetailSections({commands}),
    [{label: "Commands", values: commands}],
  );
});

test("row and button share one DOM-safe IDREF without task ID collisions", () => {
  const associate = taskId => {
    const detailRow = {id: ""};
    const attributes = new Map();
    const expandButton = {
      setAttribute(name, value) {
        attributes.set(name, value);
      },
      getAttribute(name) {
        return attributes.get(name) ?? null;
      },
    };

    const detailId = associateTaskDetail(detailRow, expandButton, taskId);
    const controls = expandButton.getAttribute("aria-controls");
    assert.equal(controls, detailRow.id);
    assert.equal(detailId, detailRow.id);
    assert.deepEqual(controls.trim().split(/\s+/), [controls]);
    assert.match(controls, /^task-detail-[a-f0-9]+$/);
    return detailId;
  };

  const punctuationId = associate("task.review follow-up/a:b?");
  assert.equal(punctuationId, associate("task.review follow-up/a:b?"));

  const whitespaceCandidate = associate("task.collision candidate");
  const punctuationCandidate = associate("task.collision-candidate");
  assert.notEqual(whitespaceCandidate, punctuationCandidate);
});


test("tasks expand and collapse independently", () => {
  const expandedTaskIds = new Set();

  assert.equal(toggleExpandedTask(expandedTaskIds, "task.first"), true);
  assert.equal(toggleExpandedTask(expandedTaskIds, "task.second"), true);
  assert.deepEqual([...expandedTaskIds], ["task.first", "task.second"]);

  assert.equal(toggleExpandedTask(expandedTaskIds, "task.first"), false);
  assert.deepEqual([...expandedTaskIds], ["task.second"]);
});

test("only Escape collapses the addressed expanded task", () => {
  const expandedTaskIds = new Set(["task.first", "task.second"]);

  assert.equal(collapseExpandedTaskOnEscape(expandedTaskIds, "task.first", "Enter"), false);
  assert.equal(collapseExpandedTaskOnEscape(expandedTaskIds, "task.first", " "), false);
  assert.deepEqual([...expandedTaskIds], ["task.first", "task.second"]);

  assert.equal(collapseExpandedTaskOnEscape(expandedTaskIds, "task.first", "Escape"), true);
  assert.deepEqual([...expandedTaskIds], ["task.second"]);
  assert.equal(collapseExpandedTaskOnEscape(expandedTaskIds, "task.first", "Escape"), false);
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
