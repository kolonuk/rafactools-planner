---
title: Run your own copy
order: 98
summary: Why and how to run the planner yourself, what you need, which GitHub settings it uses, and the free public site as an alternative.
since: next
---
# Run your own copy

Your programmes hold your squadron's plans and, in the requirements tracker, cadets'
names. You can decide who can see that data in one of two ways.

## Which should I choose?

**Run it yourself (recommended if you can).** For security and trust this is the
better choice. When the planner runs on a computer you control:

- your data stays on that computer, in one database file that you back up;
- nobody else can read it: there is no site-wide administrator, unless **you** set
  one up (see [below](#the-master-admin-on-your-own-copy));
- you decide the address, the email provider, the backups and how long data is
  kept, and you can remove everything when you wish.

**Use the public site (free of charge).** The maintainer also runs a public copy that
squadrons can use at no cost, so you do not have to run anything. Ask the maintainer
for its address and its current terms; availability is not guaranteed. The trade-off
is trust: your data is stored on the maintainer's server, the maintainer can read the
database, and the site has a **master admin** who can look at any squadron
(read-only) to investigate problems. That is explained in
[security and privacy](security-and-privacy). If that is not acceptable for your
squadron's data, run your own copy, or keep personal details out of the planner.

## What you need

| You need | Why |
| --- | --- |
| **Access to the code or the container image.** The source repository is private at the moment: ask the maintainer to give you access. | To build or pull the planner. |
| **A computer that is on when people use it**, with **Docker** (or Go 1.26 if you build from source). | The planner is one small program; the container needs nothing else. |
| **Somewhere to keep the data**: a folder or volume that survives restarts. | Everything is one SQLite file ([backups](operators-backups)). |
| **A web address with HTTPS** if people will use it from outside your own network, usually through a reverse proxy. | Logins must not travel unencrypted ([running the site](operators-running)). |
| **An email provider (SMTP)**, for confirmation, invitation and password reset emails. | Without it the links are only written to a log ([configuration](operators-configuration)). |
| **An email domain for claiming squadrons**, set as `CLAIM_DOMAIN`. | Only an address like `snco.<number>@<that domain>` or `oc.<number>@<that domain>` can claim a squadron. The default is the RAFAC domain; change it if yours is different. |

## Try it on your own computer

1. Get the code and, in its folder, run `docker compose up --build`.
2. Open `http://localhost:8080`. A second page at `http://localhost:8025` (Mailpit)
   shows every email the planner sends, so you can follow confirmation links without
   a real mail server.
3. The compose file claims squadrons for the RAFAC domain by default. To try it with
   any address, edit `CLAIM_DOMAIN` in `docker-compose.yml` to a domain you can type
   (for example `example.org`), then claim a squadron with `oc.1234@example.org`.
4. Stop it with Ctrl-C. Your data stays in a Docker volume until you delete it.

This setup uses plain HTTP and a throw-away key: it is for trying things out only.

## Running it for real

Follow [configuration](operators-configuration), [running the site](operators-running)
and [backups](operators-backups). In short:

1. Generate a long random `SESSION_KEY` (`openssl rand -hex 32`) and keep it secret.
   Create it **once** and reuse it on every start and redeploy (see
   [the signing key](#the-signing-key)).
2. Set `BASE_URL` to the public address, `CLAIM_DOMAIN` to your email domain, and leave
   `COOKIE_SECURE` at its default (`true`).
3. Give the container a volume for `/data` and take regular backups.
4. Put an HTTPS reverse proxy in front, and configure SMTP.
5. Check the log after the first start: it reports a missing or weak setting. The site
   will not start without `SMTP_HOST`; for trying it out on one computer only, set
   `ALLOW_LOG_MAIL=true` instead (see [configuration](operators-configuration)).

## Building and deploying with GitHub

This repository's workflows (in `.github/workflows/`) do the following on a push to
the `dev`, `test` or `main` branch:

1. check the code (tests, security scans and the other [automated checks](operators-checks));
2. build the container image, start it, and check it answers at `/health`;
3. scan the image and publish it to GitHub's container registry as
   `ghcr.io/<your-account>/rafactools-planner`, tagged by branch and commit (the `main`
   branch is also tagged `latest`).

The workflows then stop. **Starting the image on a host is a separate step** that is
not part of this repository: you pull the image yourself (or have your own
deployment do it) behind a reverse proxy. You can run it on any Docker host: it only
needs the settings below as environment variables.

### GitHub variables and secrets

The planner reads its settings from environment variables when it starts. This
repository's workflows do **not** read them; they are listed here so that **your own
deployment workflow or stack** can keep them in GitHub and pass them to the container.
A *secret* is encrypted and hidden; a *variable* is stored and shown in plain text, so
never put a password or key in a variable.

| Setting | Store it as | Notes |
| --- | --- | --- |
| `SESSION_KEY` | **secret**, in the repository that deploys the site | At least 32 characters. This repository's workflows do not use it: see [the signing key](#the-signing-key). |
| `SMTP_PASS` | **secret** | Your email provider's password. |
| `MASTER_ADMIN_PASSWORD` | **secret** | Only if you want a master admin; 12 to 72 characters. |
| `BASE_URL`, `CLAIM_DOMAIN`, `COOKIE_SECURE` | variable | The address, your email domain, and `true`. |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `MAIL_FROM` | variable | Your email provider. `SMTP_HOST` is required. |
| `DISABLE_SIGNUPS` | variable | `true` closes the claim form, for a copy holding real data. |
| `MASTER_ADMIN_EMAIL` | variable | Only if you want a master admin. |
| `ISSUE_EMAIL` | variable | Where problem reports go (for example `issues@your-domain`); leave empty to switch reporting off. |
| `ISSUE_MASKING` | variable | `true` (default) lets people black out parts of a screenshot; `false` only for a site you run for others. |

On your **own** copy keep `ISSUE_MASKING` at `true`: your people can then hide names in a
screenshot before it reaches you, and the reports go to an address you choose. See
[report a problem](report-a-problem).

A helper creates these settings for you (it asks for the password privately, and
never puts it on a command line). Run it yourself, signed in with `gh auth login`:

```
go run ./cmd/devtools github-setup -repo <your-account>/<your-repo> -prefix "" -apply
```

Omit `-apply` to only see the commands. Use `-prefix NAME_` if your names need a prefix.

## The signing key

`SESSION_KEY` is not set in this repository's workflows (the image build only uses a
throw-away key to smoke-test). It belongs to **whatever deploys and runs the container**.

- **Redeploying through a deployment repository (for example a Docker stack):** store the
  key there as a secret, once, and pass it to the container as `SESSION_KEY` on every
  deploy. Do not generate a new one per deploy: each change makes forms that are open in
  someone's browser fail once with `Form expired, try again.` (signed-in sessions are
  kept). Create it with `openssl rand -hex 32`.
- **Running it yourself on one computer or server (no deployment repository):** generate
  it once, put it in a `.env` file next to your compose file (`SESSION_KEY=...`) or pass
  it with `-e SESSION_KEY=...`, keep that file out of version control, and keep a copy
  with your other settings. Losing it is not a disaster: make a new one and restart.

## The master admin on your own copy

The master admin is optional and **you probably do not want one on your own copy**:
you already run the server, so there is no one else to look at your data on your behalf.
Leave `MASTER_ADMIN_EMAIL` and `MASTER_ADMIN_PASSWORD` empty and no such account exists.
If you do set them, see [configuration](operators-configuration) for exactly what the
account can do and how it is kept in step at every start.

## Keeping up to date

Each release has its own copy of these docs (see [about these docs](about-these-docs)).
Read the changelog and the version notes at the foot of each page before updating, take
a [backup](operators-backups) first, and restart the container: database changes are
applied automatically at start-up.
