---
title: Security and privacy
order: 70
summary: What the planner stores, how it is protected and what it does not do.
since: 1.0
audience: user
changed:
  - 1.13: The help pages are public (no sign-in needed); this page now states that and corrects the list of what the planner does not do.
  - 1.13: Optional Cloudflare Turnstile human check on login, claim and delete-squadron.
  - 1.9: The security log also records sign-ins and password changes; owners can download it and are emailed when the master admin looks at their squadron.
  - 1.9: The site's email password can be saved on the website by the master admin; it is stored encrypted.
  - 1.9: Added a privacy notice page, a security log for owners, the list of places you are signed in, deleting a squadron, and two-step sign-in for the master admin.
  - 1.9: Added Report a problem, which can email a screenshot and recent activity to the site's operators.
  - 1.9: Documented the optional master admin, who can view any squadron read-only.
  - 1.9: Added the optional displayed name and the instructor list (name and rank only).
  - 1.9: The rate limit now uses the address the proxy saw, so it cannot be dodged by sending a fake forwarding header.
  - 1.9: Added password reset; reset links are stored only as hashes.
  - 1.9: Unknown email addresses now take as long to reject as real ones, so response time does not reveal which addresses have accounts.
  - 1.9: Sign-in sessions and form tokens are unchanged, but the site refuses to start with a weak signing key.
  - 1.9: The browser is now allowed to run the site's own scripts (previously it was told to refuse all scripts); inline, third-party and eval scripts are still refused.
---
# Security and privacy

## What is stored

| Data | Detail |
| --- | --- |
| Accounts | Email address, role, and a salted bcrypt hash of the password. The password itself is never stored. |
| Sign-in sessions | A random token per browser, valid for 30 days (8 hours for the master admin) or until logout, with when it started and a coarse description of the browser such as "Firefox on Linux". |
| Email settings | If the master admin saves them on the website: the mail server, login, sender and password. The password is **encrypted** (with a key derived from the site's signing key) and never shown again. |
| Security log | When the master admin looks at a squadron; sign-ins and wrong passwords for an existing account; password changes and resets; signing out other sessions; and invites, role changes, removals and settings changes: who, what and when (never a password). Kept for a year; owners can read and download their squadron's. |
| Password reset requests | A SHA-256 hash of the emailed link, who it is for, and when it expires (1 hour). Deleted when used or replaced. |
| Problem reports | **Not stored.** A report you send is emailed to the site's operators, and the server briefly remembers your last 20 requests (method, path and result code only, in memory) so that a report can include them. See [report a problem](report-a-problem). |
| Names | The name each person chooses to display (optional), and your squadron's instructor list: a name and a rank each, no contact details. |
| Programmes | Everything you type: activities, instructor names, notes, closures, weekend notes. |
| Requirements tracker | **Cadets' names** and notes. |
| Squadron | Name, subtitle, protective marking, classifications, parade nights. |

The site makes no requests to third parties: no analytics, no external fonts, no
tracking. The only things it sends out are emails, through the mail server the site
is set up with: confirmation, invitation and password reset emails, temporary passwords, the notice to your old address when you change your email address, the notice to owners when the site's administrator looks at their squadron, and the problem reports you choose to send. See
[getting started](getting-started), [roles and invites](roles-and-invites) and
[signing in](signing-in).

**Do not put information in the planner that you would not be happy to see on
a printed noticeboard.** Check what your unit's rules allow before entering
names, medical notes or contact details.

The same facts are on the public [privacy notice](app:/privacy) page.

## Who can see it

**The help pages** (what you are reading) are public: anyone can read them without
signing in. They describe how the planner works and contain no squadron data.
The source code is public too, so the planner's security rests on its design and on your
secrets (such as the signing key and passwords), never on how it works being hidden.

**Your squadron's data** is visible only to signed-in members of the same squadron. Every page and every change is
checked against the signed-in person's squadron, so another squadron cannot
open or alter your programmes, even by guessing addresses. People running the
site can read the database.

The site can also have a **master admin**, a single site-wide account that can look
at any squadron to investigate problems people report. It sees a squadron exactly
as a viewer would (read-only; it cannot change anything), the page says so on every
screen, and each time it switches squadron the site logs it and adds it to that squadron's
security log, which its owners can read. If you are not
comfortable with that, ask the person who runs the site whether it is enabled.
See [configuration](operators-configuration).

## Protections

- **Passwords** are hashed with bcrypt; 12 to 72 characters are required.
- **Sessions** are stored on the server. Logging out deletes them. Changing or
  resetting your password signs out your other sessions. **Your details** lists every
  place you are signed in and can sign any of them out, or all but this one.
- **Two-step sign-in** is available to the master admin, the account that can read every
  squadron: after the password it asks for a 6-digit code from an authenticator app. Each code
  works once, the authenticator secret is stored encrypted, five wrong codes lock the account for 15 minutes, and turning it off needs the
  password and a current code.
- **Deleting a squadron:** an owner can delete the whole squadron and everything in it (members,
  programmes, the tracker with cadets' names, instructors, its security log) from Squadron
  settings. They must type the squadron number and their password. It cannot be undone.
- **Password reset links** work once, expire after 1 hour, and only a scrambled
  (SHA-256) copy of each link is kept, so a copy of the database cannot be used
  to reset anyone's password. Only one link is live per person, a second request
  within a minute sends nothing, and the page never reveals whether an address has
  an account. Resetting a password signs that person out everywhere.
- **Forms** carry a token that is checked on every change, which stops other
  websites making your browser submit changes.
- **Cookies** are marked HTTP-only and same-site; they are also marked secure
  (HTTPS only) on a correctly configured site.
- **Rate limiting:** the login, claim, invite and password reset pages share a
  limit of 20 attempts per address in five minutes. After that they answer `Too many attempts` until the
  window passes.
- **Scripts:** the site tells the browser to run only scripts that come from
  the site itself. Scripts written into a page (inline), scripts from other
  websites and `eval` are all refused, which limits the damage if someone
  manages to put text into a page.
- **Pages are not cached** by the browser, so a shared computer does not keep
  old pages.
- **Request size** is limited to 1 MB.

## Human check (optional)

When the operator sets `TURNSTILE_SITE_KEY` and `TURNSTILE_SECRET`, the login, claim and
delete-squadron forms require a [Cloudflare Turnstile](https://developers.cloudflare.com/turnstile/)
check. The server sends the check result and the visitor's address to Cloudflare to verify it, and
refuses the request if verification fails, times out (5 seconds) or cannot be reached. Only then are
Cloudflare's scripts allowed by the page's content security policy. It reduces automated
sign-up and password guessing; it does not replace the password, the rate limits or the email check.
With the keys unset, none of this applies.

## What it does not do

- No two-step sign-in for ordinary accounts (only the master admin can turn it on).
- No full audit trail. The security log records sign-ins, wrong passwords, password
  changes and membership and settings changes, and each programme's **History** records
  who saved each version, but there is no record of every individual edit.
- No automatic deletion of old programmes. Owners can delete a whole squadron, or remove a
  person, from [squadron settings](squadron-settings); nothing else removes data by itself.

For anything the screens cannot do, ask the person who runs the site (see
[backups](operators-backups) for where the data lives). Also read
[known limits](known-limits).

## Reporting a problem

If you think you have found a security problem, tell the person who runs
the site, not the rest of your squadron.
