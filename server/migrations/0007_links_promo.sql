-- Tracked Play links per app and channel; installs carry utm_campaign 'ampli-l' || id.
CREATE TABLE tracked_links (
  id           bigserial PRIMARY KEY,
  product_id   text NOT NULL REFERENCES products(id),
  channel      text NOT NULL CHECK (channel IN ('play_hindi','website','whatsapp','telegram','instagram','youtube','crosspromo','other')),
  label        text NOT NULL DEFAULT '',
  listing      text NOT NULL DEFAULT '',   -- Play custom store listing name, e.g. 'hindi'
  from_product text NOT NULL DEFAULT '',   -- for cross-promo links: the app the link sits in
  created_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz
);
CREATE INDEX installs_source_idx ON installs (product_id, created_at, utm_source);

-- Play promo codes handed to one app's Pro buyers for another app's Pro (Car Hub Pro -> MP3 Pro).
CREATE TABLE promo_codes (
  id              bigserial PRIMARY KEY,
  source_product  text NOT NULL REFERENCES products(id),
  target_product  text NOT NULL REFERENCES products(id),
  sku             text NOT NULL,
  code            text NOT NULL UNIQUE,
  claimed_install text,
  claimed_at      timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX promo_claim_idx ON promo_codes (source_product, target_product, claimed_install) WHERE claimed_install IS NOT NULL;
CREATE INDEX promo_free_idx ON promo_codes (source_product, target_product, id) WHERE claimed_at IS NULL;
