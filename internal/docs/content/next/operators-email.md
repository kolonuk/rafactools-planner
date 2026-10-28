---
title: Set up email
order: 105
summary: How to make the site send email (Cloudflare, Gmail, Google Workspace, Amazon SES or any SMTP server), the master admin's Email settings page and test button, and the DNS records that make receivers trust your mail.
since: next
---
# Set up email

For whoever runs the site. The planner sends email for account confirmation, invitations,
password resets and problem reports. It speaks plain **SMTP**, so any provider that gives you an
SMTP server, a login and a password works. Presets for the common ones mean you only supply what is
yours.

The site **will not start** with no mail server, because it would otherwise write sign-in and reset links
to its log where anyone who can read the log could use them. (For trying it out on one computer, set
`ALLOW_LOG_MAIL=true`.)

## Two places to set it

Every mail setting has two sources, and **the database wins**:

1. **The environment** (Docker or GitHub variables and secrets): the defaults the server starts with.
2. **The website:** the master admin's **Email settings** page (**Your details**, then **Email settings**).
   A value saved there replaces the environment's; an empty box means "use the server's value". The
   password is stored encrypted, is never shown again, and **Remove all saved email settings** goes back to
   the environment.

Settings saved on the website take effect on the next email, with no restart. The page also has
**Send a test email to me**, which sends to your own address with the settings in force and shows the
provider's error if it fails. Saving checks the whole set first and refuses a combination that cannot
work, so you cannot lock yourself out of sending mail.

If the environment has no mail server, start once with `ALLOW_LOG_MAIL=true`, sign in as the master admin,
save the settings and send a test, then remove `ALLOW_LOG_MAIL`. (Anyone who can read the log can see
links during that first run, so do it on a host only you can reach.)

| Variable | Meaning |
| --- | --- |
| `MAIL_PROVIDER` | `smtp` (default), `cloudflare`, `gmail`, `google-workspace` or `ses`. A preset fills in the host, port and (where fixed) the login name. |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER` | Set only to override a preset or for plain `smtp`. Anything set explicitly beats the preset. |
| `SMTP_PASS` | The password or token. **A secret**: never a plain variable. |
| `MAIL_FROM` | The address mail is sent as. Must be on a domain your provider has verified. |
| `SES_REGION` | Only for `ses`: the AWS region of your SES account, for example `eu-west-2`. |
| `ISSUE_EMAIL` | Where problem reports are sent (see [report a problem](report-a-problem)). |

Port `465` uses TLS from the first byte; other ports start in plain text and upgrade with STARTTLS.
The site never sends a password over an unencrypted connection.

## Cloudflare

Cloudflare's Email Service (sending, which was in public beta when this was written; check Cloudflare's
documentation for the current plan requirements) accepts SMTP on port 465.

1. In the Cloudflare dashboard, enable **Email Sending** and **onboard your domain**. If the domain's DNS
   is on Cloudflare it adds the SPF and DKIM records for you.
2. Create an **API token** with the **Email Sending: Edit** permission (an account-owned token is best).
3. Set:

   ```
   MAIL_PROVIDER=cloudflare
   SMTP_PASS=<the API token>           # a secret
   MAIL_FROM=no-reply@your-domain      # on the onboarded domain
   ```

   The preset uses `smtp.mx.cloudflare.net`, port `465` and the fixed login name `api_token`.
4. Notes: a message may be at most 5 MiB (so problem-report screenshots are limited to 3 MB);
   sending to arbitrary addresses needs a paid Workers plan; the sender must be on an onboarded domain,
   or the server answers `550 5.7.1`.

(Cloudflare **Email Routing** only *receives* mail. Use it to forward `issues@your-domain` to your own inbox;
it is separate from sending.)

## Gmail (a personal Google account)

Easiest to start with; not recommended for a busy site (Google limits a personal account to about
500 recipients a day and only lets you send as that account).

1. Turn on **2-Step Verification** for the Google account.
2. Create an **app password** (Google Account, Security, App passwords). Copy the 16 characters.
3. Set:

   ```
   MAIL_PROVIDER=gmail
   SMTP_USER=you@gmail.com
   SMTP_PASS=<the app password>        # a secret
   ```

   `MAIL_FROM` defaults to your address (Gmail replaces any other). The preset uses `smtp.gmail.com`, port `465`.

## Google Workspace

Two ways. Use the first unless your administrator has turned off app passwords.

**A. A Workspace user with an app password:** exactly as Gmail above, with the user's full Workspace
address as `SMTP_USER`. The user needs 2-Step Verification on and app passwords allowed by the administrator.
Mail is sent as that user (or an alias they own).

**B. The Workspace SMTP relay** (best for sending as `no-reply@your-domain`):

1. In the **Google Admin console**: Apps, Google Workspace, Gmail, Routing, **SMTP relay service**, Add.
2. Allowed senders: **Only addresses in my domains**. Authentication: either **allow your server's fixed
   public IP address** or **Require SMTP Authentication**. Tick **Require TLS encryption**.
3. Set:

   ```
   MAIL_PROVIDER=google-workspace
   MAIL_FROM=no-reply@your-domain
   ```

   The preset uses `smtp-relay.gmail.com`, port `587`. If you chose SMTP authentication, also set `SMTP_USER`
   and `SMTP_PASS` (a Workspace user and its app password).
4. Authenticate the domain in the Admin console: Gmail, **Authenticate email** to generate the DKIM record,
   and add Google's SPF include (see "Making receivers trust your mail").

## Amazon SES

1. In the **SES console**, in the region you will use, **verify your domain** (Easy DKIM adds the DKIM
   records) and request **production access**: until you have it, SES only sends to verified addresses.
2. Create **SMTP credentials** (SES, SMTP settings, Create SMTP credentials). These are a separate
   username and password, not your AWS access keys.
3. Set:

   ```
   MAIL_PROVIDER=ses
   SES_REGION=eu-west-2                # the region you verified in
   SMTP_USER=<SMTP username>
   SMTP_PASS=<SMTP password>           # a secret
   MAIL_FROM=no-reply@your-domain      # on the verified domain
   ```

   The preset uses `email-smtp.<region>.amazonaws.com`, port `465`.
4. Optional: set a **custom MAIL FROM domain** in SES so SPF lines up with your domain.

## Any other SMTP server

Brevo, Mailgun, Postmark, Resend, Fastmail and others all offer SMTP:

```
MAIL_PROVIDER=smtp
SMTP_HOST=smtp.example.net
SMTP_PORT=587          # or 465
SMTP_USER=...
SMTP_PASS=...          # a secret
MAIL_FROM=no-reply@your-domain
```

Sending straight from your own server instead of a provider is possible but rarely worth it: home and
cloud IP addresses are often distrusted, port 25 is frequently blocked, and you need reverse DNS set up.

## Making receivers trust your mail

These are DNS records on your domain. Your provider shows the exact values.

1. **SPF:** one `TXT` record on the domain naming who may send for it, such as
   `v=spf1 include:<provider's value> ~all`. A domain can only have one SPF record, so merge them.
