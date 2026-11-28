---
title: Known limits
order: 80
summary: Things the planner cannot do yet, so you do not waste time looking for them.
since: 1.0
audience: user
changed:
  - next: Uniform colours can be edited, activities have a list to choose from, and the PDF prints Greek and Cyrillic; the limits for those are corrected.
  - 1.13: Removed two entries that were wrong: the help is public, and a squadron can be deleted from squadron settings.
  - 1.9: Two-step sign-in exists for the master admin only.
  - 1.9: A month can be reset to blank; months are deliberately never deleted.
  - 1.9: Curly quotes, dashes and accents now print on the PDF; only non-Western-European characters do not.
  - 1.9: Added the Report a problem feature, and its limits.
  - 1.9: Role changes, member removal, removing a second half and automatic day labels now exist, so they are no longer listed.
  - 1.9: Password reset and change password now exist, and changing the classifications updates existing months;
  - 1.9: Passwords over 72 characters are refused; ticking Parade on a non-parade night now works; activities can be moved between dates.
---
# Known limits

These are not mistakes in how you are using the planner.

**People**
- Two-step sign-in exists only for the master admin; ordinary accounts have a password only.

**Programmes**
- Months are never deleted (a month can be reset to blank). History keeps the newest 200 versions of each month.
- Unticking **Parade** on a night that has activities leaves them stored but
  hidden; there is no "cancel this night" action.
- Moving activities swaps two dates; there is no way to move one night's
  activities to several dates at once.
- Parade weekdays are applied when a month is created; the nights of an
  existing month can be ticked or unticked one by one.
- Renaming or reordering classifications does not move the text already typed:
  it stays in the same column position.
- Activities are free text with a [list of suggestions](activities); the boxes never force a choice.
- The [instructor list](instructors) holds only a name and a rank: there is no
  contact detail or duty rota. [Time off](time-off) is recorded separately, and nothing warns when one person is on two things at once.

**Output**
- The PDF is one A4 page. Besides it there is a PNG picture and a [calendar feed](public-sharing).
- No squadron crest or logo image on the PDF (a website address can be shown).
- Greek and Cyrillic print on the PDF, but Chinese, Japanese, Korean, Arabic, Hebrew and emoji print as a question mark.

**Site**
- Problem reports are emailed, not kept in the planner: there is no list of past reports and no status to follow.
- Taking a screenshot automatically needs a desktop browser that supports it; elsewhere attach a screenshot file.
- Deleting a squadron removes everything in it at once and cannot be undone; there is no
  way to delete only part of a squadron's data (for example one person's cadet entries) other than
  removing the entries by hand.
