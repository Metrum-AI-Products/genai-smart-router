// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

const GITHUB_REPOSITORY = "https://github.com/sysadmin-metrum-ai/genai-smart-router";
const LINK_PATTERN = /(https?:\/\/[^\s]+|PR#\d+|#\d+)/gi;

export function detailList(value) {
  if (Array.isArray(value)) return value;
  return value ? [String(value)] : [];
}
export function referenceHref(value) {
  if (/^https?:\/\//i.test(value)) return value;
  const pullRequest = /^PR#(\d+)$/i.exec(value);
  if (pullRequest) return `${GITHUB_REPOSITORY}/pull/${pullRequest[1]}`;
  const issue = /^#(\d+)$/.exec(value);
  if (issue) return `${GITHUB_REPOSITORY}/issues/${issue[1]}`;
  return null;
}

export function linkedTextParts(value) {
  const text = String(value);
  const parts = [];
  let cursor = 0;
  for (const match of text.matchAll(LINK_PATTERN)) {
    if (match.index > cursor) parts.push({text: text.slice(cursor, match.index), href: null});
    let token = match[0];
    let suffix = "";
    if (/^https?:\/\//i.test(token)) {
      const trailing = /[.,;:!?]+$/.exec(token);
      if (trailing) {
        suffix = trailing[0];
        token = token.slice(0, -suffix.length);
      }
    }
    parts.push({text: token, href: referenceHref(token)});
    if (suffix) parts.push({text: suffix, href: null});
    cursor = match.index + match[0].length;
  }
  if (cursor < text.length) parts.push({text: text.slice(cursor), href: null});
  return parts.length ? parts : [{text, href: null}];
}

export function taskDetailId(taskId) {
  let encoded = "";
  const value = String(taskId);
  for (let index = 0; index < value.length; index += 1) {
    encoded += value.charCodeAt(index).toString(16).padStart(4, "0");
  }
  return `task-detail-${encoded}`;
}

export function associateTaskDetail(detailRow, expandButton, taskId) {
  const detailId = taskDetailId(taskId);
  detailRow.id = detailId;
  expandButton.setAttribute("aria-controls", detailId);
  return detailId;
}



export function taskDetailSections(task) {
  return [
    ["Actions", task.actions],
    ["Acceptance", task.acceptance],
    ["Evidence", task.evidence],
    ["Commands", task.commands],
    ["Dependencies", task.requires],
    ["References", task.references],
  ].map(([label, values]) => ({label, values: detailList(values)}))
    .filter(section => section.values.length);
}

export function toggleExpandedTask(expandedTaskIds, taskId) {
  if (expandedTaskIds.has(taskId)) {
    expandedTaskIds.delete(taskId);
    return false;
  }
  expandedTaskIds.add(taskId);
  return true;
}

export function collapseExpandedTaskOnEscape(expandedTaskIds, taskId, key) {
  if (key !== "Escape" || !expandedTaskIds.has(taskId)) return false;
  expandedTaskIds.delete(taskId);
  return true;
}
