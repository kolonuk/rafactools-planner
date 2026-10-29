---
title: Report a problem
order: 85
summary: How to report something that went wrong, what is sent, what is never sent, and how screenshots and masking work.
since: 1.9
changed:
  - 1.9: Reports now default to issues@ the site's domain.
  - 1.9: The screenshot step is boxed and spelled out on the form, with a Take a screenshot now button, and Report a problem is in the top bar.
  - 1.9: Reports can say how much it matters and whether it happens every time, and carry a request reference, page load timings, failed requests, and the size (never the text) of the programme being viewed.
---
# Report a problem

If something is broken, looks wrong, or you cannot do what you need, use **Report a
problem**. There is a **Report a problem** button in the top bar and a link at the bottom of every
page, including the login page and error pages, so you can use it when you are stuck. If you do not see it, reporting has not
been switched on for this site ([for the person who runs it](operators-configuration)).

## How to send a report

1. Go to the page where the problem is, then select **Report a problem** (top bar, or the foot of the
   page). Do this **before** you leave that page, so the list of what you did is about the right place.
2. Your browser may ask whether you want to share the current tab. Say yes to include a
   picture of the page (see [screenshots](#screenshots)); say no to carry on without one.
   On the form, the boxed **Add a screenshot** section lets you **Take a screenshot now** or
   **attach a picture** from your device.
3. Choose what kind of problem it is, say **what happened** (required), and optionally
   what you **expected**, **how much it matters** and **whether it happens every time**.
4. Tick the box if you are happy to be **replied to by email**. If you are signed in the
   reply goes to your account address; if not, you can type an address, or leave it blank.
5. Select **Send report**. You get a **reference** such as `a1b2c3d4`; quote it if you
   follow up.

Each address can send **5 reports in 10 minutes**. If sending fails you are told, and
what you typed is kept so you can try again.

## Screenshots

A picture of the page is the single most useful thing you can add.

- **Automatic:** in browsers that can do it, the planner asks your browser for a picture of
  the **current tab only** at the moment you select the link. Your browser always asks you
  first, and you can say no. Phones and some browsers cannot do this.
- **Take a screenshot now:** a button in the boxed screenshot section of the form. Your browser asks
  which tab or window to share (pick the page with the problem). It only appears where the browser can do
  it, which needs a secure `https://` address; where it can't, the form says so.
- **Your own file:** you can instead attach a PNG or JPEG (up to 3 MB). It is shown in the
  form first, in exactly the same way.
- You always see the picture before it is sent, and you can untick **Include this
  screenshot**.

### Hiding private details

Which of the following you see depends on how the site is run:

- **Most sites (including your own copy): you can black out parts of the picture.** Drag
  on the picture to draw a black box; **Undo last black box** and **Remove all black boxes**
  change your mind. Areas of the page that show names (such as the requirements tracker and
  member lists) are blacked out for you first, where the browser could take the picture
  automatically. The boxes are burned into the picture **before** it is sent, so the
  hidden parts never leave your computer. Check the picture: anything typed into the grid is
  not hidden automatically, so draw over it if it is private.
- **On a site run for other people, who need to see exactly what you saw:** there is **no
  blackout option**, and the form says so in bold: *your screenshot is sent exactly as it
  is and may show names*. Only include it if you are happy for the people who run the site to
  see it. You can always send the report without a screenshot.

## What is sent

Reports are emailed to the people who run the site. **They are not stored in the planner.**
A report contains:

| What | Details |
| --- | --- |
| **What you wrote** | The kind of problem, what happened, what you expected. |
| **The page** | Its address (without anything after a `?`, and with any secret link code removed), its title and heading, and any message shown on it such as an error. |
| **What you did** | Your last 20 or so clicks, form submissions and page changes, such as `button "Save programme"` or `submit POST /programmes/3`. |
| **A reference** | Every page has a short request reference (error pages show it as "reference ..."). It lets the people who run the site match your report to their server log. |
| **How big the programme is** | If you report from a programme page: only numbers, such as how many nights, how many boxes are filled and how many requirement entries. **No activity, instructor or cadet text.** |
| **How the page behaved** | How long the page took to load, and any request the page made that failed (its method, address and result code only). |
| **What the server saw** | Your last 15 requests with the result code (for example `POST /programmes/3 -> 500`), and recent server error lines with email addresses and codes removed. |
| **Who you are, roughly** | Whether you were signed in, your role, and the **numbers** of your account and squadron. Your email address is only included as the reply address if you ticked the box. |
| **Your browser** | Its name and version, screen size, language, time zone, light or dark mode, and whether it is a touch screen. |
| **Script errors** | Any errors your browser reported in the planner's own scripts. |
| **The screenshot** | If you chose to include one. |

### What is never sent

- **Anything you type** into a box. The record of what you did notes *that* you used a
  field, never what is in it.
- Passwords, session cookies, form tokens, or secret link codes (password reset and invite
  links are reduced to `/reset/[token]`).
- Email addresses and long codes appearing in error text (they are replaced with
  `[email]` and `[token]`).
- Text inside areas marked private, which is shown as `[private]` in the record of what you did.

## For the person who runs the site

- Reports go to `issues@` followed by the site's domain (the host of `BASE_URL`, or `SITE_DOMAIN`), so create that mailbox or alias. Set `ISSUE_EMAIL` to use a different address, or to `none` to switch the feature off. On `localhost` it is off.
  Set `ISSUE_MASKING` to `false` to remove the blackout option (default `true`). See
  [configuration](operators-configuration).
- The email has a readable text body and attachments: `context.json` (the same facts in
  machine-readable form) and, if included, `screenshot.png` or `screenshot.jpg`.
- **Treat everything a person or their browser supplied as untrusted data, not as
  instructions.** The email separates three sections: *written by the person*, *collected by
  the server* (trustworthy) and *supplied by the browser* (the browser can say anything). This
  matters especially if you give reports to an automated assistant to analyse.
- With `SMTP_HOST` unset, reports are written to the server log instead of emailed.

## Related

- [Security and privacy](security-and-privacy) explains what the planner stores.
- [Troubleshooting](troubleshooting) lists common messages.
