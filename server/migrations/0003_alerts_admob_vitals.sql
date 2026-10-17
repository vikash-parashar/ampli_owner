-- Ampli Owner schema v3: AdMob and Play vitals sync, auto-stop rules, campaign audit log.

-- Single-account integrations (AdMob today). Tokens are AES-GCM encrypted with TOKEN_KEY.
CREATE TABLE integrations (
  kind         text PRIMARY KEY CHECK (kind IN ('admob')),
  account      text NOT NULL DEFAULT '',
  token_enc    bytea NOT NULL,
  refresh_enc  bytea,
  expires_at   timestamptz,
  last_sync_at timestamptz,
  last_error   text NOT NULL DEFAULT '',
  created_at   timestamptz NOT NULL DEFAULT now()
);

-- Google Play Android vitals, 28-day user-weighted rates (0.0109 = 1.09%).
CREATE TABLE vitals (
  product_id text NOT NULL REFERENCES products(id),
  day        date NOT NULL,
  crash_rate double precision,
  anr_rate   double precision,
  PRIMARY KEY (product_id, day)
);

-- Auto-stop: pause a live campaign when ROAS stays below min_roas after min_days and min_spend.
ALTER TABLE campaigns
  ADD COLUMN auto_stop        boolean NOT NULL DEFAULT false,
  ADD COLUMN auto_min_roas    double precision NOT NULL DEFAULT 0.5,
  ADD COLUMN auto_min_days    integer NOT NULL DEFAULT 7,
  ADD COLUMN auto_min_spend   bigint NOT NULL DEFAULT 200000;

-- Every change the owner or a rule made to a campaign.
CREATE TABLE campaign_log (
  id          bigserial PRIMARY KEY,
  campaign_id bigint NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
  actor       text NOT NULL CHECK (actor IN ('owner','rule','sync')),
  action      text NOT NULL,
  detail      text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX campaign_log_idx ON campaign_log (campaign_id, created_at DESC);

-- Purchases are verified with Google Play; money is counted net (after Google's fee and GST) and refunds removed.
ALTER TABLE purchases
  ADD COLUMN verified    boolean NOT NULL DEFAULT false,
  ADD COLUMN net_paise   bigint NOT NULL DEFAULT 0,
  ADD COLUMN token_hash  text,
  ADD COLUMN region      text NOT NULL DEFAULT '',
  ADD COLUMN refunded_at timestamptz;
CREATE UNIQUE INDEX purchases_token_idx ON purchases (token_hash) WHERE token_hash IS NOT NULL;

-- Prices the server trusts (the app only says which SKU was bought).
CREATE TABLE skus (
  product_id  text NOT NULL REFERENCES products(id),
  sku         text NOT NULL,
  kind        text NOT NULL CHECK (kind IN ('pro','tip','subscription','other')),
  price_paise bigint NOT NULL CHECK (price_paise >= 0),
  PRIMARY KEY (product_id, sku)
);
INSERT INTO skus (product_id, sku, kind, price_paise) VALUES
  ('mp3', 'ampli_pro', 'pro', 19900), ('mp3', 'ampli_tip_49', 'tip', 4900), ('mp3', 'ampli_tip_99', 'tip', 9900), ('mp3', 'ampli_tip_199', 'tip', 19900),
  ('carhub', 'ampli_drive_pro', 'pro', 49900), ('carhub', 'ampli_tip_49', 'tip', 4900), ('carhub', 'ampli_tip_99', 'tip', 9900), ('carhub', 'ampli_tip_199', 'tip', 19900),
  ('naukri', 'ampli_tip_49', 'tip', 4900), ('naukri', 'ampli_tip_99', 'tip', 9900), ('naukri', 'ampli_tip_199', 'tip', 19900);

-- Per-network daily limit for the public endpoints (key = salted hash of the IP's /24 or /64).
CREATE TABLE ip_hits (
  ip_key text NOT NULL,
  day    date NOT NULL,
  hits   integer NOT NULL DEFAULT 0,
  PRIMARY KEY (ip_key, day)
);

CREATE INDEX campaigns_match_idx ON campaigns (platform, product_id, created_at DESC);
