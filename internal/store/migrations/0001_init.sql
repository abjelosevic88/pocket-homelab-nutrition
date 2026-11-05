CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);

-- ---------- Pocket Nutrition: everything below is per person ----------

-- The foods a person logs: the built-in list (builtin = its key, copied
-- into each account the first time it is used, so it can be renamed or
-- corrected), their own, ones found by barcode on Open Food Facts, recipes
-- from Pocket Kitchen and ones the AI estimated. Nutrients are per 100 g
-- (unit g), per 100 ml (unit ml) or per serving (unit serving).
CREATE TABLE foods (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,                         -- "Greek yoghurt 2%"
  brand        TEXT NOT NULL DEFAULT '',
  builtin      TEXT NOT NULL DEFAULT '',              -- catalog key, '' = not from the built-in list
  barcode      TEXT NOT NULL DEFAULT '',              -- EAN / UPC
  category     TEXT NOT NULL DEFAULT 'other',         -- fruit | vegetables | grains | bread | dairy | eggs | meat | fish | legumes | nuts | fats | sweets | snacks | drinks | meals | other
  unit         TEXT NOT NULL DEFAULT 'g',             -- g | ml | serving: what the nutrients are counted per (100 g, 100 ml, 1 serving)
  kcal         REAL NOT NULL DEFAULT 0,
  protein      REAL NOT NULL DEFAULT 0,               -- grams
  carbs        REAL NOT NULL DEFAULT 0,
  fat          REAL NOT NULL DEFAULT 0,
  fiber        REAL,
  sugar        REAL,
  sat_fat      REAL,
  sodium       REAL,                                  -- milligrams
  servings     TEXT NOT NULL DEFAULT '[]',            -- JSON [{"label":"1 egg","amount":50}] in the food's unit
  favorite     INTEGER NOT NULL DEFAULT 0,
  archived     INTEGER NOT NULL DEFAULT 0,            -- hidden from search, the diary keeps it
  source       TEXT NOT NULL DEFAULT '',              -- '' by hand | builtin | off (Open Food Facts) | kitchen | ai | import | <app slug>
  source_ref   TEXT NOT NULL DEFAULT '',
  notes        TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX foods_name ON foods(user_id, name COLLATE NOCASE, brand COLLATE NOCASE);
CREATE INDEX foods_builtin ON foods(user_id, builtin);
CREATE INDEX foods_barcode ON foods(user_id, barcode) WHERE barcode <> '';
CREATE INDEX foods_source ON foods(user_id, source, source_ref) WHERE source_ref <> '';

-- The diary: what was eaten, meal by meal. The numbers are the entry's own
-- (worked out from the food when logged), so correcting a food later does
-- not rewrite the past; changing the amount works them out again.
CREATE TABLE entries (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date         TEXT NOT NULL,                         -- YYYY-MM-DD, local
  meal         TEXT NOT NULL DEFAULT 'snacks',        -- breakfast | lunch | dinner | snacks
  position     INTEGER NOT NULL DEFAULT 0,
  food_id      INTEGER REFERENCES foods(id) ON DELETE SET NULL,
  name         TEXT NOT NULL,                         -- the food's name when logged, or "Quick add"
  qty          REAL NOT NULL DEFAULT 1,               -- how many of the serving
  serving      TEXT NOT NULL DEFAULT '',              -- "1 egg", "100 g", "1 serving"
  amount       REAL NOT NULL DEFAULT 0,               -- in the food's unit, all servings together (150 g); 0 for quick adds
  unit         TEXT NOT NULL DEFAULT 'g',
  kcal         REAL NOT NULL DEFAULT 0,
  protein      REAL NOT NULL DEFAULT 0,
  carbs        REAL NOT NULL DEFAULT 0,
  fat          REAL NOT NULL DEFAULT 0,
  fiber        REAL,
  sugar        REAL,
  sat_fat      REAL,
  sodium       REAL,
  note         TEXT NOT NULL DEFAULT '',
  source       TEXT NOT NULL DEFAULT '',              -- '' by hand | quick | meal | copy | kitchen | ai | import | mfp | cronometer | <app slug>
  source_ref   TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX entries_user_date ON entries(user_id, date, meal, position);
CREATE INDEX entries_food ON entries(user_id, food_id);
CREATE UNIQUE INDEX entries_source ON entries(user_id, source, source_ref) WHERE source_ref <> '';

-- Saved meals: foods often eaten together ("My usual breakfast"), logged in one go.
CREATE TABLE meals (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  meal         TEXT NOT NULL DEFAULT '',              -- the meal it usually is, '' = any
  notes        TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX meals_user ON meals(user_id, name);

CREATE TABLE meal_items (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  meal_id      INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
  food_id      INTEGER NOT NULL REFERENCES foods(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL DEFAULT 0,
  qty          REAL NOT NULL DEFAULT 1,
  serving      TEXT NOT NULL DEFAULT '',
  amount       REAL NOT NULL DEFAULT 0
);
CREATE INDEX meal_items_meal ON meal_items(user_id, meal_id, position);

-- Water drunk per day, in millilitres.
CREATE TABLE water (
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date         TEXT NOT NULL,
  ml           INTEGER NOT NULL DEFAULT 0,
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (user_id, date)
);

-- A day's own words ("ate out, guessed"), and the last totals sent to
-- Pocket Health so a changed day is sent again.
CREATE TABLE days (
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date         TEXT NOT NULL,
  note         TEXT NOT NULL DEFAULT '',
  health_sent  TEXT NOT NULL DEFAULT '',              -- fingerprint of the totals Pocket Health has
  health_error TEXT NOT NULL DEFAULT '',
  habit_done   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, date)
);
