-- Play has no OAuth token (it uses the service account) but its sync state lives next to AdMob's.
ALTER TABLE integrations DROP CONSTRAINT integrations_kind_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_kind_check CHECK (kind IN ('admob', 'play'));
ALTER TABLE integrations ALTER COLUMN token_enc DROP NOT NULL;
