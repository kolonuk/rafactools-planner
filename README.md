# RAFACTools Planner

A friendly, shared planner for an air cadet squadron's monthly training
programme. Instead of one person keeping a spreadsheet, everyone who needs to
can see and edit the same plan, then print it as a PDF that looks like the
paper programme you already use.

It is free and open source. It would suit any youth group that runs a similar
parade-night format.

A public copy is running at <https://rafac.tools> if you want to try it without
installing anything.

## What you can do with it

- **Plan the month in one place.** Fill in each parade night: the activity and
  the instructor for each stage of training, plus notes, weekend events and
  closures. A tracker at the bottom keeps tabs on what still needs covering.
- **Make it your squadron's own.** Choose your training stages, your parade
  nights (one to seven a week) and your uniform colour key.
- **Share the work.** The first person from a squadron to sign up with their
  squadron email claims it and becomes its owner. They can then invite anyone
  else, with an email invite, as an owner, editor or viewer.
- **Move things around.** Drag a night's activities to another date, or use the
  Move button if you prefer.
- **Print it.** Export the month as a PDF laid out like a paper programme.
- **Look back.** Every saved version of a programme is kept, so you can go back
  to an earlier one.
- **Get help as you go.** Built-in help pages, with no sign-in needed, explain
  each screen and are versioned alongside the app.

## Privacy

The planner is built to hold as little personal information as possible. Plan
with first names, ranks or initials where you can, and keep contact details and
medical information out of it. The in-app help has a short "Security and
privacy" page that explains what is stored and who can see it.

## Try it, or run your own copy

You can run the planner on any computer that has
[Docker](https://www.docker.com/). Your squadron's data then stays with you, in
a single database file that you back up.

To try it on your own machine:

```sh
cp .env.template .env      # then set SESSION_KEY to a long random value
docker compose up --build
```

Open <http://localhost:8080>. A mailbox viewer at <http://localhost:8025> shows
the sign-up and invite emails the app would send.

For a real deployment, the "Run your own copy" help page
(`internal/docs/content/next/run-your-own.md`) lists what you need and how to
set it up.

## Getting involved

Ideas, bug reports and pull requests are welcome.

- Found a problem or have an idea? Open an issue on GitHub.
- Want to see what is planned? Read [TODO.md](TODO.md).
- Want to see what has changed? Read [CHANGELOG.md](CHANGELOG.md).
- Want to work on the code? Read [CONTRIBUTING.md](CONTRIBUTING.md).

Please never put real cadet or staff details in an issue, a screenshot or a
pull request. The example programmes in this repository use made-up data on
purpose.

## Licence

Copyright (C) 2026. Released under the GNU General Public License, version 2
only. See [LICENSE](LICENSE).
