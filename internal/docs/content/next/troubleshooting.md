---
title: Troubleshooting
order: 90
summary: What the messages mean and what to do.
since: 1.0
changed:
  - next: Error pages show a reference to quote.
  - next: Added the email settings and test email messages.
  - next: Added the sign-up closed, authenticator code and delete-squadron messages, and the site refusing to start without mail.
  - next: Curly quotes and accents no longer print as odd symbols; other scripts print as a question mark.
  - next: Added the password reset, change password, last-owner, website, classification and report messages.
---
# Troubleshooting

| What you see | What it means | What to do |
| --- | --- | --- |
| `Invalid email or password.` | The email or password is wrong. | Check both, or use **Forgot your password?**: see [signing in](signing-in). |
| `That password reset link is invalid or has expired.` | The link was used, is older than 1 hour, or a newer one replaced it. | Ask for a new link on the [reset page](app:/forgot). |
| `Confirm your email first` | You claimed a squadron but never opened the confirmation link. | Open the link in the email (valid 24 hours). If it expired, ask whoever runs the site. |
| `Form expired, try again.` | The page you were on is too old, or you were signed out. | Reload the page and repeat the change. |
| `your current password is not correct` | The **current password** you typed on **Your details** is wrong. | Retype it, or [reset it by email](signing-in) if you have forgotten it. |
| `these classifications still have text in your programmes, so they can't be removed yet: …` | You removed a classification from [squadron settings](squadron-settings) but some programme still has text in its column. | Clear that text in the named columns, or keep the classification. |
| `Too many attempts` | Too many tries from your network in five minutes. | Wait five minutes. Many people behind one network share the limit. |
| `email must be your squadron's snco.<number>@ or oc.<number>@ address` | Only those addresses can claim a squadron. | Use your squadron's own address, or ask the owner for an [invite](roles-and-invites). |
| `that squadron number has already been claimed` | Someone from your squadron already did. | Ask them to invite you. |
| `password must be at least 12 characters` / `at most 72 characters` | The password length is outside the allowed range. | Choose a password of 12 to 72 characters. |
| `a programme for YYYY-MM already exists` | That month is already on your dashboard. | Open it from the [dashboard](app:/). |
| `That invite link is invalid or has expired.` | The invite was used, revoked or is older than 7 days. | Ask an owner for a new one. |
| `Viewers can't make changes.` | Your role is viewer. | Ask an owner to invite you again as an editor: see [roles and invites](roles-and-invites). |
| `Only the squadron owner can do that.` | The page is for owners. | Ask an owner. |
| `can't remove or demote the squadron's only owner` | Every squadron must keep at least one owner. | Make someone else an owner first: see [roles and invites](roles-and-invites). |
| `The website must be a full address starting with http:// or https://.` | The squadron website field only accepts web addresses. | Include `https://`, or leave it blank: see [squadron settings](squadron-settings). |
| `We couldn't send your report just now.` | The planner could not hand your report to the email system. | Try again in a minute; what you typed is kept. If it keeps happening, tell the person who runs the site. |
| `Sign-ups are closed` | This site has turned off claiming new squadrons. | Ask an owner of your squadron to invite you. |
| `Enter the 6-digit code from your authenticator app...` | The account has two-step sign-in on and the code was missing, wrong or already used. | Wait for a new code, then enter it in **Authenticator code**. Five wrong codes lock it for 15 minutes. |
| `Nothing was deleted: ...` | Deleting a squadron needs your password and its number typed exactly. | Check both and try again. |
| `The test email was not sent: ...` | The mail server refused the settings you saved. | See the table in [set up email](operators-email). |
| `Not saved: MAIL_PROVIDER=... needs ...` | The provider you chose needs a value you left empty. | Fill in the named value and save again. |
| `(reference ...)` after an error | A short code for that request. | Quote it when you report the problem, or tell whoever runs the site: it finds the matching line in the server log. |
| `Please describe what went wrong.` | The report needs a description. | Type what happened, then send again. |
| Closure dates rejected | Dates must be real, in order, and at most 366 days apart. | Check the dates: see [programmes](programmes). |
| Question marks in the PDF | Characters outside Western European text, such as Greek, Cyrillic or Chinese. | Use Latin letters: see [PDF export](pdf-export). |

## The page says "Not found"

The address is wrong, the programme belongs to another squadron, or it no
longer exists. Return to the [dashboard](app:/).