2. **DKIM:** the provider gives you a `CNAME` or `TXT` record and then signs your mail. This matters most.
3. **DMARC:** a `TXT` record at `_dmarc.your-domain`. Start with
   `v=DMARC1; p=none; rua=mailto:issues@your-domain` and move to `p=quarantine` once SPF and DKIM pass.
4. **Matching From address:** keep `MAIL_FROM` on the domain you verified.
5. **Check it:** send a test to a Gmail address, choose **Show original**, and look for SPF, DKIM and
   DMARC all saying `PASS`.

The site adds a `Date` and a unique `Message-ID` to every message, and encodes non-English characters in
subjects, which receivers expect.

## When it does not work

| The test says | Likely cause |
| --- | --- |
| `MAIL_PROVIDER=... needs SMTP_PASS` / `SMTP_USER` / `MAIL_FROM` / `SES_REGION` | A required value for that provider is missing; the page names it and does not save. |
| `535` or `authentication failed` | Wrong login or password. Gmail needs an app password, not your normal one; SES needs SMTP credentials, not access keys; Cloudflare's login is exactly `api_token`. |
| `550 5.7.1` or `not authorized to send as` | `MAIL_FROM` is not on a domain the provider has verified (or is not your Gmail address). |
| `timed out` or `connection refused` | The host or port is wrong, or your server's firewall blocks outgoing mail ports. |
| Arrives in spam | SPF, DKIM or DMARC is missing or does not match `MAIL_FROM`. |
| `stored secret cannot be decrypted` in the log | `SESSION_KEY` was changed after the password was saved on the website. Save the password again. |

## Security notes

- The password saved on the website is encrypted with a key derived from `SESSION_KEY`, so a copy of the
  database file alone does not reveal it. Changing `SESSION_KEY` means entering it again.
- Only the master admin can open the page, every change is written to the log and shown under **Recent changes**
  (what changed, never the values), and the password is never sent back to a browser.
- Prefer the environment for the password where you can: a secret in GitHub or Docker is not in the database
  at all.
