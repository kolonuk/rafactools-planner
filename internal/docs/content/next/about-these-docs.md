---
title: About these docs
order: 140
summary: How versions work, how to compare them, and how the docs are kept accurate.
since: 1.9
audience: user
changed:
  - 1.13: Added the licence and copyright notice.
  - 1.13: Anyone can read the help without signing in, and the pages are grouped by audience in a tree.
---
# About these docs

These pages are part of the planner, use the same look as the rest of the site,
and are built into the program, so they always come with the version that is
running. **You do not need to sign in to read them**, so you can look things up
before you have an account. They contain no squadron data, only instructions.

## Finding your way

The tree on the left groups the pages by who they are for:

- **Using the planner**: everyday use, for everyone with a login.
- **Squadron admins**: managing people and settings, for owners.
- **Running the site (dev and power users)**: hosting, settings, email, backups and the automated checks.

Select a group to open or close it. The group holding the page you are on is open,
and the page you are on lists its own sections underneath, so you can jump straight to one.

## Versions

Every release of the planner has its own copy of the docs. Use **Version** at
the top of the page list to switch. The same page stays open if it exists in the
version you pick; otherwise you are taken to that version's home page.

- **Unreleased** describes changes that are not in a release yet.
- A banner tells you when you are reading something other than the newest version.

## Comparing versions

Choose **Compare this page between versions** to see two versions of one page
side by side as a list of added and removed lines, or **What changed overall**
for a list of every page that was added, removed or changed between two
versions. This works without any scripts.

## Version notes

At the foot of every page:

- *first appeared in* shows when the topic was introduced;
- a list of **behaviour changes**, each with the version it arrived in, so you
  can tell whether a change applies to the version you are using.

Wording fixes are not listed; changes to behaviour, limits, defaults and
permissions are.

## Licence and copyright

RAFACTools Planner is copyright (C) 2026 and released under the **GNU General Public
License, version 2** (GPL-2.0-only). You may use, study, change and share it under that licence; if
you share a changed copy, you must share its source under the same licence and keep the copyright
notices. There is no warranty. The full text is the `LICENSE` file in the
[source repository](https://github.com/kolonuk/rafactools-planner/blob/main/LICENSE). These help pages
are part of the program and are covered by the same licence.

## How they are kept accurate

Changing anything that people can notice means changing these pages in the same
change, and an automated check blocks changes that do not. Pages describe only
what exists today. Plans live in the repository's `TODO.md`, never here.
If you find a page that is wrong, tell the person who runs the site.
