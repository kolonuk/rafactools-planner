---
title: Troubleshooting
order: 90
summary: What the messages mean and what to do.
since: 1.0
changed:
  - next: Added the password reset messages.
---
# Troubleshooting

| What you see | What it means | What to do |
| --- | --- | --- |
| `Invalid email or password.` | The email or password is wrong. | Check both, or use **Forgot your password?**: see [signing in](signing-in). |
| `That password reset link is invalid or has expired.` | The link was used, is older than 1 hour, or a newer one replaced it. | Ask for a new link on the [reset page](app:/forgot). |
| `Confirm your email first` | You claimed a squadron but never opened the confirmation link. | Open the link in the email (valid 24 hours). If it expired, ask whoever runs the site. |
| `Form expired, try again.` | The page you were on is too old, or you were signed out. | Reload the page and repeat the change. |
| `Too many attempts` | Too many tries from your network in five minutes. | Wait five minutes. Many people behind one network share the limit. |
| `email must be your squadron's snco.<number>@ or oc.<number>@ address` | Only those addresses can claim a squadron. | Use your squadron's own address, or ask the owner for an [invite](roles-and-invites). |
| `that squadron number has already been claimed` | Someone from your squadron already did. | Ask them to invite you. |
| `password must be at least 12 characters` / `at most 72 characters` | The password length is outside the allowed range. | Choose a password of 12 to 72 characters. |
| `a programme for YYYY-MM already exists` | That month is already on your dashboard. | Open it from the [dashboard](app:/). |
| `That invite link is invalid or has expired.` | The invite was used, revoked or is older than 7 days. | Ask an owner for a new one. |
| `Viewers can't make changes.` | Your role is viewer. | Ask an owner to invite you again as an editor: see [roles and invites](roles-and-invites). |
| `Only the squadron owner can do that.` | The page is for owners. | Ask an owner. |
| Closure dates rejected | Dates must be real, in order, and at most 366 days apart. | Check the dates: see [programmes](programmes). |
| Odd characters in the PDF | Characters outside Western European text. | Use plain quotes and hyphens: see [PDF export](pdf-export). |

## The page says "Not found"

The address is wrong, the programme belongs to another squadron, or it no
longer exists. Return to the [dashboard](app:/).
