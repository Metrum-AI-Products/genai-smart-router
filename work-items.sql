-- Read-only reconciliation and analytical views for work-items.ndjson plus its event journal.
-- Run from the repository root: duckdb -init work-items.sql

CREATE OR REPLACE VIEW work_item_records AS
SELECT *
FROM read_ndjson_auto('work-items.ndjson', union_by_name = true);

CREATE OR REPLACE VIEW work_item_event_records AS
SELECT *
FROM read_ndjson_auto('work-item-events.ndjson', union_by_name = true);

CREATE OR REPLACE VIEW work_item_events AS
SELECT
    event_id,
    event_type,
    task_id,
    revision,
    base_revision,
    CAST(occurred_at AS TIMESTAMPTZ) AS occurred_at,
    actor,
    status,
    CAST(due_date AS DATE) AS due_date,
    changed_fields,
    task
FROM work_item_event_records
WHERE kind = 'work_item_event';

CREATE OR REPLACE VIEW work_item_reconciliation_conflicts AS
WITH ordered_events AS (
    SELECT
        event_id,
        event_type,
        task_id,
        revision,
        base_revision,
        lag(revision, 1, 0) OVER (PARTITION BY task_id ORDER BY revision, event_id) AS previous_revision,
        count(*) OVER (PARTITION BY task_id, revision) AS revision_event_count
    FROM work_item_events
),
baseline_tasks AS (
    SELECT id, lower(id) AS normalized_id
    FROM work_item_records
    WHERE kind = 'task'
),
duplicate_baseline_task_ids AS (
    SELECT normalized_id
    FROM baseline_tasks
    GROUP BY normalized_id
    HAVING count(*) > 1
)
SELECT
    baseline.id AS task_id,
    0 AS revision,
    'duplicate_baseline_task_id' AS conflict_type,
    count(*) OVER (PARTITION BY baseline.normalized_id) AS conflicting_event_count
FROM baseline_tasks AS baseline
JOIN duplicate_baseline_task_ids AS duplicate USING (normalized_id)
UNION ALL
SELECT
    task_id,
    revision,
    'duplicate_revision' AS conflict_type,
    count(*) AS conflicting_event_count
FROM ordered_events
WHERE revision_event_count > 1
GROUP BY task_id, revision
UNION ALL
SELECT
    task_id,
    revision,
    'revision_gap' AS conflict_type,
    1 AS conflicting_event_count
FROM ordered_events
WHERE revision_event_count = 1
  AND (base_revision <> previous_revision OR revision <> previous_revision + 1)
UNION ALL
SELECT
    event.task_id,
    event.revision,
    'create_existing_task' AS conflict_type,
    1 AS conflicting_event_count
FROM ordered_events AS event
JOIN baseline_tasks AS baseline ON baseline.normalized_id = lower(event.task_id)
WHERE event.event_type = 'task.created'
UNION ALL
SELECT
    event.task_id,
    event.revision,
    'update_missing_task' AS conflict_type,
    1 AS conflicting_event_count
FROM ordered_events AS event
LEFT JOIN baseline_tasks AS baseline ON baseline.id = event.task_id
WHERE event.event_type = 'task.updated'
  AND baseline.id IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM ordered_events AS created
      WHERE created.task_id = event.task_id
        AND created.event_type = 'task.created'
        AND created.revision < event.revision
  );

CREATE OR REPLACE VIEW work_item_versions AS
SELECT
    id,
    stream,
    phase,
    title,
    status,
    CAST(due_date AS DATE) AS due_date,
    priority,
    assignee,
    requires,
    CAST(created_at AS TIMESTAMPTZ) AS created_at,
    CAST(updated_at AS TIMESTAMPTZ) AS updated_at,
    CAST(started_at AS TIMESTAMPTZ) AS started_at,
    CAST(completed_at AS TIMESTAMPTZ) AS completed_at,
    0 AS revision,
    NULL::VARCHAR AS event_id,
    NULL::TIMESTAMPTZ AS occurred_at,
    'baseline' AS source
FROM work_item_records
WHERE kind = 'task'
UNION ALL
SELECT
    task.id AS id,
    task.stream AS stream,
    task.phase AS phase,
    task.title AS title,
    task.status AS status,
    CAST(task.due_date AS DATE) AS due_date,
    task.priority AS priority,
    task.assignee AS assignee,
    task.requires AS requires,
    CAST(task.created_at AS TIMESTAMPTZ) AS created_at,
    CAST(task.updated_at AS TIMESTAMPTZ) AS updated_at,
    CAST(task.started_at AS TIMESTAMPTZ) AS started_at,
    CAST(task.completed_at AS TIMESTAMPTZ) AS completed_at,
    revision,
    event_id,
    occurred_at,
    'event' AS source
FROM work_item_events;

CREATE OR REPLACE VIEW work_items AS
WITH ranked_versions AS (
    SELECT
        *,
        row_number() OVER (PARTITION BY id ORDER BY revision DESC, event_id DESC NULLS LAST) AS version_rank
    FROM work_item_versions
)
SELECT * EXCLUDE (version_rank)
FROM ranked_versions AS version
WHERE version_rank = 1
  AND NOT EXISTS (
      SELECT 1
      FROM work_item_reconciliation_conflicts AS conflict
      WHERE conflict.task_id = version.id
  );

CREATE OR REPLACE VIEW work_item_reconciliation_status AS
SELECT
    (SELECT count(*) FROM work_item_records WHERE kind = 'task') AS baseline_task_count,
    (SELECT count(*) FROM work_item_events) AS event_count,
    (SELECT count(*) FROM work_items) AS current_task_count,
    (SELECT count(*) FROM work_item_reconciliation_conflicts) AS conflict_count;

CREATE OR REPLACE VIEW work_item_status_summary AS
SELECT status, count(*) AS item_count
FROM work_items
GROUP BY status
ORDER BY status;

CREATE OR REPLACE VIEW work_item_stream_status_summary AS
SELECT stream, status, count(*) AS item_count
FROM work_items
GROUP BY stream, status
ORDER BY stream, status;

CREATE OR REPLACE VIEW work_item_phase_status_summary AS
SELECT stream, phase, status, count(*) AS item_count
FROM work_items
GROUP BY stream, phase, status
ORDER BY stream, phase, status;

CREATE OR REPLACE VIEW work_item_due_summary AS
SELECT
    CASE
        WHEN status IN ('done', 'cancelled') THEN 'closed'
        WHEN due_date IS NULL THEN 'unscheduled'
        WHEN due_date < current_date THEN 'overdue'
        WHEN due_date = current_date THEN 'due_today'
        WHEN due_date <= current_date + INTERVAL 7 DAY THEN 'due_next_7_days'
        ELSE 'scheduled_later'
    END AS due_bucket,
    count(*) AS item_count
FROM work_items
GROUP BY due_bucket
ORDER BY due_bucket;

CREATE OR REPLACE VIEW work_item_dependencies AS
SELECT item.id AS item_id, dependency.id AS dependency_id
FROM work_items AS item,
UNNEST(item.requires) AS dependency(id)
ORDER BY item_id, dependency_id;
