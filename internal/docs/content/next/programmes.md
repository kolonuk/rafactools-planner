---
title: Programmes
order: 50
summary: Creating a month and filling in the grid, closures, weekend notes and the requirements tracker.
since: 1.0
changed:
  - next: A closure locks the dates it covers in the grid.
  - 1.9: Moving a night now carries its uniform and notes too, and un-ticks the date it left when the target was not a parade night.
  - 1.9: Every save, reset and restore is kept as a version; the History page lists them (who, when, what) and any can be restored. The draft status is gone: every month is live.
  - 1.9: This month is made automatically when anyone signs in; past months cannot be created.
  - 1.9: Ticking Parade shows the night's rows straight away; nothing is saved until Save. "Show parade evenings only" is a switch, on by default.
  - 1.9: Activity and Instructor labels have their own column after Uniform, which now has a heading.
  - 1.9: The New month box can copy last month's requirement category names (never cadet names).
  - 1.9: A month can be reset to blank, after a confirmation.
  - 1.9: The day label is worked out from the date and can no longer be edited.
  - 1.9: A new second half starts as a copy of the first, and a second half can be removed (with a warning if the halves differ).
  - 1.9: Instructor boxes suggest All Staff, your instructor list and your members as you type.
  - 1.9: Activities can be moved to another date, by dragging or with the Move control.
  - 1.9: "+ Add a second half to this night" now works in the browser (it previously saved the grid instead).
  - 1.9: Closure and weekend-note dates are checked when saved; a closure must end on or after its start and last at most 366 days.
  - 1.9: Ticking Parade on a night that was not generated as a parade night now gives it activity and instructor rows.
---
# Programmes

A programme is one calendar month for your squadron. The [dashboard](app:/)
lists them, newest first, each with an **Edit** link and a **PDF** link.

## Create a month

Editors and owners see a **New month** box on the dashboard. Choose the month
and year and select **Create**. Each squadron can have one programme per month;
creating the same month again is refused.

Leave **Copy the requirement tracker's category names** ticked and the new month starts with the same
categories (and due labels) as the nearest earlier month that has any. **Cadets' names and notes are never
copied**; untick the box to start with an empty tracker.

The new programme has one row per calendar day. Nights that match your
[parade weekdays](squadron-settings) are marked as parade nights and get an
**Activity** row and an **Instructor** row with one box for each
classification.

## The grid

Open **Edit** on a month. For each day you can set:

- **Day** – the weekday (for example `M` or `Tu`), worked out from the date. It
  cannot be edited, so it can never disagree with the calendar.
- **Parade** – tick for a parade night. A night ticked after the month was
  created gets its Activity and Instructor rows when you save.
- The uniform for the night (the dropdown beside Parade).
- **Activity** and **Instructor** text for each classification. If neighbouring
  classifications have exactly the same text, the PDF shows them as one wide cell.
- **Notes** for the day.

Select **Save programme** to keep your changes. Nothing is saved until you do.

As you type in an **Instructor** box the browser suggests **All Staff**, the people in
your [instructor list](instructors) and each member of your squadron. They are only
suggestions: you can type any name.

On a narrow screen the grid scrolls sideways rather than squeezing its columns.

**Show parade evenings only** hides the days that are not parade nights, to make
the grid shorter.

### Adding and removing a second half

A parade night starts with one half. **+ Add a second half to this night** adds a
second pair of Activity and Instructor rows, **copied from the first half** so
you only need to change what differs.

**− Remove second half** takes a night back to one half.

- If the second half has the same text as the first, it is removed straight away.
- If it is different, you are shown both halves side by side and asked to
  confirm with **Yes, remove Half 2**, because removing it deletes that text.
  **Cancel** keeps it.

Both buttons act on the saved programme, so select **Save programme** first if
you have typed changes you want to keep.

## Moving activities to another date

You can move a parade night's activities to a different date, for example when
a session is rescheduled. Only owners and editors can.

**With the mouse:** each parade night has a handle (two columns of dots) beside
its date. Drag it and drop it on any row of the date you want. You are asked to
confirm first, and nothing changes if you cancel.

**On a touch screen, or if scripts are switched off:** under the
date of each parade night, open **Move**, choose the date and select
**Move activities**.

What happens:

- The whole night moves: the Activity and Instructor text of both halves, the
  number of halves, the uniform and the night's notes. The date and weekday shown
  are those of the date it now sits on.
- It is a **swap**, so nothing is overwritten. If the other date already has
  activities, the two nights trade places and both stay parade nights.
- If the date you move to was not a parade night, it becomes one, and the date you
  moved from stops being one, because the parade is no longer on that date.
- Anything else you have typed on the page but not yet saved is saved first when
  you use drag and drop, so it is not lost. With the **Move** control, select
  **Save programme** first, because that button reloads the page.

## Closures

Use a closure when the squadron is closed (for example an Easter stand-down).
Enter the first and last date and a message, then select **Add closure**.

- Dates must be real dates. The last date must be on or after the first date,
  and a closure can span at most 366 days.
- The message is printed on the PDF over the closed dates.
- **The dates are locked in the grid.** A closed date is shown as one "Closed" row with the
  message and has nothing to edit: you cannot tick it, change its uniform or notes, or move
  activities to or from it. Delete the closure to edit the date again. What was typed there
  before is kept, and comes back if the closure is deleted.
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

## Starting a month again

**Reset this month**, at the top of a programme, empties it so you can start again.
You are asked to confirm first, and **Cancel** changes nothing.

- Emptied: every activity and instructor box, each night's notes, the closures, the weekend
  entries and the requirements tracker (including the cadets' names).
- Kept: the month itself, which nights are parade nights, uniform colours and any second halves.
- It cannot be undone, so use **Export PDF** first if you want a copy. Only owners and editors can.
- Months are never deleted; they stay in your list.

## Who can do what

Viewers can open a programme and its PDF but do not see the editing forms. See
[roles and invites](roles-and-invites).

## This month, made for you

You never need to create the current month. When anyone in the squadron signs in (or opens the
dashboard after a month change), the current month is made if it is missing, using the squadron's
parade weekdays and classifications. Months in the past cannot be created. The master admin's sign-in
makes nothing, because the master admin has no squadron of their own.

## Ticking a night

Tick **Parade** on a night and its Activity and Instructor rows appear straight away so you can type
into them. Nothing is stored until you select **Save**. **Show parade evenings only** is a switch
above the grid and is on by default; turn it off to see every day.

## History and restoring

There is no draft or final status: every programme is live. Instead the planner keeps a **version**
every time the programme is saved, reset, restored, or changed in any other way (closures, weekend
notes, the tracker, second halves and so on). Select **History** at the top of a programme to see the
newest first: the version number, the date and time (UTC), who made the change and what it was.
Identical consecutive versions are kept once, and the newest 200 are kept.

Editors and owners can **Restore** any version after a confirmation page. Restoring is itself saved as
a new version, so it can be undone by restoring the one before. A restored programme is fitted to the
squadron's current classifications. There is no preview.
