-- 0002_budget_core — account, category, envelope, allocation, transaction
-- (technical/03 §3). Created in FK-dependency order. ON DELETE defaults to
-- RESTRICT (L4); cross-column rules (type↔policy, no-cyclic-parent, flow_type
-- edit legality) are service-layer validations. `transaction` is a reserved
-- word, quoted throughout.

CREATE TABLE account (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id          INTEGER NOT NULL REFERENCES user(id),
  name             TEXT    NOT NULL,
  type             TEXT    NOT NULL CHECK (type IN ('current','passbook','securities','employee_savings')),
  month_end_policy TEXT    NOT NULL CHECK (month_end_policy IN ('sweep','carry','none')),
  fill_priority    INTEGER,                                       -- cascade order (savings only)
  ceiling          INTEGER CHECK (ceiling IS NULL OR ceiling >= 0),
  status           TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
  external_ref     TEXT,                                          -- DSP2 (anticipated)
  created_at       TEXT    NOT NULL,
  updated_at       TEXT    NOT NULL
);
CREATE UNIQUE INDEX idx_account_user_name ON account(user_id, name);
CREATE UNIQUE INDEX idx_account_user_fillprio ON account(user_id, fill_priority) WHERE fill_priority IS NOT NULL;
CREATE INDEX idx_account_user_status ON account(user_id, status);

CREATE TABLE category (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id          INTEGER NOT NULL REFERENCES user(id),
  name             TEXT    NOT NULL,
  parent_id        INTEGER REFERENCES category(id),
  flow_type        TEXT    NOT NULL CHECK (flow_type IN ('expense','income','transfer')),
  default_expanded INTEGER NOT NULL DEFAULT 0,
  status           TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
  created_at       TEXT    NOT NULL,
  updated_at       TEXT    NOT NULL
);
CREATE INDEX idx_category_user_parent ON category(user_id, parent_id);
CREATE INDEX idx_category_user_status ON category(user_id, status);

CREATE TABLE envelope (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id        INTEGER NOT NULL REFERENCES user(id),
  category_id    INTEGER NOT NULL REFERENCES category(id),
  account_id     INTEGER NOT NULL REFERENCES account(id),
  mode           TEXT    NOT NULL CHECK (mode IN ('fixed_recurring','variable','residual')),
  default_amount INTEGER CHECK (default_amount IS NULL OR default_amount >= 0),
  frequency      TEXT    CHECK (frequency IS NULL OR frequency IN ('monthly','quarterly','semiannual','annual')),
  due_months     TEXT,                                            -- CSV of month numbers (1-12)
  expected_day   INTEGER CHECK (expected_day IS NULL OR (expected_day BETWEEN 1 AND 31)),
  status         TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
  created_at     TEXT    NOT NULL,
  updated_at     TEXT    NOT NULL
);
CREATE UNIQUE INDEX idx_envelope_user_cat_acc ON envelope(user_id, category_id, account_id);
CREATE INDEX idx_envelope_user_acc_status ON envelope(user_id, account_id, status);

CREATE TABLE allocation (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id        INTEGER NOT NULL REFERENCES user(id),
  envelope_id    INTEGER NOT NULL REFERENCES envelope(id),
  period         TEXT    NOT NULL,                                -- 'YYYY-MM'
  planned_amount INTEGER NOT NULL CHECK (planned_amount >= 0),
  created_at     TEXT    NOT NULL,
  updated_at     TEXT    NOT NULL
);
CREATE UNIQUE INDEX idx_allocation_env_period ON allocation(envelope_id, period);
CREATE INDEX idx_allocation_user_period ON allocation(user_id, period);

CREATE TABLE "transaction" (
  id                    INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id               INTEGER NOT NULL REFERENCES user(id),
  account_id            INTEGER NOT NULL REFERENCES account(id),  -- source for transfers
  dest_account_id       INTEGER REFERENCES account(id),          -- transfers only
  category_id           INTEGER REFERENCES category(id),         -- NULL for transfers
  flow_type             TEXT    NOT NULL CHECK (flow_type IN ('expense','income','transfer')),
  amount                INTEGER NOT NULL CHECK (amount <> 0),     -- signed minor units
  op_date               TEXT,                                    -- ISO date; NULL while awaited
  budget_period         TEXT    NOT NULL,                        -- 'YYYY-MM'; independent of op_date
  status                TEXT    NOT NULL CHECK (status IN ('awaited','pending','cleared')),
  label                 TEXT    NOT NULL DEFAULT '',
  note                  TEXT,
  source                TEXT    NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','import')),
  external_ref          TEXT,                                    -- DSP2 dedup/reconcile key
  paired_transaction_id INTEGER REFERENCES "transaction"(id),    -- transfer auto-pairing
  created_at            TEXT    NOT NULL,
  updated_at            TEXT    NOT NULL
);
CREATE INDEX idx_txn_user_period ON "transaction"(user_id, budget_period);
CREATE INDEX idx_txn_user_acc_period ON "transaction"(user_id, account_id, budget_period);
CREATE INDEX idx_txn_user_cat_period ON "transaction"(user_id, category_id, budget_period);
CREATE INDEX idx_txn_user_status ON "transaction"(user_id, status);
CREATE INDEX idx_txn_external_ref ON "transaction"(external_ref);
CREATE INDEX idx_txn_paired ON "transaction"(paired_transaction_id);
