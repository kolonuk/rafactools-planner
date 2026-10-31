---
title: Signing in
order: 20
summary: Logging in and out, password rules and how long you stay signed in.
since: 1.0
audience: user
changed:
  - 1.13: When the operator turns on Turnstile, the login form includes a human check.
  - 1.9: A member given a temporary password must choose their own at first sign-in; an inactive account cannot sign in. Email address can be changed on Your details for non-rafac.mod.gov.uk addresses.
  - 1.9: Your details now lists where you are signed in and can sign out other places; the master admin signs in for 8 hours and can use two-step sign-in.
  - 1.9: Added Change password on the Your details page.
  - 1.9: Added "Forgot your password?": a reset link by email.
  - 1.9: Passwords longer than 72 characters are now refused with a clear message instead of an internal error.
---
# Signing in

Use the [login page](app:/login) with your email address and password. Email
addresses are not case-sensitive.

## The human check

If the site's operator has turned on Cloudflare Turnstile, the login form shows a short check
above the **Log in** button. If it fails or times out you see "The human check failed or timed out.
Reload the page and try again." Your password is not tried until the check passes. The check is
also on the [claim page](app:/claim) and on **Delete this squadron**. It is off unless the operator
sets it up ([configuration](operators-configuration)).

## Staying signed in

You stay signed in for **30 days** on that browser, or until you choose
**Log out**. Logging out ends the session on the server as well, so a copy of
the session cannot be used afterwards.

### Where you are signed in

**Your details** lists every browser that is signed in as you: what kind of browser, when it
signed in and when it expires (times are UTC). **Sign out** next to one ends only that session;
**Sign out everywhere else** ends all of them except this browser. Use it if you forgot to log out
of a shared computer or lost a device. To end this browser's session, use **Log out**.

### The master admin

The master admin signs in for **8 hours** rather than 30 days, and can turn on **two-step
sign-in** on **Your details**: add the shown key to an authenticator app (as a time-based key),
enter the 6-digit code to confirm, and from then on logging in also asks for a code in the
**Authenticator code** box. Each code works once.

## Passwords

- 12 to 72 characters. (Longer than 72 is refused because only the first 72
  would be used.)
- There is no maximum number of attempts per account, but the site slows
  repeated attempts: see [security and privacy](security-and-privacy).
- If you enter a wrong email or a wrong password you get the same message
  (`Invalid email or password.`), so the page does not reveal which email
  addresses have accounts.
- If you have not yet confirmed your email you are told to do that first.

## Changing your password

Select your name (or email) in the top bar to open **Your details**. Under
**Change password**, enter your **current password** and a new one (12 to 72
characters) and select **Change password**.

- You stay signed in on this browser. Every **other** place you were signed in is
  signed out, which is what you want if someone else may have your old password.
- A wrong current password is refused and nothing changes. Wrong attempts count
  towards the same limit as logging in (see [security and privacy](security-and-privacy)).
- Any password reset link you had requested before is cancelled.

## If you forget your password

1. On the [login page](app:/login) select **Forgot your password?**, or go to the
   [reset page](app:/forgot), and enter your email address.
2. The page always says the same thing, whether or not the address has an
   account: *"If that address has an account on this site, we've sent a link…"*.
   This is deliberate, so nobody can use the page to find out who has an account.
3. If the address has an account that you have confirmed, you are emailed a link.
   It works **once** and expires after **1 hour**.
4. Open the link, choose a new password (12 to 72 characters) and select
   **Change password**. Then log in.

Things to know:

- Changing your password **signs you out everywhere**, on every browser.
- Opening the link does not use it up; only choosing a new password does. If the
  password you type is refused, you can try again with the same link.
- Asking for a new link cancels the previous one.
- To stop someone filling your inbox, a second request within a minute sends
  nothing. Wait a minute and ask again.
- No email? Check your spam folder. An address that never confirmed its email
  gets no reset link: use the original confirmation email instead (see
  [getting started](getting-started)).
- If you did not ask for a reset, ignore the email: your password has not changed.

## A temporary password

If an owner sends you a temporary password, sign in with it within 7 days. You are taken straight to
a page to choose your own password and can use nothing else until you do. After 7 days it stops
working: ask the owner for another, or use **Forgot your password?**. An **inactive** account cannot
sign in at all; ask an owner to make it active.

## Changing your email address

On [Your details](app:/account), **Change email address** sends a confirmation link to the new
address; the change happens only when you follow it, and the old address is told. This is offered only
when your current address is **not** a `rafac.mod.gov.uk` address (those are tied to the service
and fixed), and the new address cannot be a `rafac.mod.gov.uk` one or one already in use.
