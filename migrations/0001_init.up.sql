-- 0001_init — baseline schema for the auth core (technical/03 §2.1/§2.2/§4.5).
-- The schema_migrations tracking table is bootstrapped by the runner itself
-- (a migration-tracking table cannot migrate itself), so it is not created here.
-- Portable subset: INTEGER PRIMARY KEY AUTOINCREMENT, TEXT enums with CHECK,
-- TEXT ISO-8601 timestamps (technical/03 §1).

CREATE TABLE user (
  id                    INTEGER PRIMARY KEY AUTOINCREMENT,
  email                 TEXT    NOT NULL UNIQUE,
  password_hash         TEXT    NOT NULL,                          -- Argon2id PHC (technical/05 §1)
  is_admin              INTEGER NOT NULL DEFAULT 0,                -- boolean 0/1 (A9)
  status                TEXT    NOT NULL DEFAULT 'active'
                                CHECK (status IN ('active','deactivated')),
  language              TEXT    NOT NULL DEFAULT 'fr'
                                CHECK (language IN ('fr','en')),
  currency              TEXT    NOT NULL DEFAULT 'EUR',
  totp_enabled          INTEGER NOT NULL DEFAULT 0,
  totp_secret           TEXT,                                     -- base32 when 2FA on (inc 8)
  must_change_password  INTEGER NOT NULL DEFAULT 0,               -- admin/CLI temp reset (A3)
  failed_login_count    INTEGER NOT NULL DEFAULT 0,               -- lockout counter (A7/A12)
  last_failed_login_at  TEXT,
  locked_until          TEXT,
  created_at            TEXT    NOT NULL,
  updated_at            TEXT    NOT NULL
);

CREATE TABLE session (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
  token_hash    TEXT    NOT NULL UNIQUE,                          -- SHA-256 of the opaque cookie token
  kind          TEXT    NOT NULL CHECK (kind IN ('short','remember')),
  expires_at    TEXT    NOT NULL,
  created_at    TEXT    NOT NULL,
  last_seen_at  TEXT    NOT NULL,
  user_agent    TEXT,
  ip            TEXT
);

CREATE INDEX idx_session_user ON session(user_id);

CREATE TABLE settings (
  user_id                 INTEGER PRIMARY KEY REFERENCES user(id) ON DELETE CASCADE,
  -- default_account_id has no DB-level FK yet: the account table lands in
  -- increment 3 and SQLite cannot add an FK to an existing table. Referential
  -- integrity is enforced in the service layer until then (I-011, G6).
  default_account_id      INTEGER,
  pea_initial_deposit     INTEGER NOT NULL DEFAULT 0
                                  CHECK (pea_initial_deposit >= 0),              -- minor units
  pea_social_charge_rate  INTEGER NOT NULL DEFAULT 1720
                                  CHECK (pea_social_charge_rate BETWEEN 0 AND 9999),  -- basis points
  near_cap_threshold      INTEGER NOT NULL DEFAULT 9000
                                  CHECK (near_cap_threshold BETWEEN 0 AND 10000),     -- basis points
  secured_savings_basis   TEXT    NOT NULL DEFAULT 'all_planned'
                                  CHECK (secured_savings_basis IN ('all_planned','fixed_only')),
  comment_autoprefill     INTEGER NOT NULL DEFAULT 0,
  theme                   TEXT    NOT NULL DEFAULT 'light'
                                  CHECK (theme IN ('light','dark')),
  language                TEXT    NOT NULL DEFAULT 'fr'
                                  CHECK (language IN ('fr','en')),
  currency                TEXT    NOT NULL DEFAULT 'EUR',
  dsp2_enabled            INTEGER NOT NULL DEFAULT 0,             -- present-but-disabled (foundation §15)
  updated_at              TEXT    NOT NULL
);
