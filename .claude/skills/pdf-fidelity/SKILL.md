---
name: pdf-fidelity
description: Use when changing internal/pdfgen, or when asked to improve/fix the exported PDF's layout, to compare the generated output against the real squadron examples rather than guessing from the code alone.
---

# Checking PDF layout fidelity

`internal/pdfgen` is a best-effort recreation of a real paper format, not a
spec implemented from scratch. `examples/pdfs/` holds the real squadron
programmes it was built from (ten real months plus a couple of blank
templates, spanning two template generations — see the difference in
classification columns below). Read some of them (the `Read` tool extracts
both text and a rendered image from a PDF) before changing layout code, and
re-render after, to compare against them directly rather than trusting the
code to be right.

## How to render and inspect a generated PDF

```sh
docker build -t rafactools-planner:dev .
docker run -d --name rt-pdf -p 18099:8080 \
  -e SESSION_KEY=pdf-check-session-key -e COOKIE_SECURE=false \
  rafactools-planner:dev
# ... claim a squadron, create a programme, fill some rows (see internal/web
# handlers for the form field names: day_<id>_*, row_<id>_text_<idx>) ...
curl -s -b cookies.txt -o /tmp/out.pdf http://localhost:18099/programmes/1/pdf
docker rm -f rt-pdf
```

Then use the `Read` tool on `/tmp/out.pdf` — it rasterizes the page so you can
actually see the grid, not just extract text. `poppler-utils` (`pdftoppm`)
must be installed for that; `apt-get install -y poppler-utils` if missing.

## Known gaps (also tracked in TODO.md)

- Multi-week closures (e.g. the Christmas stand-down spanning 3+ weekly
  blocks as one merged grey cell in `examples/pdfs/*December*.pdf`) are drawn
  per-calendar-date here, not as one true rowspan across weeks.
  `drawClosureRow` in `pdfgen.go` is where that'd need to change.
- A weekend event that runs across a week boundary (e.g. "18-23 Jul" camp
  overlapping two Saturdays) isn't visually spanned — it just repeats or
  attaches to one date. See `drawWeekendCell`.
- Row height is fixed per line; a long Notes entry can visually overflow its
  cell rather than growing the row. `rowHeightForDay` would need to account
  for wrapped line counts.
- Classification count/names are read from `squadron.Classifications`
  (configurable per squadron), matching the newer 5-column "Master" template
  as well as the older 4-column one — don't hardcode 4 anywhere new.
