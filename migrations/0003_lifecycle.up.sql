-- 0003_lifecycle — month lifecycle (period) + unlock audit (period_event)
-- (technical/03 §4.1/§4.2). A period row exists only once "Créer le mois" is
-- validated; "not created" = no row. Transactions/allocations reference a period
-- by their budget_period/period string, not an FK to period.id (§6).

CREATE TABLE period (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES user(id),
  period     TEXT    NOT NULL,                                   -- 'YYYY-MM'
  state      TEXT    NOT NULL CHECK (state IN ('active','locked')),
  locked_at  TEXT,
  created_at TEXT    NOT NULL,
  updated_at TEXT    NOT NULL
);
CREATE UNIQUE INDEX idx_period_user_period ON period(user_id, period);

CREATE TABLE period_event (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES user(id),
  period        TEXT    NOT NULL,
  action        TEXT    NOT NULL CHECK (action IN ('create','lock','unlock')),
  at            TEXT    NOT NULL,
  actor_user_id INTEGER NOT NULL REFERENCES user(id)
);
CREATE INDEX idx_period_event_user_period ON period_event(user_id, period);
