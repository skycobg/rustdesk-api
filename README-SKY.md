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
  Database version 266 adds the columns. A note is accepted only from the device that controlled the
  connection (the device id of the caller's access token = the row's from_peer), while it is open or up to an
  hour after, and never over another user's note.
- Admin: the connection log can be filtered by technician (`from_name`) and dates (`from`, `to`);
  `GET /api/admin/audit_conn/summary` gives sessions and total time per technician.
- Login alert, in place of a ban by IP (the office goes out through one IP): an email to `app.login-alert-to`
  after `app.login-alert-threshold` wrong passwords within `app.login-alert-window` (all users and IPs together,
  in the program and in the web panel), over the SMTP server in `mail` (STARTTLS on 587 or TLS on 465). It lists
  them (time, user and whether it exists, IP, program or panel, device); the next email comes an hour later at
  the earliest and lists everything that came meanwhile. Off unless set (`utils/sky_login_alert.go`).
- Logins ignore case and spaces in the username: usernames are stored lower case and without spaces
  (`formatUsername`), so "Niki" logs in as niki; the check for an existing name does the same.
- Help requests (`/api/sky/help`, database version 267): the customer phones SKY and presses the button in SKY
  Support (started by the SKY Zapper app or another of our programs); SKY Support sends its ID, its one-time password
  and what it knows about the computer and the program. Without a login: `GET /api/sky/help/config` (the phones),
  `POST /api/sky/help` (at most 5 per IP in 10 minutes; a new one from the same ID cancels the waiting one),
  `POST /api/sky/help/status` (every few seconds while the customer waits, with the current password; 45 s of
  silence and the request expires) and `/cancel`. Logged in: `GET /api/sky/help/pending` (no passwords) and
  `POST /api/sky/help/accept` (the first technician gets the ID and the password, the others "taken by"); only admins and
  the group `app.sky-help-group` (default SKY) may use them. A new request cancels a waiting one only from the same
  machine (ID and uuid). The password
  is wiped once the technician is in (the customer reports a new one-time password after the request was accepted:
  RustDesk changes it as soon as it has let someone in), when the request ends, and 10 minutes after it was accepted
  at the latest.
  Settings `app.sky-help-*`: the phones, the waiting time and an email when nobody accepts in time.
- Build: `.github/workflows/sky-build.yml` (x86_64 Linux, static musl).
