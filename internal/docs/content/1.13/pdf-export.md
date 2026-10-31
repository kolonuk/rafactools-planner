---
title: PDF export
order: 60
summary: What the exported programme looks like.
since: 1.0
audience: user
changed:
  - 1.9: Only ticked parade nights are printed, and a closed day is one merged block across the classification columns.
  - 1.9: Curly quotes, dashes and accented letters now print correctly; only characters outside Western European text print as a question mark.
  - 1.9: The squadron website, if set, is printed under the subtitle as a clickable link.
  - 1.9: A closure with unreadable dates can no longer make the export hang.
---
# PDF export

Select **PDF** next to a month on the [dashboard](app:/), or **Export PDF** at
the top of a programme. The file opens in your browser and is named like
`April_2030_Programme.pdf`. Any role can export.

## What is in it

- One portrait A4 page for the whole month.
- A title band with the squadron name, subtitle, the squadron website (if you set
  one in [squadron settings](squadron-settings)) and the month.
- The protective marking at the top and bottom, if you set one in
  [squadron settings](squadron-settings).
- One row group per ticked parade night (nights not ticked are left out), with the Activity and Instructor text under each
  classification. Neighbouring columns are merged only when **both** the activity and the
  instructor are the same as the next column's; if only one of them matches, they stay separate.
- Closures and weekend notes against their dates; a closed day is a single merged cell across the classification columns and rows.
- A key for the uniform colours.
- The requirements tracker.
- A footer showing the version of the planner that produced it.

The export always reflects what is saved at that moment. Save your changes in
the editor before exporting.

## Limits

- The page is fixed at one page. Very long text makes the rows taller; there is
  no second page.
- The built-in font covers Western European characters, including curly quotes,
  dashes and accented letters copied from a word processor. Characters outside
  that (for example Greek, Cyrillic or Chinese) and emoji print as a question mark. They are saved and shown correctly on screen.
