-- 0006_auth_extra — invitation (single-use expiring tokens) + totp_backup_code
-- (single-use 2FA recovery codes) (technical/03 §2.3/§2.4). The auth flows that
-- consume them are built in increment 8; the tables land now so nothing reshapes.

CREATE TABLE invitation (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  email            TEXT,                                         -- optional pre-fill
  token_hash       TEXT    NOT NULL UNIQUE,                      -- SHA-256 of the single-use token
  invited_is_admin INTEGER NOT NULL DEFAULT 0,
  created_by       INTEGER NOT NULL REFERENCES user(id),
  expires_at       TEXT    NOT NULL,                             -- 7 days (A12)
  consumed_at      TEXT,                                         -- single-use
  revoked_at       TEXT
);
CREATE INDEX idx_invitation_created_by ON invitation(created_by);

CREATE TABLE totp_backup_code (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
  code_hash   TEXT    NOT NULL,
  consumed_at TEXT
);
CREATE INDEX idx_totp_backup_user ON totp_backup_code(user_id);
