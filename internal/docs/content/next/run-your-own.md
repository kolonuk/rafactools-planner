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
2. Set `BASE_URL` to the public address, `CLAIM_DOMAIN` to your email domain, and leave
   `COOKIE_SECURE` at its default (`true`).
3. Give the container a volume for `/data` and take regular backups.
4. Put an HTTPS reverse proxy in front, and configure SMTP.
5. Check the log after the first start: it reports a missing or weak setting.

## Building and deploying with GitHub

This repository's workflows (in `.github/workflows/`) do the following on a push to
the `dev`, `test` or `main` branch:

1. check the code (tests, security scans and the other [automated checks](operators-checks));
2. build the container image, start it, and check it answers at `/health`;
3. scan the image and publish it to GitHub's container registry as
   `ghcr.io/<your-account>/rafactools-planner`, tagged by branch and commit (the `main`
   branch is also tagged `latest`).

The workflows then stop. **Starting the image on a host is a separate step**: the
maintainer's own host is a Docker Compose stack kept in a separate repository (called
`dadock`), which pulls `latest` behind a reverse proxy. If you set the optional secret
`DADOCK_DEPLOY_TOKEN`, the `main` workflow asks that stack to redeploy; if you do not,
that step is skipped and you pull the new image yourself. You can run the image on any
Docker host: it only needs the settings below as environment variables.

### GitHub variables and secrets

The planner reads its settings from environment variables when it starts. This
repository's workflows do **not** read them; they are listed here so that **your own
deployment workflow or stack** can keep them in GitHub and pass them to the container.
A *secret* is encrypted and hidden; a *variable* is stored and shown in plain text, so
never put a password or key in a variable.

| Setting | Store it as | Notes |
| --- | --- | --- |
| `SESSION_KEY` | **secret** | At least 32 characters. |
| `SMTP_PASS` | **secret** | Your email provider's password. |
| `MASTER_ADMIN_PASSWORD` | **secret** | Only if you want a master admin; 12 to 72 characters. |
| `DADOCK_DEPLOY_TOKEN` | **secret** | Only for the maintainer's stack; used by the `main` workflow. |
| `BASE_URL`, `CLAIM_DOMAIN`, `COOKIE_SECURE` | variable | The address, your email domain, and `true`. |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `MAIL_FROM` | variable | Your email provider. |
| `MASTER_ADMIN_EMAIL` | variable | Only if you want a master admin. |

A helper creates the master admin pair for you (it asks for the password privately, and
never puts it on a command line). Run it yourself, signed in with `gh auth login`:

```
go run ./cmd/devtools github-setup -repo <your-account>/<your-repo> -prefix "" -apply
```

Omit `-apply` to only see the commands. The default prefix is `RAFACTOOLS_`, the naming
used by the maintainer's stack.

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
