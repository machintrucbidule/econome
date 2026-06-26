-- 0007_envelope_dest_account — add the internal-transfer destination to the
-- envelope template (T11, resolves O-14). Additive forward-only column: set only
-- for flow_type='transfer' envelopes, NULL otherwise. month-init's recurring
-- generator sends the awaited transfer account_id=source -> dest_account_id=dest
-- (rules §10). No DB-level FK is added on ALTER (SQLite cannot add an FK to an
-- existing table); referential integrity is service-enforced (cf. I-011/I-020).

ALTER TABLE envelope ADD COLUMN dest_account_id INTEGER;
