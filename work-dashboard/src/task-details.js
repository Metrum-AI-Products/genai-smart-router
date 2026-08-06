export function detailList(value) {
  if (Array.isArray(value)) return value;
  return value ? [String(value)] : [];
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
