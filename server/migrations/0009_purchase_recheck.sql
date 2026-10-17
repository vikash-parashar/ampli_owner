-- While a purchase is unconfirmed, its Play token is kept encrypted (AES-GCM, TOKEN_KEY) so the daily sync can
-- re-check it after a setup fix; it is wiped once Google answers or after 30 days. verify_error says why it waits.
ALTER TABLE purchases ADD COLUMN token_enc bytea, ADD COLUMN verify_error text NOT NULL DEFAULT '';
