-- Owner actions that change money or data (erase, prices, integrations) and soft-deleted campaigns keep a trail.
CREATE TABLE audit_log (
  id         bigserial PRIMARY KEY,
  action     text NOT NULL,
  detail     text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE campaigns ADD COLUMN deleted_at timestamptz;
