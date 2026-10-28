---
title: Squadron settings
order: 40
summary: Name, classifications, parade nights, members and invites (owners only).
since: 1.0
changed:
  - next: Added the security log and the Delete this squadron section.
  - next: Changing the classifications now updates existing months; removing a classification that still has text is refused.
  - next: Added the Squadron website field, and owners can change roles and remove members from the Members table.
  - next: Parade weekdays outside Monday to Sunday are ignored, and an empty name keeps the existing name.
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
invites. Owners can change a role or remove a person here, and invite new people.
See [roles and invites](roles-and-invites).

## Security log

At the bottom of the page the **Security log** lists the newest 50 events of the last year:
when the site's administrator looked at your squadron (read-only), and when an owner invited,
changed the role of, or removed someone. Each line shows who, what and when (UTC).

## Delete this squadron

The last section permanently deletes the squadron and **everything in it**: every member's account,
all programmes, the requirements tracker (including cadets' names), the instructor list and the
security log. To do it you type the squadron number and your password. **It cannot be undone and
there is no backup to restore from**, so export each month's PDF first.
