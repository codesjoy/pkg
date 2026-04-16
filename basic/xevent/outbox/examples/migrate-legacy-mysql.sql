-- Upgrade an existing relay deployment in place and import any legacy
-- Debezium rows from xevent_debezium_outbox_records into the shared table.

ALTER TABLE xevent_outbox_records
  ADD COLUMN IF NOT EXISTS message_id VARCHAR(36) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS mode VARCHAR(16) NOT NULL DEFAULT 'relay',
  ADD COLUMN IF NOT EXISTS handoff_from_id BIGINT UNSIGNED NULL;

ALTER TABLE xevent_outbox_records
  MODIFY COLUMN event_id VARCHAR(255) NOT NULL DEFAULT '',
  MODIFY COLUMN claim_owner VARCHAR(255) NOT NULL DEFAULT '',
  MODIFY COLUMN status VARCHAR(16) NOT NULL DEFAULT '';

UPDATE xevent_outbox_records
SET mode = 'relay'
WHERE mode = '' OR mode IS NULL;

UPDATE xevent_outbox_records
SET message_id = CAST(id AS CHAR(36))
WHERE message_id = '' OR message_id IS NULL;

CREATE UNIQUE INDEX idx_xevent_outbox_handoff_from_id
  ON xevent_outbox_records (handoff_from_id);

CREATE INDEX idx_xevent_outbox_mode_status_partition_available_id
  ON xevent_outbox_records (mode, status, partition_key, available_at, id);

CREATE INDEX idx_xevent_outbox_mode_created_at
  ON xevent_outbox_records (mode, created_at);

INSERT INTO xevent_outbox_records (
  message_id,
  mode,
  event_type,
  event_id,
  partition_key,
  payload,
  topic,
  available_at,
  status,
  attempts,
  last_error,
  claim_owner,
  claim_until,
  sent_at,
  created_at,
  updated_at
)
SELECT
  legacy.id,
  'cdc',
  legacy.event_type,
  legacy.event_id,
  legacy.partition_key,
  legacy.payload,
  legacy.topic,
  legacy.created_at,
  '',
  0,
  '',
  '',
  NULL,
  NULL,
  legacy.created_at,
  legacy.created_at
FROM xevent_debezium_outbox_records AS legacy
LEFT JOIN xevent_outbox_records AS shared
  ON shared.mode = 'cdc' AND shared.message_id = legacy.id
WHERE shared.id IS NULL;

-- After this migration:
-- 1. deploy code that reads/writes the shared table
-- 2. update Debezium connector to xevent_outbox_records + message_id
-- 3. if moving unsent relay backlog to cdc, stop relay workers, wait one
--    ClaimTTL, then run debeziumgorm.CutoverRelayBacklog(...)
