---
title: Signing in
order: 20
summary: Logging in and out, password rules and how long you stay signed in.
since: 1.0
changed:
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

There is currently **no password reset**. See [known limits](known-limits).
Ask the person who runs the site for help.
