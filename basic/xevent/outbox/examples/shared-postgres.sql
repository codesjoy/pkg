CREATE TABLE xevent_outbox_records (
  id BIGSERIAL PRIMARY KEY,
  message_id VARCHAR(36) NOT NULL DEFAULT '',
  mode VARCHAR(16) NOT NULL,
  handoff_from_id BIGINT NULL,
  event_type VARCHAR(255) NOT NULL,
  event_id VARCHAR(255) NOT NULL DEFAULT '',
  partition_key VARCHAR(255) NOT NULL DEFAULT '',
  payload BYTEA NOT NULL,
  topic VARCHAR(255) NOT NULL DEFAULT '',
  available_at TIMESTAMPTZ NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT '',
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  claim_owner VARCHAR(255) NOT NULL DEFAULT '',
  claim_until TIMESTAMPTZ,
  sent_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT idx_xevent_outbox_handoff_from_id UNIQUE (handoff_from_id)
);

CREATE INDEX idx_xevent_outbox_mode_status_partition_available_id
  ON xevent_outbox_records (mode, status, partition_key, available_at, id);

CREATE INDEX idx_xevent_outbox_mode_created_at
  ON xevent_outbox_records (mode, created_at);

CREATE INDEX idx_xevent_outbox_pending_partition_available_id
  ON xevent_outbox_records (partition_key, available_at, id)
  WHERE mode = 'relay' AND status = 'pending';

CREATE INDEX idx_xevent_outbox_sending_partition_available_id
  ON xevent_outbox_records (partition_key, available_at, id)
  WHERE mode = 'relay' AND status = 'sending';
