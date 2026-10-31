---
title: Running the site
order: 110
summary: Starting, stopping, checking health, and running behind a proxy.
since: 1.0
audience: dev
changed:
  - 1.13: The help pages under /docs are public; the page lists which pages need no sign-in.
  - 1.9: The health check now fails (503) when the database does not answer.
  - 1.9: Every response carries an X-Request-ID, and connections to the mail server have timeouts.
---
# Running the site

For whoever runs the site.

## Trying it locally

`docker compose up` starts the planner on port 8080 and a mail catcher (Mailpit) on
port 8025. The steps are in [run your own copy](run-your-own#try-it-on-your-own-computer).

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

## Pages that need no sign-in

Besides `/health`, these are public: the login, claim, forgot-password and privacy pages, the
help pages under `/docs` (what you are reading), and the site's static files. If you put another
login in front of the planner (for example at the proxy), exempt `/docs` and `/health` if you want
the help and health checks to keep working for people and tools that are not signed in. The help
pages contain instructions only, never squadron data.

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
