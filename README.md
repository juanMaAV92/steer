<h1 align="center">⛵ Steer</h1>

<p align="center">
  <b>The simplicity of a PaaS, on your own AWS.</b><br>
  Set it up once; then anyone on your team deploys — without touching the console or memorizing commands.
</p>

<p align="center">
  <a href="#license"><img alt="License: MIT" src="https://img.shields.io/badge/License-MIT-blue.svg"></a>
  <img alt="Status" src="https://img.shields.io/badge/status-WIP-orange.svg">
  <img alt="Go" src="https://img.shields.io/badge/built%20with-Go-00ADD8.svg">
</p>

---

> ⚠️ **Work in progress.** The design is settled; implementation is starting.
> See [`docs/design.md`](docs/design.md).

## What is Steer?

Steer makes deploying to your own cloud **stop being scary**. Platforms like Vercel or
Railway made shipping effortless — but they lock you into their platform. Steer brings that
same "just ship it" feeling to the cloud account *you already own*.

The name evokes *steering the course* — you're at the helm. It ships as a **single static
binary** with two faces over one shared core:

- **TUI** — an interactive hybrid dashboard with a command palette: `steer tui`. This is
  where someone who doesn't live in AWS gets things done without fear.
- **CLI** — scriptable and CI-friendly: `steer -e stg service deploy -s my-svc -t v1.2.3 -y`.
  First-class for the lead and for pipelines.

No more memorizing service names: running a command without arguments opens an interactive
picker (fuzzy filter + multi-select) populated live from your cloud.

## Who is it for?

Steer is built around **two roles**:

- **The one who knows cloud** (you, the lead, the platform person) configures `steer.toml`
  **once** — accounts, roles, environments, naming.
- **Everyone else** deploys, scales and checks status with simple commands and an
  interactive TUI, **without understanding AWS underneath**.

That's how lazygit makes git approachable: it doesn't teach you the internals, it removes
the fear. Steer's sweet spot is **small teams with one technical person and N people who
just want to ship**.

> Honest scope: a solo developer with nobody to do the one-time setup is probably better
> served by a managed PaaS. Steer shines when someone owns the AWS setup and wants to hand
> the team a safe, simple way to deploy on top of it.

### Highlights

- 🛡️ **Guardrails by default** — read-only production, confirmations, and a preview of *what
  will happen* before it happens. Rollback is one command. Hard to break things by accident.
- 🚀 **Interactive deploys** — pick services and image tags from live, fuzzy-filtered lists.
- 📊 **Hybrid TUI** — health of everything at a glance + a `⌘k` command palette.
- 💬 **Errors that teach** — failures explain *what* went wrong, *why*, and *what to try next*.
- ⚙️ **Config-driven** — point Steer at *your* accounts, roles, environments and naming via `steer.toml`. No code changes.
- 📦 **Single binary** — install via Homebrew, `go install`, or a direct download. No runtime to manage.
- ☁️ **AWS today, multi-cloud ready** — built on cloud-agnostic capability interfaces, so Azure/GCP can be added later without reworking the core.

## Status & roadmap

V1 targets **AWS** behind agnostic interfaces. Commands are named by **capability**, not by
AWS service, so the same command keeps working when other clouds are added. Built
incrementally:

1. Foundation — config + AWS session + capability interfaces + CLI skeleton
2. **`service`** — interactive deploy + status (the star vertical slice)
3. `registry`, `db`, `queue`, `storage`, `host`, `env`, `assets`
4. Hybrid TUI dashboard

See [`docs/superpowers/plans/2026-06-15-roadmap.md`](docs/superpowers/plans/2026-06-15-roadmap.md).

## Configuration

Steer reads `steer.toml` from the current repo or `~/.config/steer/steer.toml`.
See [`steer.example.toml`](steer.example.toml). Your config (accounts, role ARNs) stays
private — never commit it to a public repo.

## License

[MIT](LICENSE) © 2026 [juanMaAV92](https://github.com/juanMaAV92).

You may use, modify and distribute this software freely, **but you must preserve the
copyright notice and license** — attribution to the author is required, and you may not
pass it off as your own.
