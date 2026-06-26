# migrations/

Forward-only, embedded SQL migrations (`G6`, `technical/08`).

- Naming: `NNNN_description.up.sql`, zero-padded sequential `NNNN` (e.g. `0001_init.up.sql`).
- A `.down.sql` may exist for local convenience but is **not** relied upon in production
  (forward-only is the contract).
- **Never edit an applied migration** — history is append-only; a correction is a new migration.
- Stay within the SQLite/PostgreSQL-portable subset (`technical/03` §1).

The first migration (`0001_init.up.sql`: `user`, `session`, `settings`, `schema_migrations`) and the
hand-rolled runner (pre-migration `VACUUM INTO` backup → transactional apply → abort-on-failure,
I-003) land in **increment 1**. At increment 0 this folder only carries the `embed.FS` home and this
note.
