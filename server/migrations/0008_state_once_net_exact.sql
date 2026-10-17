-- OAuth states are single use: a callback replayed within the 15-minute window is refused.
CREATE TABLE oauth_states_used (
  state_hash text PRIMARY KEY,
  used_at    timestamptz NOT NULL DEFAULT now()
);
-- net_paise was estimated from the SKU price; true once Google's monthly earnings report gave the real figure.
ALTER TABLE purchases ADD COLUMN net_exact boolean NOT NULL DEFAULT false;
