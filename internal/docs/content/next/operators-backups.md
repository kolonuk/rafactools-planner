---
title: Backups
order: 120
summary: Where the data lives and how to protect it.
since: 1.0
audience: dev
changed:
  - next: Owners can remove members and delete a squadron from squadron settings, so removing data no longer means editing the database.
---
# Backups

For whoever runs the site. **The planner does not make backups for you.**

## Where the data is

Everything is in one SQLite database file, at `DB_PATH` (default
`/data/rafactools.db`). The database runs in write-ahead mode, so while the site
is running you will also see `rafactools.db-wal` and `rafactools.db-shm` beside it.
Recent changes may be in the `-wal` file, so copying only the main file of a
running site can lose them.

## Taking a backup

Either of these gives a consistent copy:

- Stop the site, then copy the database file together with any `-wal` and `-shm`
  files; or
- With the `sqlite3` tool installed, run
  `sqlite3 /data/rafactools.db ".backup 'backup.db'"`, which is safe while the
  site is running.

Store copies somewhere other than the server, and protect them: they contain
email addresses, password hashes and cadets' names.

## Restoring

Stop the site, replace the database file with the backup, delete any old
`-wal` and `-shm` files next to it, and start the site again. Try a restore
before you need one.

## Removing someone's data

Owners can do this themselves in [squadron settings](squadron-settings): **Remove**
a member, or **Delete this squadron** to remove every member, programme, tracker entry,
instructor and log entry. That is the preferred route, because it cannot leave
half-deleted data behind.

A deletion does not reach existing backups. Anything deleted in the planner is still in
older copies of the database file, so delete or age out old backups too if you have promised
that someone's data is gone. See [security and privacy](security-and-privacy).
