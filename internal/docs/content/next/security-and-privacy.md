---
title: Security and privacy
order: 70
summary: What the planner stores, how it is protected and what it does not do.
since: 1.0
changed:
  - next: The rate limit now uses the address the proxy saw, so it cannot be dodged by sending a fake forwarding header.
  - next: Unknown email addresses now take as long to reject as real ones, so response time does not reveal which addresses have accounts.
  - next: Sign-in sessions and form tokens are unchanged, but the site refuses to start with a weak signing key.
  - next: The browser is now allowed to run the site's own scripts (previously it was told to refuse all scripts); inline, third-party and eval scripts are still refused.
---
# Security and privacy

## What is stored

| Data | Detail |
| --- | --- |
| Accounts | Email address, role, and a salted bcrypt hash of the password. The password itself is never stored. |
| Sign-in sessions | A random token per browser, valid for 30 days or until logout. |
| Programmes | Everything you type: activities, instructor names, notes, closures, weekend notes. |
| Requirements tracker | **Cadets' names** and notes. |
| Squadron | Name, subtitle, protective marking, classifications, parade nights. |

Nothing is sent to any other service. The site makes no requests to third
parties: no analytics, no external fonts, no tracking. The only email it sends
is the confirmation and invitation emails described in
[getting started](getting-started) and [roles and invites](roles-and-invites).

**Do not put information in the planner that you would not be happy to see on
a printed noticeboard.** Check what your unit's rules allow before entering
names, medical notes or contact details.

## Who can see it

Only signed-in members of the same squadron. Every page and every change is
checked against the signed-in person's squadron, so another squadron cannot
open or alter your programmes, even by guessing addresses. People running the
site can read the database.

## Protections

- **Passwords** are hashed with bcrypt; 12 to 72 characters are required.
- **Sessions** are stored on the server. Logging out deletes them.
- **Forms** carry a token that is checked on every change, which stops other
  websites making your browser submit changes.
- **Cookies** are marked HTTP-only and same-site; they are also marked secure
  (HTTPS only) on a correctly configured site.
- **Rate limiting:** the login, claim and invite pages allow 20 attempts per
  address in five minutes. After that they answer `Too many attempts` until the
  window passes.
- **Scripts:** the site tells the browser to run only scripts that come from
  the site itself. Scripts written into a page (inline), scripts from other
  websites and `eval` are all refused, which limits the damage if someone
  manages to put text into a page.
- **Pages are not cached** by the browser, so a shared computer does not keep
  old pages.
- **Request size** is limited to 1 MB.

## What it does not do

- No two-factor sign-in.
- No audit trail of who changed what.
- No way to delete a squadron or a person from inside the planner.
- No automatic deletion of old programmes.

Ask the person who runs the site if you need data removed (see
[backups](operators-backups) for where it lives). Also read
[known limits](known-limits).

## Reporting a problem

If you think you have found a security problem, tell the person who runs
the site, not the rest of your squadron.
