---
title: Configuration
order: 100
summary: Every setting the site reads, with defaults, and what happens when one is wrong.
since: 1.0
changed:
  - 1.9: Added MAIL_PROVIDER and SES_REGION, and the master admin's Email settings page, whose saved values win over these.
  - 1.9: SMTP_HOST is now required unless ALLOW_LOG_MAIL=true, so secret links are no longer logged by accident.
  - 1.9: Added DISABLE_SIGNUPS and MASTER_ADMIN_RESET_2FA; the master admin's sessions last 8 hours and can use two-step sign-in.
  - 1.9: Added ISSUE_EMAIL and ISSUE_MASKING for the Report a problem feature.
  - 1.9: Added the optional master admin account (MASTER_ADMIN_EMAIL and MASTER_ADMIN_PASSWORD).
  - 1.9: Password reset emails also depend on the mail settings.
  - 1.9: SESSION_KEY must now be at least 32 characters, or the site refuses to start.
---
# Configuration

For whoever runs the site (new to this? start with [run your own copy](run-your-own)). Settings are environment variables; there is no
configuration file. See `.env.template` in the repository for an annotated copy.

| Variable | Default | Meaning |
| --- | --- | --- |
| `SESSION_KEY` | none (**required**) | Secret of at least **32 characters** that signs form tokens. Generate one with `openssl rand -hex 32`. Keep it secret and different in every environment; set it once and reuse it on redeploys ([the signing key](run-your-own#the-signing-key)). |
| `PORT` | `8080` | Port to listen on. |
| `DB_PATH` | `/data/rafactools.db` | Where the SQLite database file lives. The directory must be writable. |
| `BASE_URL` | `http://localhost:8080` | The public address, used in links inside emails. A path in it (for example `https://example.org/planner`) becomes the prefix of every link the site generates. |
| `COOKIE_SECURE` | `true` | Mark cookies HTTPS-only and send the HSTS header. Set `false` **only** for plain-HTTP local use. |
| `CLAIM_DOMAIN` | `rafac.mod.gov.uk` | Email domain allowed to claim a squadron. |
| `SKIP_EMAIL_VERIFICATION` | `false` | Marks every claimed squadron as confirmed immediately. **Development only**: it removes the proof that the address is real. |
| `MASTER_ADMIN_EMAIL`, `MASTER_ADMIN_PASSWORD` | empty | The optional site-wide **master admin** (see below). Set both or neither. |
| `ISSUE_EMAIL` | empty | Where **problem reports** are emailed, for example `issues@your-domain`. Empty switches reporting off: no link appears and `/report` does not exist. |
| `ISSUE_MASKING` | `true` | `true` lets people black out parts of a screenshot before sending it. Set `false` only on a site you run for other people when you need to see exactly what they saw: the form then says plainly that screenshots are sent as they are. |
| `MAIL_PROVIDER` | `smtp` | A mail preset: `cloudflare`, `gmail`, `google-workspace` or `ses` fill in the host and port; `smtp` assumes nothing. See [set up email](operators-email). |
| `SES_REGION` | empty | Only for `MAIL_PROVIDER=ses`: the AWS region, for example `eu-west-2`. |
| `SMTP_HOST` | none (**required** unless `MAIL_PROVIDER` or the website supplies it, or `ALLOW_LOG_MAIL`) | Mail server. Without any, the site refuses to start (see below). |
| `ALLOW_LOG_MAIL` | `false` | **Development only.** Lets the site start without `SMTP_HOST`, writing the emails it would send, links included, to the log. |
| `DISABLE_SIGNUPS` | `false` | `true` closes the claim-a-squadron form (it answers `403` and the login page stops linking to it). People already signed up, invitations and password resets keep working. Use it on a copy that holds real data. |
| `MASTER_ADMIN_RESET_2FA` | `false` | Set to `true` for one start to clear the master admin's two-step sign-in if the authenticator app is lost; then remove it. |
| `SMTP_PORT` | `587` | Mail port. Port `465` uses TLS from the start; other ports use plain SMTP and upgrade with STARTTLS if the server offers it. |
| `SMTP_USER`, `SMTP_PASS` | empty | Mail login, if the server needs one. |
| `MAIL_FROM` | `no-reply@rafactools-planner.local` | Sender address on emails. |

A true/false setting that is not understood (for example `flase`) falls back to
its **default**, which for `COOKIE_SECURE` and `SKIP_EMAIL_VERIFICATION` is the
safe choice.

## When email is not configured

All the mail variables above are only **defaults**: the master admin can save different values on the website
(**Your details**, **Email settings**) and a saved value wins. See [set up email](operators-email).

The site **refuses to start** with no mail server from either source, because without mail it would write
each confirmation, invitation and password-reset link to its log, and anyone who can read the
log could use them. Set `ALLOW_LOG_MAIL=true` to start anyway on a development machine; the
messages, links included, then go to the log. Never set it where other people can read the log.

## Problem reports

With `ISSUE_EMAIL` set, every page has a **Report a problem** link and reports are emailed
through the same SMTP settings as the other emails; see [report a problem](report-a-problem) for
what people see and what a report contains. Notes for whoever receives them:

- An invalid `ISSUE_EMAIL` stops the site starting, with a message naming it.
- With `SMTP_HOST` empty the report, including its text, is written to the log instead.
- Each address can send 5 reports per 10 minutes; a report may be up to 5 MB (a screenshot up to 3 MB, so the email stays under the 5 MB some providers allow).
- Reports can be filed without signing in, so treat them as untrusted data: see [report a problem](report-a-problem).
- The server keeps the last 20 requests per browser session in memory (method, path with any
  secret code removed, and result code; never bodies, cookies or typed values) and the last 30
  error-looking log lines (emails and codes removed) so a report can include them. Nothing is
  written to disk.

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
the log as a line starting `AUDIT`, and the visit appears in that squadron's **security log**
(Squadron settings, owners only), so owners can see it. The master admin signs in for **8 hours**
(everyone else 30 days) and can turn on **two-step sign-in** (a code from an authenticator app)
on **Your details**. If the authenticator is lost, start the site once with
`MASTER_ADMIN_RESET_2FA=true`. See [security and privacy](security-and-privacy).

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
- `SMTP_HOST` is empty and `ALLOW_LOG_MAIL` is not `true`;
- `SMTP_PORT` is not a number, or `BASE_URL` is not a valid address;
- the database cannot be opened (for example the folder in `DB_PATH` does not exist);
- the port is already in use.

It also exits if its own built-in documentation is malformed.

## Changing the signing key

Changing `SESSION_KEY` makes any form that is open in someone's browser fail
once with `Form expired, try again.` Signed-in sessions are kept in the
database and stay valid.

Two things stored encrypted with a key derived from it can no longer be read after a change: the
email password saved on the website (enter it again) and the master admin's two-step sign-in secret
(start once with `MASTER_ADMIN_RESET_2FA=true` and set it up again).
