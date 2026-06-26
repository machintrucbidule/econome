-- 0004_networth — savings_snapshot (gross monthly values) + networth_month (one
-- comment per month) (technical/03 §4.3/§4.4). pea_net/subtotals/deltas are
-- derived by the engine, never stored. Snapshots are always editable,
-- independent of the budget month lock (L7).

CREATE TABLE savings_snapshot (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES user(id),
  account_id  INTEGER NOT NULL REFERENCES account(id),
  period      TEXT    NOT NULL,                                  -- 'YYYY-MM'
  gross_value INTEGER NOT NULL CHECK (gross_value >= 0),
  created_at  TEXT    NOT NULL,
  updated_at  TEXT    NOT NULL
);
CREATE UNIQUE INDEX idx_snapshot_acc_period ON savings_snapshot(account_id, period);
CREATE INDEX idx_snapshot_user_period ON savings_snapshot(user_id, period);

CREATE TABLE networth_month (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES user(id),
  period     TEXT    NOT NULL,                                   -- 'YYYY-MM'
  comment    TEXT    NOT NULL DEFAULT '',
  created_at TEXT    NOT NULL,
  updated_at TEXT    NOT NULL
);
CREATE UNIQUE INDEX idx_networth_month_user_period ON networth_month(user_id, period);
