---
title: Configuration
order: 100
summary: Every setting the site reads, with defaults, and what happens when one is wrong.
since: 1.0
changed:
  - next: SESSION_KEY must now be at least 32 characters, or the site refuses to start.
---
# Configuration

For whoever runs the site. Settings are environment variables; there is no
configuration file. See `.env.template` in the repository for an annotated copy.

| Variable | Default | Meaning |
| --- | --- | --- |
| `SESSION_KEY` | none (**required**) | Secret of at least **32 characters** that signs form tokens. Generate one with `openssl rand -hex 32`. Keep it secret and different in every environment. |
| `PORT` | `8080` | Port to listen on. |
| `DB_PATH` | `/data/rafactools.db` | Where the SQLite database file lives. The directory must be writable. |
| `BASE_URL` | `http://localhost:8080` | The public address, used in links inside emails. A path in it (for example `https://example.org/planner`) becomes the prefix of every link the site generates. |
| `COOKIE_SECURE` | `true` | Mark cookies HTTPS-only and send the HSTS header. Set `false` **only** for plain-HTTP local use. |
| `CLAIM_DOMAIN` | `rafac.mod.gov.uk` | Email domain allowed to claim a squadron. |
| `SKIP_EMAIL_VERIFICATION` | `false` | Marks every claimed squadron as confirmed immediately. **Development only**: it removes the proof that the address is real. |
| `SMTP_HOST` | empty | Mail server. Empty means no email is sent (see below). |
| `SMTP_PORT` | `587` | Mail port. Port `465` uses TLS from the start; other ports use plain SMTP and upgrade with STARTTLS if the server offers it. |
| `SMTP_USER`, `SMTP_PASS` | empty | Mail login, if the server needs one. |
| `MAIL_FROM` | `no-reply@rafactools-planner.local` | Sender address on emails. |

A true/false setting that is not understood (for example `flase`) falls back to
its **default**, which for `COOKIE_SECURE` and `SKIP_EMAIL_VERIFICATION` is the
safe choice.

## When email is not configured

With `SMTP_HOST` empty the site does not send mail. It writes the message,
**including the confirmation or invitation link**, to its log instead. That is
useful on a development machine. Anywhere else, treat the log as sensitive or
configure real mail.

## Start-up failures

The site checks its settings when it starts and exits with a non-zero status
and a message naming the problem if:

- `SESSION_KEY` is missing or shorter than 32 characters;
- `SMTP_PORT` is not a number, or `BASE_URL` is not a valid address;
- the database cannot be opened (for example the folder in `DB_PATH` does not exist);
- the port is already in use.

It also exits if its own built-in documentation is malformed.

## Changing the signing key

Changing `SESSION_KEY` makes any form that is open in someone's browser fail
once with `Form expired, try again.` Signed-in sessions are kept in the
database and stay valid.
