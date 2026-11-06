---
title: Public link and calendar
order: 35
summary: Publishing a month, the public link, the calendar feed you can subscribe to, and what is and is not shared.
since: next
audience: admin
changed:
  - next: New page. Publishing a month makes the public link if there is none, and the month page and the list of months show the addresses, a QR code and Publish or Unpublish buttons.
  - next: (earlier) New page. Publish a month, share a one-month public link with a calendar feed, a PDF and a PNG of each published month, and a QR code on the PDF.
---
# Public link and calendar

A squadron can share its programme with people who do not have a login, such as cadets and
parents. Nothing is shared until an owner makes a public link **and** an editor publishes a month.

## Make the public link (owners)

1. Publishing a month (below) makes the link for you if there is none or it has expired. Owners can also make or renew it any time: open [Squadron](squadron-settings), find **Public link and calendar** and select **Make a public link** or **Make a new link**.
2. The page shows the calendar address.
3. The link works for **one month**. Select **Make a new link** whenever you need one. A new
   link replaces the old one straight away, so anyone with the old address must be given the new one.

Treat the link like a password for what it shows: anyone who has it can see the published months.
It is not listed anywhere and search engines are asked to ignore it.

## Publish a month (editors and owners)

Open the month and choose **Publish** in its menu bar (or **Publish** beside the month in the list of months). Choose **Unpublish** to take it down again. Once a month is published its page shows a **Public link** panel with the calendar, PDF and PNG addresses and a QR code. A month that is not published is never shared, even if the
link is known. Changes you save to a published month appear at the link straight away.

## What people see

- **A calendar feed** (`calendar.ics`) to subscribe to in a phone or calendar app. It has one entry
  for each parade night from today onwards in the published months, running from the squadron's
  **parade start time** to its **end time** (set in squadron settings, UK time, 19:00 to 21:30 to
  begin with). The entry lists the uniform and the activities, grouped so that classes doing the same
  thing share a line, and a link to that month's PDF. Weekend events appear as all-day entries.
  Nights covered by a closure are left out.
- **The month's PDF**, the same as the exported one except that the requirements tracker is left off.
- **The month's PNG**, a picture of the programme (dates, uniform, activities, closures, weekend
  events and the uniform key) that is easy to send in a message. It is drawn separately from the
  PDF, so it looks a little different.

The PDF and PNG addresses are `.../share/<link>/2026-11.pdf` and `.../2026-11.png` (year and month),
and the calendar's description links to the PDF. All of them stop working when the link expires.

## The QR code

While the squadron has a working public link, every PDF and PNG ends with a QR code for the calendar,
with the date the link stops. People scan it and their phone offers to subscribe. When you make a
new link, the next PDF you open carries the new code; **a printed copy keeps the old code, which
stops working**, so reprint after renewing.

## What is never shared

Cadet names and the requirements tracker, notes on a night, instructor names in the calendar,
history, members and any unpublished month. The PDF does show the instructor boxes of the grid, as
the paper programme does, so leave names out of those boxes if you do not want them public.

## Files are kept until the month changes

The planner keeps each PDF and PNG it has made and sends the saved copy until something in the month
changes (a save, a move, a restore, a reset, a closure, a settings change or a new link). The next request after
a change makes a new file, so you never get an out-of-date PDF.

## When the link has expired

Anyone who opens an expired link is told that it has expired. Make a new link and share it. Calendar
apps keep showing old entries until they next refresh, which can take hours.
