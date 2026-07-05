<h1 align="center">⛵ Steer</h1>

<p align="center">
  <b>The simplicity of a PaaS, on your own AWS.</b><br>
  Set it up once; then anyone on your team deploys — without touching the console or memorizing commands.
</p>

<p align="center">
  <a href="#license"><img alt="License: MIT" src="https://img.shields.io/badge/License-MIT-blue.svg"></a>
  <img alt="Status" src="https://img.shields.io/badge/status-alpha-yellow.svg">
  <img alt="Go" src="https://img.shields.io/badge/built%20with-Go-00ADD8.svg">
</p>

---

> 🚧 **Alpha.** The `service` vertical (deploy, scale, rollback, status) and the interactive
> TUI work today on AWS ECS. Registry, databases and more capabilities are on the
> [roadmap](docs/superpowers/plans/2026-06-15-roadmap.md).
> Design: [`docs/design.md`](docs/design.md).

## What is Steer?

Steer makes deploying to your own cloud **stop being scary**. Platforms like Vercel or
Railway made shipping effortless — but they lock you into their platform. Steer brings that
same "just ship it" feeling to the cloud account *you already own*.

The name evokes *steering the course* — you're at the helm. It ships as a **single static
binary** with two faces over one shared core:

- **TUI** — `steer tui`: a mouse-driven dashboard in the spirit of lazydocker/lazygit.
  Pick a service in the sidebar, hit a button (or `d`/`s`/`R`) and confirm in an inline
  form; the rollout streams live into the Events panel. This is where someone who doesn't
  live in AWS gets things done without fear.
- **CLI** — scriptable and CI-friendly:
  `steer --context stg service deploy -s my-svc -t v1.2.3 -y`.
  First-class for the lead and for pipelines.

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

- 🛡️ **Guardrails by default** — contexts marked `writable = false` (e.g. prod) block every
  mutating action, in both CLI and TUI. Deploys preview *what will happen* and ask before
  applying. Rollback is one command. Hard to break things by accident.
- 🖱️ **A TUI you can actually click** — everything is mouse-friendly: services, section
  headers, tabs, buttons and forms. Keyboard works everywhere too, and every shortcut is
  visible on screen.
- 📡 **Live rollouts** — deploy from the TUI (or `--watch` from the CLI) and follow the
  rollout events in real time until it completes or fails.
- 🗂️ **Built for big lists** — collapsible sidebar sections, a live `/` filter and
  scrolling with `↑/↓ more` indicators keep dozens of services navigable.
- 🌐 **Contexts** — each environment/account/cloud is a context; switch from inside the TUI
  (click the top bar or press `c`) or with `--context` / `STEER_CONTEXT` on the CLI.
- ⚙️ **Config-driven** — point Steer at *your* accounts, roles, environments and naming via
  `steer.toml`. No code changes.
- 📦 **Single binary** — no runtime to manage.
- ☁️ **AWS today, multi-cloud ready** — built on cloud-agnostic capability interfaces, so
  Azure/GCP can be added later without reworking the core.

## Quick start

```bash
# 1. Configure (once, by whoever knows the cloud)
cp steer.example.toml steer.toml   # then fill in your accounts/roles

# 2. Open the dashboard
steer tui

# 3. Or script it
steer service status
steer --context stg service deploy -s my-svc -t v1.2.3 --watch
steer service scale -s my-svc --count 3
steer service rollback -s my-svc
```

In the TUI: `↑/↓` or click to select · `d` deploy · `s` scale · `R` rollback · `/` filter ·
`c` switch context · `enter`/`space` collapse a section · `q` quit.

## Status & roadmap

V1 targets **AWS** behind agnostic interfaces. Commands are named by **capability**, not by
AWS service, so the same command keeps working when other clouds are added.

1. ✅ Foundation — config + AWS session + capability interfaces + CLI skeleton
2. ✅ **`service`** — deploy / scale / rollback / status with preview and watch
3. ✅ TUI dashboard — mouse-driven, multi-context, live rollouts
4. ✅ **`image`** (ECR) — repos & tags in TUI and CLI, deploy tag-picker, registry-validated deploys, stuck-rollout detection
5. ⏳ Distribution (Homebrew) & onboarding wizard — next up
6. `service logs/events`, `db`, `promote`, `⌘k` palette, minor capabilities

See [`docs/superpowers/plans/2026-06-15-roadmap.md`](docs/superpowers/plans/2026-06-15-roadmap.md).

## Configuration

Steer reads `steer.toml` from the current repo or `~/.config/steer/steer.toml`.
See [`steer.example.toml`](steer.example.toml): you declare environments per provider
(profile, account, role, `writable`) and naming templates that resolve short names to real
resource names. Your config (accounts, role ARNs) stays private — never commit it to a
public repo.

## License

[MIT](LICENSE) © 2026 [juanMaAV92](https://github.com/juanMaAV92).

You may use, modify and distribute this software freely, **but you must preserve the
copyright notice and license** — attribution to the author is required, and you may not
pass it off as your own.
