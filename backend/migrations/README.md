# There is no SQL here

This directory used to contain six PostgreSQL migration files, roughly a thousand
lines describing forty-one tables. **They have been removed.**

They were never executed. There is no database driver in
`backend/go.mod`, no migration runner, and no Go code anywhere in the repository
that read, embedded, or applied any of it. They were an intended relational
schema written before the storage layer was settled, and they drifted away from
the code that actually runs.

They also contained defects that would have bitten anyone who tried to use them:

- `0004` re-adds a constraint whose name PostgreSQL had already auto-assigned
  from the column's inline `CHECK` in `0001`, which aborts the migration on a
  real database. Every other statement in the set was written defensively with
  `IF NOT EXISTS` or `DROP ... IF EXISTS`; that one was not.
- `0004` and `0006` use `CREATE TABLE IF NOT EXISTS` against tables that earlier
  files had already created. On a real database the **earlier** shape silently
  wins and every column the newer file adds simply never appears.
- `0006` reintroduces `webhooks` with the signing `secret` in plaintext, undoing
  the `secret_hash` that `0001` had. That is a credential regression.
- Three separate incompatible designs exist for `comments`, two for
  `team_invites` and `ballots`, and two for `webhooks`, with conflicting column
  sets. Which one the code implements is not discoverable from the SQL.

Shipping known-broken, unreferenced, misleading code is worse than not shipping
it. A reviewer who opens this directory should be able to trust what they find.

## Where the real schema is

**[`DATA-MODEL.md`](../../DATA-MODEL.md)**, which documents the schema that
actually runs: the Go types in `internal/domain`, their invariants, the
relationships between them, the versioned snapshot format used for persistence,
and the import and export paths.

If you are looking for the shape of the data, that is the document. If you are
looking for how it is stored on disk, it is section 3 of the same document.
