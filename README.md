<h1 align="center">⛵ Steer</h1>

<p align="center">
  <b>Steer your cloud from the terminal.</b><br>
  Deploy, scale and monitor your infrastructure — as a scriptable CLI and an interactive TUI.
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

Steer is an open-source cloud operations tool. The name evokes *steering the course* —
you're at the helm of your cloud. It ships as a **single static binary** with two faces
over one shared core:

- **CLI** — scriptable and CI-friendly: `steer -e stg ecs deploy -s my-svc -t v1.2.3 -y`
- **TUI** — an interactive hybrid dashboard with a command palette: `steer tui`

No more memorizing service names: running a command without arguments opens an
interactive picker (fuzzy filter + multi-select) populated live from your cloud.

### Highlights

- 🚀 **Interactive deploys** — pick services and image tags from live, fuzzy-filtered lists.
- 📊 **Hybrid TUI** — health of everything at a glance + a `⌘k` command palette (k9s/lazygit style).
- ⚙️ **Config-driven** — point Steer at *your* accounts, roles, environments and naming via `steer.toml`. No code changes.
- 📦 **Single binary** — install via Homebrew, `go install`, or a direct download. No runtime to manage.
- ☁️ **AWS today, multi-cloud ready** — built on cloud-agnostic capability interfaces, so Azure/GCP can be added later without reworking the core.

## Status & roadmap

V1 targets **AWS** behind agnostic interfaces. Migration is incremental, domain by domain:

1. Core + config + AWS session
2. **ECS** — interactive deploy + status
3. ECR, RDS, SQS, obs, env, S3, EC2
4. Hybrid TUI dashboard

## Configuration

Steer reads `steer.toml` from the current repo or `~/.config/steer/steer.toml`.
See [`steer.example.toml`](steer.example.toml). Your config (accounts, role ARNs) stays
private — never commit it to a public repo.

## License

[MIT](LICENSE) © 2026 [juanMaAV92](https://github.com/juanMaAV92).

You may use, modify and distribute this software freely, **but you must preserve the
copyright notice and license** — attribution to the author is required, and you may not
pass it off as your own.
