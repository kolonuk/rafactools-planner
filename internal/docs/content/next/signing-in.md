---
title: Signing in
order: 20
summary: Logging in and out, password rules and how long you stay signed in.
since: 1.0
changed:
  - next: Added "Forgot your password?": a reset link by email.
  - next: Passwords longer than 72 characters are now refused with a clear message instead of an internal error.
---
# Signing in

Use the [login page](app:/login) with your email address and password. Email
addresses are not case-sensitive.

## Staying signed in

You stay signed in for **30 days** on that browser, or until you choose
**Log out**. Logging out ends the session on the server as well, so a copy of
the session cannot be used afterwards.

## Passwords

- 12 to 72 characters. (Longer than 72 is refused because only the first 72
  would be used.)
- There is no maximum number of attempts per account, but the site slows
  repeated attempts: see [security and privacy](security-and-privacy).
- If you enter a wrong email or a wrong password you get the same message
  (`Invalid email or password.`), so the page does not reveal which email
  addresses have accounts.
- If you have not yet confirmed your email you are told to do that first.

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
