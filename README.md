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

> 🚧 **Alpha.** The `service` vertical (deploy, scale, rollback, status, logs, events) and
> the interactive TUI work today on AWS ECS. Registry, databases and more capabilities are
> on the [roadmap](docs/superpowers/plans/2026-06-15-roadmap.md).
> Design: [`docs/design.md`](docs/design.md).
>
> 📖 **Documentation:** <https://juanmaav92.github.io/steer/docs>

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

## Install & update

### macOS — Homebrew (recommended)

```bash
# install
brew install juanMaAV92/tap/steer

# update
brew update && brew upgrade steer
```

No Gatekeeper prompts: the cask clears the quarantine flag on install.

### Linux (and any OS with Go installed)

```bash
# install AND update — same command, always fetches the latest release
go install github.com/juanMaAV92/steer/cmd/steer@latest
```

Without Go: download `steer_*_linux_{amd64,arm64}.tar.gz` from the
[latest release](https://github.com/juanMaAV92/steer/releases/latest) and place `steer`
in your `PATH`. To update, download the new release and replace the binary.

### Windows

Download `steer_*_windows_amd64.zip` from the
[latest release](https://github.com/juanMaAV92/steer/releases/latest), unzip and add
`steer.exe` to your `PATH`. To update, download the new zip and replace `steer.exe`.

Use **Windows Terminal** — the TUI (mouse included) degrades in the legacy `cmd.exe` console.

### Notes

- Check your version anytime with `steer --version`; new versions are announced on the
  [releases page](https://github.com/juanMaAV92/steer/releases) with an automatic changelog.
- **macOS direct download** (releases page, not brew): binaries are not notarized; clear
  the quarantine flag once with `xattr -d com.apple.quarantine ./steer`.

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
  applying. Rollback is one command. Resizing CPU/memory only offers combos your provider
  actually supports — via a picker in the TUI, and teaching errors in the CLI. Hard to
  break things by accident.
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
steer config init   # interactive setup — detects your AWS profiles
# (or: cp steer.example.toml steer.toml && edit it by hand; `config init --example` writes the same starter file)

# 2. Open the dashboard
steer tui

# 3. Or script it
steer service status
steer --context stg service deploy -s my-svc -t v1.2.3 --watch
steer service scale -s my-svc --count 3
steer service resize -s my-svc --cpu 0.5 --memory 2GB
steer service rollback -s my-svc
steer service logs -s my-svc -f
steer service events -s my-svc
```

In the TUI: `↑/↓` or click to select · `d` deploy · `s` scale · `R` rollback · `/` filter ·
`c` switch context · `enter`/`space` collapse a section · `q` quit.

## Status & roadmap

V1 targets **AWS** behind agnostic interfaces. Commands are named by **capability**, not by
AWS service, so the same command keeps working when other clouds are added.

1. ✅ Foundation — config + AWS session + capability interfaces + CLI skeleton
2. ✅ **`service`** — deploy / scale / rollback / resize / status with preview and watch
3. ✅ TUI dashboard — mouse-driven, multi-context, live rollouts
4. ✅ **`image`** (ECR) — repos & tags in TUI and CLI, deploy tag-picker, registry-validated deploys, stuck-rollout detection
5. ✅ Distribution — Homebrew tap (macOS), 5-platform releases
6. ✅ Onboarding wizard — interactive `config init`, plus `config add/remove/list`
7. ✅ `service logs/events` — CLI (`logs -s [-f] [-n]`, `events -s`) and live TUI tabs
8. `db`, `promote`, `⌘k` palette, minor capabilities

See [`docs/superpowers/plans/2026-06-15-roadmap.md`](docs/superpowers/plans/2026-06-15-roadmap.md).

## Configuration

Steer reads `steer.toml` from the current repo or `~/.config/steer/steer.toml`.
A **context** is one target = cloud + credential + cluster + naming templates + `writable`.
Several accounts, environments, or projects are just several contexts; switch between them
with `c` in the TUI or `--context` on the CLI. Your config (accounts, role ARNs) stays
private — never commit it to a public repo.

The fastest way in is the interactive wizard — it reads your AWS profiles, lists your real
clusters to pick from, and runs a connection smoke test:

```bash
steer config init          # interactive setup for the first context
steer config add           # add another account/environment later
steer config list          # NAME · CLOUD · CLUSTER · MODE · DEFAULT
steer config remove <name> # drop a context (reassigns the default if needed)
steer config validate      # check the discovered steer.toml
```

Prefer editing by hand? `steer config init --example` writes a starter file (see also
[`steer.example.toml`](steer.example.toml)) — `steer.toml` is plain TOML: add or tweak a
context, then `steer config validate`. If a command hits an AWS problem (expired SSO,
missing credentials, wrong cluster), the error tells you what to run to fix it.

## License

[MIT](LICENSE) © 2026 [juanMaAV92](https://github.com/juanMaAV92).

You may use, modify and distribute this software freely, **but you must preserve the
copyright notice and license** — attribution to the author is required, and you may not
pass it off as your own.
