---
title: Programmes
order: 50
summary: Creating a month and filling in the grid, closures, weekend notes and the requirements tracker.
since: 1.0
changed:
  - next: Closure and weekend-note dates are checked when saved; a closure must end on or after its start and last at most 366 days.
  - next: Ticking Parade on a night that was not generated as a parade night now gives it activity and instructor rows.
---
# Programmes

A programme is one calendar month for your squadron. The [dashboard](app:/)
lists them, newest first, each with an **Edit** link and a **PDF** link.

## Create a month

Editors and owners see a **New month** box on the dashboard. Choose the month
and year and select **Create**. Each squadron can have one programme per month;
creating the same month again is refused.

The new programme has one row per calendar day. Nights that match your
[parade weekdays](squadron-settings) are marked as parade nights and get an
**Activity** row and an **Instructor** row with one box for each
classification.

## The grid

Open **Edit** on a month. For each day you can set:

- **Day** – the short label shown on the PDF. It is a text box you can edit; if
  you change it, make sure it is still right for the date.
- **Parade** – tick for a parade night. A night ticked after the month was
  created gets its Activity and Instructor rows when you save.
- The uniform for the night (the dropdown beside Parade).
- **Activity** and **Instructor** text for each classification. If neighbouring
  classifications have exactly the same text, the PDF shows them as one wide cell.
- **Notes** for the day.

Select **Save programme** to keep your changes. Nothing is saved until you do.

**Show parade evenings only** hides the days that are not parade nights, to make
the grid shorter.

### Adding a second half

A parade night starts with one half. **+ Add a second half to this night** adds a
second pair of Activity and Instructor rows. It cannot currently be removed
again: see [known limits](known-limits).

## Closures

Use a closure when the squadron is closed (for example an Easter stand-down).
Enter the first and last date and a message, then select **Add closure**.

- Dates must be real dates. The last date must be on or after the first date,
  and a closure can span at most 366 days.
- The message is printed on the PDF over the closed dates.
- **Delete** removes it.

## Weekend notes

A weekend note is a line of text against one date, for events such as a camp.
Enter the date and the text, then select **Add**. **Delete** removes it.

## Notes / requirements tracker

The table at the foot of the PDF tracks things such as mandatory briefings.
Add a **category** (a name and an optional due label), then add **entries**
(a cadet's name and an optional note) under it. Deleting a category deletes its
entries.

Entries hold cadets' names. Treat them as personal data: see
[security and privacy](security-and-privacy).

## Who can do what

Viewers can open a programme and its PDF but do not see the editing forms. See
[roles and invites](roles-and-invites).
