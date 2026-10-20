# SKY Support API server (rustdesk-api)

The account, address-book and log server of **SKY Support** (<https://github.com/skycobg/SKY-Support>), a
fork of [lejianwen/rustdesk-api](https://github.com/lejianwen/rustdesk-api) v2.7 (MIT). Our changes live on
the `sky` branch:

- Audit posts (`/api/audit/conn`, `/api/audit/file`) answer with an empty body: the RustDesk client takes only
  that as stored, and posted every record three times before.
- A connection's update and close go to its newest still-open row (conn_id restarts with every client start);
  the session id is kept exactly (a u64; as float64 it lost digits).
- Notes: on address-book entries (the client's "Edit note") and on connection logs: the end-of-session note
  of the controlling client (`GET /api/audit/conn/active`, `PUT /api/audit`), with the user who wrote it.
  Database version 266 adds the columns.
- Build: `.github/workflows/sky-build.yml` (x86_64 Linux, static musl).
