# Distribución (hito 07) — Diseño

**Fecha:** 2026-07-06 · **Estado:** aprobado
**Objetivo:** que el equipo instale y actualice steer sin compilar: `brew install` en
mac/linux, zip en Windows, releases automáticos por tag. El flujo completo de publicar
una versión debe ser **un comando** (`git tag vX.Y.Z && git push origin vX.Y.Z`).

## Decisiones (votadas)

1. **Matriz de plataformas:** macOS arm64 + amd64, Linux amd64 + arm64, Windows amd64.
   Binario Go puro (sin CGO) → compilación cruzada desde un runner Linux.
2. **Canales v1:** Homebrew tap (mac y linux), binarios directos en GitHub Releases
   (tar.gz unix / **zip Windows**) con checksums, y `go install` documentado (ya
   funciona). **Windows: solo zip** — Scoop diferido hasta que alguien lo pida (stanza
   de minutos); winget/Chocolatey descartados.
3. **Tap:** repo nuevo `juanMaAV92/homebrew-tap` (nombre genérico, sirve para futuras
   herramientas). GoReleaser crea y mantiene `Formula/steer.rb` en cada release.
4. **Versionado:** semver con tags `v*`; primer release **`v0.1.0`** (alpha honesto,
   como el badge del README). `steer --version` muestra la versión del tag.
5. **Sin firma/notarización de macOS en v1:** brew no la necesita; para descargas
   directas por navegador se documenta `xattr -d com.apple.quarantine`. El certificado
   de Apple queda para cuando haya usuarios externos.

## Piezas

### `.goreleaser.yaml`

- `builds`: main `./cmd/steer`, `env: [CGO_ENABLED=0]`, `goos: [darwin, linux, windows]`,
  `goarch: [amd64, arm64]`, `ignore: windows/arm64`,
  `ldflags: -s -w -X main.version={{.Version}}` (la costura `var version = "dev"` ya existe).
- `archives`: tar.gz con `format_overrides: windows → zip`; incluye LICENSE y README.
- `checksum`, `snapshot` (para builds locales de prueba), `changelog` agrupado por
  conventional commits (`feat:` → Features, `fix:` → Bug Fixes; excluir `docs:`/`test:`/
  `refactor:`/`chore:`).
- `brews`: fórmula `steer` → repo `juanMaAV92/homebrew-tap`, carpeta `Formula`,
  homepage y descripción del README, `test` block con `steer --version`, token vía
  env `TAP_GITHUB_TOKEN`.

### Workflow `.github/workflows/release.yml`

- Dispara con `push: tags: ['v*']`.
- Pasos: checkout (con `fetch-depth: 0` — el changelog necesita historia), setup-go 1.26,
  `goreleaser/goreleaser-action@v6` con `args: release --clean`.
- Secrets: `GITHUB_TOKEN` (releases, lo da Actions) + `TAP_GITHUB_TOKEN` (PAT
  fine-grained con contents:write SOLO sobre `homebrew-tap`).
- El CI existente (`ci.yml`) gana un paso `goreleaser check` (valida el yaml en cada PR
  sin publicar nada).

### Repo del tap + pasos manuales del usuario

Una sola vez (documentados en el plan, ejecutables con `gh`):
1. Crear `juanMaAV92/homebrew-tap` público y vacío (`gh repo create`).
2. Crear el PAT fine-grained (contents: read/write, solo ese repo) y guardarlo:
   `gh secret set TAP_GITHUB_TOKEN`.

### Docs

README: sección **Install** reescrita —

```bash
brew install juanMaAV92/tap/steer          # macOS y Linux
go install github.com/juanMaAV92/steer/cmd/steer@latest
# Windows: descargar steer_*_windows_amd64.zip del último Release y añadir al PATH
```

- Nota Windows: usar **Windows Terminal** (la TUI con mouse degrada en cmd.exe clásico).
- Nota mac descarga-directa: `xattr -d com.apple.quarantine ./steer`.
- Roadmap: 07 → ✅; `parity.md` no cambia (distribución no es una capacidad).

## Flujo de release (resultado final)

```
git tag v0.1.0 && git push origin v0.1.0
```
→ workflow → GoReleaser compila los 5 binarios → GitHub Release (archivos + checksums +
changelog automático) → fórmula actualizada en el tap. El equipo: `brew upgrade steer`.
Una versión mala se corrige publicando la siguiente (o borrando tag+release).

## Fuera de alcance

Scoop/winget/Chocolatey; deb/rpm (nfpm) y AUR; imagen Docker; firma Apple/notarización;
auto-update embebido en el binario; release candidates/canales beta.

## Pruebas / verificación

- `goreleaser check` en CI (cada PR) y localmente.
- `goreleaser release --snapshot --clean` local: verifica que los 5 artefactos se
  construyen y que `./dist/.../steer --version` reporta la versión snapshot.
- El primer release real (`v0.1.0`) es la prueba end-to-end: Release publicado,
  fórmula en el tap, `brew install juanMaAV92/tap/steer` funcionando en esta máquina.
