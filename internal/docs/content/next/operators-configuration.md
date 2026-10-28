---
title: Configuration
order: 100
summary: Every setting the site reads, with defaults, and what happens when one is wrong.
since: 1.0
changed:
  - next: Added the optional master admin account (MASTER_ADMIN_EMAIL and MASTER_ADMIN_PASSWORD).
  - next: Password reset emails also depend on the mail settings.
  - next: SESSION_KEY must now be at least 32 characters, or the site refuses to start.
---
# Configuration

For whoever runs the site (new to this? start with [run your own copy](run-your-own)). Settings are environment variables; there is no
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
| `MASTER_ADMIN_EMAIL`, `MASTER_ADMIN_PASSWORD` | empty | The optional site-wide **master admin** (see below). Set both or neither. |
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

## The master admin

An optional site-wide administrator who can look at **any** squadron, read-only, to
investigate problems people report. It exists only if you set both
`MASTER_ADMIN_EMAIL` and `MASTER_ADMIN_PASSWORD`.

**Put the password in a secret, never in a plain variable or a file in the
repository.** In GitHub, variables are stored and shown in plain text; only a
*secret* is protected. The email address is not secret and can be a variable.
Choose a long, random password (12 to 72 characters), and treat any password that
has been typed into a chat or an email as compromised.

How it works:

- **At every start** (so on every deployment) the site makes the database match
  these settings, and logs what it did, never the password: `master admin created`,
  `password updated; sessions signed out`, or `unchanged`.
- **Changing the password** in the settings updates the account and signs it out
  everywhere. **Changing the email** removes the previous master admin and creates
  the new one. There is never more than one.
- **It is never promoted from an ordinary account.** If the email already belongs to
  a member of a squadron, the site logs an error and does not create a master admin:
  remove that account first.
- **A weak password is refused** (under 12 characters, over 72, or empty): the site
  keeps running but logs `ERROR master admin NOT configured` and creates nothing.
  Check the log after a deployment.
- The account lives in a hidden squadron numbered `master`, which cannot clash with a
  real squadron and is not offered in the squadron list.
- It cannot be created from the website: not by claiming, inviting or editing members.

What the master admin can do: sign in like anyone else, open **Your details**, and
under **Switch squadron** pick any squadron. From then on, until they switch back
(or sign out), they see that squadron **exactly as a viewer would**: read-only, with
every edit refused, and a banner on every page says so. Each switch is written to
the log as a line starting `AUDIT`. Other people in the squadron are not told, so
see [security and privacy](security-and-privacy).

If someone resets the master admin's password by email, the next start puts the
configured password back.

## Password reset needs email

The reset link is sent by email like the confirmation and invitation links. With
`SMTP_HOST` empty the link is only written to the log, so users cannot reset their
passwords themselves: configure real mail wherever people need this. The link is
built from `BASE_URL`.

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
