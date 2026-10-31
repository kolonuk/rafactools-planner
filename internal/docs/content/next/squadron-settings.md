---
title: Squadron settings
order: 40
summary: Name, classifications, parade nights, members and invites (owners only).
since: 1.0
changed:
  - next: Delete this squadron includes a human check when the operator has turned on Turnstile.
  - 1.9: Owners can mark members inactive or email them a temporary password.
  - 1.9: The security log also records sign-ins, wrong passwords, password changes and resets, signing out other sessions and settings changes; owners can download it and are emailed when the site's administrator looks at the squadron.
  - 1.9: Added the security log and the Delete this squadron section.
  - 1.9: Changing the classifications now updates existing months; removing a classification that still has text is refused.
  - 1.9: Added the Squadron website field, and owners can change roles and remove members from the Members table.
  - 1.9: Parade weekdays outside Monday to Sunday are ignored, and an empty name keeps the existing name.
---
# Squadron settings

Only owners can open [Squadron settings](app:/squadron).

## Details

| Field | What it does |
| --- | --- |
| **Name** | Shown on the dashboard and the PDF. Leave it blank to keep the current name. |
| **Subtitle** | The line under the name on the PDF (default `Royal Air Force Air Cadets`). |
| **Protective marking** | A banner printed at the top and bottom of the PDF, for example `OFFICIAL - Public`. Leave blank for none. |
| **Classifications** | One per line, in progression order. These are the columns of the programme grid. Blank lines are ignored. If you clear the box the existing list is kept, because a programme needs at least one column. |
| **Squadron website** | An optional address, starting with `http://` or `https://`, shown as a link on the dashboard and printed under the squadron name on the PDF (where it can be clicked). Leave blank for none. Anything else, such as `javascript:` or an address without `http`, is refused. |
| **Parade weekdays** | The nights your squadron parades. |

Select **Save** to apply.

## Changing parade weekdays and classifications

- **Parade weekdays:** months that already exist keep the nights they were
  created with. You can still tick or untick **Parade** night by night: see
  [programmes](programmes).
- **Classifications:** existing months are updated to match. Adding a
  classification adds a blank box to every row of every programme. Removing one
  removes its boxes, but **only if they are empty**: if any programme still has
  text in a column that would be removed, nothing is changed and you are told
  which classifications to clear first, so typed text is never silently deleted.
  Text stays in its column position when you **rename or reorder**
  classifications, so add new ones at the end of the list.

## Members and invites

The page lists everyone in the squadron with their name (if they have set one, see
[instructors](instructors)), their email and their role, and any pending
invites. Owners can change a role, mark a person inactive, send a temporary password or remove them here, and invite new people.
See [roles and invites](roles-and-invites).

## Security log

At the bottom of the page the **Security log** lists the newest 100 events of the last year. Each line
shows who, what and when (UTC). Ordinary sign-ins are listed apart, under **Recent sign-ins** (the newest 30),
so that frequent sign-ins, or someone guessing a password, can never push the rarer events off the screen.
The log records:

- when the site's administrator looked at your squadron (read-only);
- when someone **signed in**, and a **wrong password** entered for a real account (repeats within five minutes
  count once; unknown addresses are not recorded, and passwords never are);
- password changes, password resets from the emailed link, and signing out other sessions;
- when an owner invited, changed the role of, or removed someone, or saved the squadron settings.

**Download all as a spreadsheet (CSV)** gives the whole year, not just the newest 100. Cells that a
spreadsheet could run as a formula are made harmless first.

When the site's administrator opens your squadron, **each owner is emailed** (at most once an hour
however often they look), so you hear about it when it happens, not just afterwards.

## Delete this squadron

The last section permanently deletes the squadron and **everything in it**: every member's account,
all programmes, the requirements tracker (including cadets' names), the instructor list and the
security log. To do it you type the squadron number and your password. **It cannot be undone and
there is no backup to restore from**, so export each month's PDF first. If the operator has turned
on Cloudflare Turnstile, the form also shows a human check, and nothing is deleted unless it passes.

## Inactive members and temporary passwords

In the **Members** table each person has a **Status**.

- **Mark inactive** stops someone signing in or resetting their password, signs them out, and keeps
  their account and entries. **Mark active** brings them back. An inactive owner does not count as an
  owner, so the last active owner cannot be made inactive, demoted or removed.
- **Send temporary password** emails the member a new password (you never see it). Their old password
  stops working, they must choose their own the next time they sign in, and the temporary one expires
  after **7 days**. It cannot be sent to an inactive member, or to yourself.

Both actions are written to the security log. Members do not delete their own accounts: ask an owner.
