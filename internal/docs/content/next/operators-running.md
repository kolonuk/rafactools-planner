---
title: Running the site
order: 110
summary: Starting, stopping, checking health, and running behind a proxy.
since: 1.0
changed:
  - 1.9: The health check now fails (503) when the database does not answer.
  - 1.9: Every response carries an X-Request-ID, and connections to the mail server have timeouts.
---
# Running the site

For whoever runs the site.

## Run it locally

`docker compose up` in the repository starts the planner on port 8080 and a
mail catcher (Mailpit) whose web page, on port 8025, shows the emails the
planner sends. Local settings use plain HTTP, so `COOKIE_SECURE` is `false`
there. See [configuration](operators-configuration).

## The container

The image holds one program and nothing else (no shell). It runs as an
unprivileged user and keeps its data in `/data`, which must be a volume so it
survives restarts: see [backups](operators-backups). The web pages, styles and
these docs are built into the program, so there are no other files to deploy.

## Health check

`GET /health` returns `{"status": "ok"}` with status 200 without signing in. Use
it for load-balancer and container health checks. It also asks the database a trivial
question: if that fails the answer is status `503` with `{"status": "database unavailable"}`, so a
stuck or unreadable database takes the container out of service instead of looking healthy.

## Stopping

The site shuts down cleanly on `SIGTERM` or `SIGINT` (what `docker stop` and
Ctrl-C send): it stops taking new connections, lets running requests finish for
up to 10 seconds, and exits with status 0.

## Behind a reverse proxy

- The site expects a proxy to add the real client address in `X-Forwarded-For`.
  The **last** address in that header is used to count login attempts, so the
  proxy in front must be one you control and must append to the header.
  If there is no proxy the connection's own address is used.
- To serve the planner under a path such as `/planner`, set `BASE_URL` to
  include it and have the proxy strip the prefix before passing requests on. The
  site then writes the prefix into every link and form it generates.
- Terminate HTTPS at the proxy and keep `COOKIE_SECURE=true`.

## Connection limits

Slow clients are dropped: headers must arrive within 5 seconds, and a whole
request within 15. Request bodies are limited to 1 MB.

## Request references

Every response carries an `X-Request-ID` header (12 hex characters). Error pages show it as "reference ...", it is
in the page's `<meta name="request-id">`, it is listed with each request in a problem report, and a server error is logged as
`request <id> failed: <status> <title>`. Searching the log for a reference finds that request. Have your reverse proxy pass the
header through, or log it, if you want to match proxy logs too.
