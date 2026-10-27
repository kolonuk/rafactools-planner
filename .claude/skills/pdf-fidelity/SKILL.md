---
name: pdf-fidelity
description: Use when changing internal/pdfgen, or when asked to improve/fix the exported PDF's layout, to compare the generated output against the example PDFs rather than guessing from the code alone.
---

# Checking PDF layout fidelity

`internal/pdfgen` is a best-effort recreation of a real paper format, not a
spec implemented from scratch. The real squadron programmes it was modelled
on are **not** in this repo or its git history — they carried real cadets'
and staff's names, so they were deliberately kept out (see `git log` for the
history-rewrite commit if you need the full story). `examples/pdfs/` instead
holds synthetic reconstructions with fabricated names/squadron/dates but the
same layout patterns (merged classification cells, closures, weekend notes,
the footer requirement tracker, and both the 4- and 5-classification
template variants) — generate/regenerate them with `go run ./cmd/genexamples`
(see `cmd/genexamples/main.go` to add a new pattern). Read some of them (the
`Read` tool extracts both text and a rendered image from a PDF) before
changing layout code, and re-render after, to compare against them directly
rather than trusting the code to be right.

**If you're ever handed a real example PDF again** (the user uploading one to
discuss a layout bug, say): look at it, describe what you learn in prose or
reflect it in a *synthetic* `cmd/genexamples` fixture, but don't commit the
real file itself into `examples/pdfs/` or anywhere else in this repo.

## How to render and inspect a generated PDF

```sh
docker build -t rafactools-planner:dev .
docker run -d --name rt-pdf -p 18099:8080 \
  -e SESSION_KEY=pdf-check-session-key-0123456789abcd -e COOKIE_SECURE=false \
  rafactools-planner:dev
# ... claim a squadron, create a programme, fill some rows (see internal/web
# handlers for the form field names: day_<id>_*, row_<id>_text_<idx>) ...
curl -s -b cookies.txt -o /tmp/out.pdf http://localhost:18099/programmes/1/pdf
docker rm -f rt-pdf
```

Then use the `Read` tool on `/tmp/out.pdf` — it rasterizes the page so you can
actually see the grid, not just extract text. `poppler-utils` (`pdftoppm`)
must be installed for that; `apt-get install -y poppler-utils` if missing.

## Layout model: measure first, draw second

The whole month must fit on **one portrait A4 page** — no page-break
fallback (`SetAutoPageBreak(false, 0)`; a month that doesn't fit is a bug to
fix here, not a second page to add). To make that work, nothing has a fixed
height: `computeDayLayout` measures every cell's wrapped line count (via
`Fpdf.SplitText`, which needs the font already set to whatever that cell
will actually draw with) *before* anything is drawn, and `drawWeek` does the
same across a whole week for the Weekends column, scaling every day in the
week up if the week's Sat/Sun note text needs more room than the days'
own content would otherwise get.

Each row is then drawn in **two passes**: first every cell's border+fill at
its full computed height (so the grid stays a clean rectangle no matter
which cell needed the extra lines), then the text overlaid without its own
border, top-aligned. Don't draw a bordered cell with its text in one
`CellFormat`/`MultiCell` call directly off a computed height — `CellFormat`
vertically *centers* single-line text within whatever height it's given,
which looks fine for a fixed-height cell but puts a label like "SAT" in the
dead center of a tall merged cell, colliding with wrapped text that starts
from the top. This bit real: the Weekends column's "SAT"/"SUN" header used
to be drawn this way and would visually collide with long weekend-note text
on a short week — now it's drawn as its own small fixed-height slice at the
top of the cell, separate from the note text below it.

If you add a new piece of grid content, measure it the same way (a
`wrapLineCount` call with the font already set) before drawing it, rather
than assuming a fixed height will be enough.

## Known gaps (also tracked in TODO.md)

- Multi-week closures (e.g. the Christmas stand-down spanning 3+ weekly
  blocks as one merged grey cell in a real squadron's December PDF) are
  drawn per-calendar-date here, not as one true rowspan across weeks.
  `drawClosureRow` in `pdfgen.go` is where that'd need to change.
- A weekend event that runs across a week boundary (e.g. "18-23 Jul" camp
  overlapping two Saturdays) isn't visually spanned — it just repeats or
  attaches to one date. See `drawWeekendCell`.
- `SplitText`'s word-wrap breaks mid-word when a single word is wider than
  the column (no hyphenation) — visible on long weekend-note phrases in a
  narrow Weekends column. Acceptable but ugly; widening those columns
  further (at classification columns' expense) is the lever if it matters.
- An extremely dense month (many 2-half parade nights, long requirement
  lists) could in principle still be too tall for one page even with
  wrapping — there's no fallback if so, by design (see above). Hasn't been
  hit with real-shaped data yet; if it is, the fix is tighter fonts/columns,
  not a second page.
- Classification count/names are read from `squadron.Classifications`
  (configurable per squadron), matching the newer 5-column "Master" template
  as well as the older 4-column one — don't hardcode 4 anywhere new.
