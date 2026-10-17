-- Ampli Owner schema v2: Naukri, ad accounts and campaigns run from the app,
-- install/purchase attribution, reviews, app event counters. Money is in paise.

INSERT INTO products (id, name, package_id, repo, price_paise, color) VALUES
  ('naukri', 'Ampli Naukri', 'com.vikash.ampli.naukri', 'vikash-parashar/ampli-naukri', 0, 'saffron')
ON CONFLICT (id) DO NOTHING;

ALTER TABLE daily_stats
  ADD COLUMN tips_paise          bigint NOT NULL DEFAULT 0,
  ADD COLUMN other_revenue_paise bigint NOT NULL DEFAULT 0;

-- One row per connected ad account. Tokens are AES-GCM encrypted with TOKEN_KEY.
CREATE TABLE ad_accounts (
  id           bigserial PRIMARY KEY,
  platform     text NOT NULL CHECK (platform IN ('meta','tiktok','google')),
  external_id  text NOT NULL,
  name         text NOT NULL DEFAULT '',
  currency     text NOT NULL DEFAULT 'INR',
  token_enc    bytea NOT NULL,
  refresh_enc  bytea,
  expires_at   timestamptz,
  status       text NOT NULL DEFAULT 'connected' CHECK (status IN ('connected','expired')),
  last_sync_at timestamptz,
  last_error   text NOT NULL DEFAULT '',
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (platform, external_id)
);

ALTER TABLE campaigns DROP CONSTRAINT campaigns_platform_check;
UPDATE campaigns SET platform = 'meta'   WHERE platform IN ('instagram','facebook');
UPDATE campaigns SET platform = 'google' WHERE platform = 'youtube';
ALTER TABLE campaigns ADD CONSTRAINT campaigns_platform_check CHECK (platform IN ('meta','tiktok','google','other'));

ALTER TABLE campaigns DROP CONSTRAINT campaigns_status_check;
UPDATE campaigns SET status = 'in_review' WHERE status = 'scheduled';
ALTER TABLE campaigns ADD CONSTRAINT campaigns_status_check
  CHECK (status IN ('draft','in_review','live','paused','ended','error'));

ALTER TABLE campaigns
  ADD COLUMN ad_account_id      bigint REFERENCES ad_accounts(id) ON DELETE SET NULL,
  ADD COLUMN external_id        text NOT NULL DEFAULT '',   -- platform campaign id
  ADD COLUMN external_set_id    text NOT NULL DEFAULT '',   -- ad set / ad group / budget id
  ADD COLUMN countries          text[] NOT NULL DEFAULT '{}',
  ADD COLUMN daily_budget_paise bigint NOT NULL DEFAULT 0,
  ADD COLUMN creative_url       text NOT NULL DEFAULT '',
  ADD COLUMN utm_campaign       text,
  ADD COLUMN revenue_paise      bigint NOT NULL DEFAULT 0, -- manual extra revenue on top of attributed purchases
  ADD COLUMN last_error         text NOT NULL DEFAULT '',
  ADD COLUMN created_at         timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN synced_at          timestamptz;
UPDATE campaigns SET utm_campaign = 'ampli-c' || id WHERE utm_campaign IS NULL;
CREATE UNIQUE INDEX campaigns_utm_idx ON campaigns (utm_campaign);

-- Daily numbers pulled from the ad platforms.
CREATE TABLE campaign_days (
  campaign_id bigint NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
  day         date NOT NULL,
  spend_paise bigint NOT NULL DEFAULT 0,
  impressions bigint NOT NULL DEFAULT 0,
  clicks      bigint NOT NULL DEFAULT 0,
  installs    bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (campaign_id, day)
);

-- First-run ping from each app with the Play Install Referrer.
CREATE TABLE installs (
  product_id   text NOT NULL REFERENCES products(id),
  install_id   text NOT NULL,
  utm_source   text NOT NULL DEFAULT '',
  utm_medium   text NOT NULL DEFAULT '',
  utm_campaign text NOT NULL DEFAULT '',
  utm_content  text NOT NULL DEFAULT '',
  ad_platform  text NOT NULL DEFAULT '',  -- meta, tiktok, google or '' (organic), derived from the referrer
  referrer     text NOT NULL DEFAULT '',
  app_version  text NOT NULL DEFAULT '',
  country      text NOT NULL DEFAULT '',
  created_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (product_id, install_id)
);
CREATE INDEX installs_campaign_idx ON installs (utm_campaign) WHERE utm_campaign <> '';
CREATE INDEX installs_day_idx ON installs (product_id, created_at);

CREATE TABLE purchases (
  id           bigserial PRIMARY KEY,
  product_id   text NOT NULL REFERENCES products(id),
  install_id   text NOT NULL,
  kind         text NOT NULL CHECK (kind IN ('pro','tip','subscription','other')),
  sku          text NOT NULL DEFAULT '',
  amount_paise bigint NOT NULL CHECK (amount_paise >= 0),
  currency     text NOT NULL DEFAULT 'INR',
  order_id     text NOT NULL UNIQUE,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX purchases_install_idx ON purchases (product_id, install_id);

CREATE TABLE reviews (
  id          bigserial PRIMARY KEY,
  product_id  text NOT NULL REFERENCES products(id),
  external_id text,                  -- Google Play review id
  source      text NOT NULL DEFAULT 'manual' CHECK (source IN ('play','manual')),
  author      text NOT NULL DEFAULT '',
  rating      smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
  body        text NOT NULL DEFAULT '',
  sentiment   text GENERATED ALWAYS AS (CASE WHEN rating >= 4 THEN 'positive' WHEN rating = 3 THEN 'mixed' ELSE 'negative' END) STORED,
  reply       text NOT NULL DEFAULT '',
  replied_at  timestamptz,
  app_version text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (product_id, external_id)
);
CREATE INDEX reviews_product_idx ON reviews (product_id, created_at DESC);

-- Anonymous per-day counters the apps send (feature used, voice picked...).
CREATE TABLE app_events (
  product_id text NOT NULL REFERENCES products(id),
  day        date NOT NULL,
  name       text NOT NULL,
  count      bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (product_id, day, name)
);

-- Rate limit for the public install/purchase/event endpoints.
CREATE TABLE public_hits (
  install_id text NOT NULL,
  day        date NOT NULL,
  hits       integer NOT NULL DEFAULT 0,
  PRIMARY KEY (install_id, day)
);
