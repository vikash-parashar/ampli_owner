-- Ampli Owner schema v1. Money is stored in paise.
CREATE TABLE products (
  id          text PRIMARY KEY,               -- 'mp3', 'carhub'
  name        text NOT NULL,
  package_id  text NOT NULL,
  repo        text NOT NULL DEFAULT '',
  price_paise integer NOT NULL DEFAULT 0,
  color       text NOT NULL DEFAULT '',
  play_status text NOT NULL DEFAULT 'not_listed'
);

INSERT INTO products (id, name, package_id, repo, price_paise, color) VALUES
  ('mp3',    'Ampli MP3 Player', 'com.vikash.ampli',       'vikash-parashar/music-player', 19900, 'rose'),
  ('carhub', 'Ampli Car Hub',    'com.vikash.ampli.drive', 'vikash-parashar/ampli-drive',  49900, 'cyan');

CREATE TABLE tickets (
  id               bigserial PRIMARY KEY,
  product_id       text NOT NULL REFERENCES products(id),
  kind             text NOT NULL CHECK (kind IN ('bug','feature','support','review','feedback')),
  title            text NOT NULL,
  body             text NOT NULL DEFAULT '',
  status           text NOT NULL DEFAULT 'new'
                   CHECK (status IN ('new','waiting_on_owner','waiting_on_user','planned','in_progress','fixed','closed')),
  priority         smallint NOT NULL DEFAULT 2,  -- 1 high, 2 normal, 3 low
  source           text NOT NULL DEFAULT 'manual' CHECK (source IN ('app','play','email','manual')),
  install_id       text NOT NULL DEFAULT '',
  reporter_name    text NOT NULL DEFAULT '',
  reporter_contact text NOT NULL DEFAULT '',
  app_version      text NOT NULL DEFAULT '',
  device           text NOT NULL DEFAULT '',
  os_version       text NOT NULL DEFAULT '',
  locale           text NOT NULL DEFAULT '',
  is_pro           boolean NOT NULL DEFAULT false,
  order_id         text NOT NULL DEFAULT '',
  votes            integer NOT NULL DEFAULT 0,
  github_issue_url text NOT NULL DEFAULT '',
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tickets_status_idx  ON tickets (status, created_at DESC);
CREATE INDEX tickets_install_idx ON tickets (install_id, created_at DESC);

CREATE TABLE ticket_messages (
  id         bigserial PRIMARY KEY,
  ticket_id  bigint NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  author     text NOT NULL CHECK (author IN ('user','owner')),
  body       text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE feature_votes (
  ticket_id  bigint NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  install_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (ticket_id, install_id)
);

-- Built-vs-left list and roadmap in one table.
CREATE TABLE features (
  id         bigserial PRIMARY KEY,
  product_id text NOT NULL REFERENCES products(id),
  title      text NOT NULL,
  state      text NOT NULL CHECK (state IN ('built','left','now','next','later')),
  ticket_id  bigint REFERENCES tickets(id) ON DELETE SET NULL,
  build      text NOT NULL DEFAULT '',
  sort       integer NOT NULL DEFAULT 0,
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- Same ids as ampli-company/checklist.json so ticks carry over.
CREATE TABLE checklist_items (
  id       text PRIMARY KEY,
  phase    text NOT NULL,
  title    text NOT NULL,
  detail   text NOT NULL DEFAULT '',
  products text[] NOT NULL DEFAULT '{}',
  status   text NOT NULL DEFAULT 'todo' CHECK (status IN ('todo','in-progress','done')),
  link     text NOT NULL DEFAULT '',
  sort     integer NOT NULL DEFAULT 0,
  done_at  timestamptz
);

CREATE TABLE todos (
  id         bigserial PRIMARY KEY,
  title      text NOT NULL,
  due        date,
  product_id text REFERENCES products(id),
  done       boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE builds (
  id         bigserial PRIMARY KEY,
  product_id text NOT NULL REFERENCES products(id),
  tag        text NOT NULL,
  apk_url    text NOT NULL DEFAULT '',
  notes      text NOT NULL DEFAULT '',
  tested     boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (product_id, tag)
);

CREATE TABLE daily_stats (
  product_id        text NOT NULL REFERENCES products(id),
  day               date NOT NULL,
  installs          integer NOT NULL DEFAULT 0,
  uninstalls        integer NOT NULL DEFAULT 0,
  active_users      integer NOT NULL DEFAULT 0,
  rating            numeric(3,2),
  reviews           integer NOT NULL DEFAULT 0,
  pro_sales         integer NOT NULL DEFAULT 0,
  pro_revenue_paise bigint NOT NULL DEFAULT 0,
  ad_revenue_paise  bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (product_id, day)
);

CREATE TABLE campaigns (
  id           bigserial PRIMARY KEY,
  platform     text NOT NULL CHECK (platform IN ('instagram','facebook','youtube','google','other')),
  product_id   text REFERENCES products(id),
  name         text NOT NULL,
  status       text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','scheduled','live','ended')),
  budget_paise bigint NOT NULL DEFAULT 0,
  spend_paise  bigint NOT NULL DEFAULT 0,
  impressions  bigint NOT NULL DEFAULT 0,
  clicks       bigint NOT NULL DEFAULT 0,
  installs     bigint NOT NULL DEFAULT 0,
  post_url     text NOT NULL DEFAULT '',
  starts_on    date,
  ends_on      date
);

CREATE TABLE guides (
  id      text PRIMARY KEY,
  title   text NOT NULL,
  body_md text NOT NULL DEFAULT ''
);
