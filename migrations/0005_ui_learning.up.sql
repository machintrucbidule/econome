-- 0005_ui_learning — label_mapping (autocomplete M21 + learned categorisation,
-- reused by the future DSP2 import) + ui_preference (per-user expand state, M4)
-- (technical/03 §5.1/§5.2).

CREATE TABLE label_mapping (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES user(id),
  label        TEXT    NOT NULL,
  label_key    TEXT    NOT NULL,                                 -- normalised for matching
  category_id  INTEGER REFERENCES category(id),
  account_id   INTEGER REFERENCES account(id),
  usage_count  INTEGER NOT NULL DEFAULT 1,
  last_used_at TEXT    NOT NULL
);
CREATE INDEX idx_label_user_key ON label_mapping(user_id, label_key);
CREATE INDEX idx_label_user_usage ON label_mapping(user_id, usage_count);

CREATE TABLE ui_preference (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES user(id),
  node_type  TEXT    NOT NULL CHECK (node_type IN ('category','envelope')),
  node_id    INTEGER NOT NULL,
  expanded   INTEGER NOT NULL,
  updated_at TEXT    NOT NULL
);
CREATE UNIQUE INDEX idx_uipref_user_node ON ui_preference(user_id, node_type, node_id);
