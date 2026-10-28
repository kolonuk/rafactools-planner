---
title: Squadron settings
order: 40
summary: Name, classifications, parade nights, members and invites (owners only).
since: 1.0
changed:
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

## Change these before you create a month

Parade weekdays and classifications are used **when a programme is created**.

- **Parade weekdays:** months that already exist keep the nights they were
  created with. You can still tick or untick **Parade** night by night: see
  [programmes](programmes).
- **Classifications:** the column headings of an existing month follow the new
  list, but the boxes already in that month do not change. If the number of
  classifications changed, the headings and boxes of that month no longer line
  up. Set your classifications **before** you create months, and do not change
  the number afterwards. See [known limits](known-limits).

## Members and invites

The page lists everyone in the squadron with their role, and any pending
invites. Owners can change a role or remove a person here, and invite new people.
See [roles and invites](roles-and-invites).
