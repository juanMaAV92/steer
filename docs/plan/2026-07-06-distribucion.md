# Distribución (hito 07) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Releases automáticos por tag: `git tag v0.1.0 && git push origin v0.1.0` produce binarios para 5 plataformas en GitHub Releases + fórmula Homebrew actualizada en `juanMaAV92/homebrew-tap` (ya creado).

**Architecture:** GoReleaser hace todo (builds cruzados sin CGO desde un runner Linux, archivos, checksums, changelog por conventional commits, push de la fórmula); un workflow de Actions lo dispara por tag; el CI existente valida el yaml en cada PR. La costura de versión (`var version = "dev"` en cmd/steer/main.go) ya existe.

**Tech Stack:** GoReleaser v2, GitHub Actions, Homebrew tap.
Spec: `docs/superpowers/specs/2026-07-06-distribucion-design.md`.

## Global Constraints

- PROHIBIDO cualquier atribución a Claude/IA en commits o archivos (sin trailer Co-Authored-By).
- Matriz exacta: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64 (windows/arm64 excluido). `CGO_ENABLED=0`; `ldflags: -s -w -X main.version={{.Version}}`.
- Windows empaqueta en **zip**; unix en tar.gz; ambos incluyen LICENSE y README.md.
- Tap: `juanMaAV92/homebrew-tap` (existe), fórmula en carpeta `Formula`, token vía env `TAP_GITHUB_TOKEN`.
- Changelog agrupado: `feat:` → Features, `fix:` → Bug fixes; excluir `^docs`, `^test`, `^refactor`, `^chore`, `^ci`.
- No se publica NADA sin checkpoint del usuario: el tag `v0.1.0` (Task 3) requiere su PAT y su OK explícito.
- Verificación local antes de commitear config: `goreleaser check` y `goreleaser release --snapshot --clean` (instalar con `brew install goreleaser` si falta).

---

### Task 1: `.goreleaser.yaml` + validación en CI

**Files:**
- Create: `.goreleaser.yaml`
- Modify: `.github/workflows/ci.yml` (paso `goreleaser check`)

**Interfaces:**
- Consumes: `var version = "dev"` (`cmd/steer/main.go:15`), LICENSE y README.md en la raíz (verificar que LICENSE existe; si no, PARAR y reportar).
- Produces: config que Task 2 dispara desde el workflow; artefactos `dist/` locales en snapshot.

- [ ] **Step 1: Crear la branch e instalar goreleaser**

```bash
git checkout main && git pull && git checkout -b feat/distribucion
command -v goreleaser || brew install goreleaser
goreleaser --version   # esperar v2.x
ls LICENSE README.md   # ambos deben existir
```

- [ ] **Step 2: Escribir `.goreleaser.yaml`**

```yaml
# Configuración de release: binarios multiplataforma + Homebrew tap.
# Verificar cambios con: goreleaser check && goreleaser release --snapshot --clean
version: 2

project_name: steer

before:
  hooks:
    - go mod tidy

builds:
  - id: steer
    main: ./cmd/steer
    binary: steer
    env:
      - CGO_ENABLED=0
    goos: [darwin, linux, windows]
    goarch: [amd64, arm64]
    ignore:
      - goos: windows
        goarch: arm64
    ldflags:
      - -s -w -X main.version={{.Version}}
    mod_timestamp: "{{ .CommitTimestamp }}"

archives:
  - id: default
    formats: [tar.gz]
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        formats: [zip]
    files:
      - LICENSE
      - README.md

checksum:
  name_template: checksums.txt

snapshot:
  version_template: "{{ incpatch .Version }}-next"

changelog:
  sort: asc
  groups:
    - title: Features
      regexp: "^feat"
      order: 0
    - title: Bug fixes
      regexp: "^fix"
      order: 1
    - title: Others
      order: 999
  filters:
    exclude:
      - "^docs"
      - "^test"
      - "^refactor"
      - "^chore"
      - "^ci"

brews:
  - name: steer
    repository:
      owner: juanMaAV92
      name: homebrew-tap
      token: "{{ .Env.TAP_GITHUB_TOKEN }}"
    directory: Formula
    homepage: https://github.com/juanMaAV92/steer
    description: "The simplicity of a PaaS, on your own AWS — deploy, scale and watch services from CLI/TUI"
    license: MIT
    test: |
      system "#{bin}/steer --version"
```

NOTA de compatibilidad: si `goreleaser check` rechaza `formats` (esquema viejo), usar
`format: tar.gz` / `format: zip` en singular. Si rechaza `version_template` en snapshot,
usar `name_template`. Adaptar SOLO ante error del check y documentarlo en el reporte.

- [ ] **Step 3: Validar y probar snapshot local**

```bash
goreleaser check
TAP_GITHUB_TOKEN=dummy goreleaser release --snapshot --clean --skip=publish
ls dist/ | sort
```

Expected: check "1 configuration file(s) validated"; en `dist/`: 5 archivos
(`steer_*_darwin_amd64.tar.gz`, `darwin_arm64`, `linux_amd64`, `linux_arm64`,
`windows_amd64.zip`) + `checksums.txt`. Verificar la versión inyectada:

```bash
./dist/steer_darwin_arm64*/steer --version   # (el dir de la arch local); esperar "0.x.y-next"
```

- [ ] **Step 4: Paso de validación en el CI**

En `.github/workflows/ci.yml`, tras el paso `go build ./...`:

```yaml
      - name: goreleaser check
        uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: check
```

- [ ] **Step 5: Gates + commit**

```bash
echo "dist/" >> .gitignore   # los artefactos locales no se commitean
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add .goreleaser.yaml .github/workflows/ci.yml .gitignore
git commit -m "feat(release): configuración de GoReleaser con matriz de 5 plataformas y tap"
```

---

### Task 2: Workflow de release + docs de instalación

**Files:**
- Create: `.github/workflows/release.yml`
- Modify: `README.md` (sección Install nueva; quick start ya no asume binario mágico)
- Modify: `docs/superpowers/plans/2026-06-15-roadmap.md` (07 → ✅)

**Interfaces:**
- Consumes: `.goreleaser.yaml` (T1); secrets `GITHUB_TOKEN` (lo da Actions) y `TAP_GITHUB_TOKEN` (lo crea el usuario en T3).
- Produces: el pipeline que T3 dispara con el tag.

- [ ] **Step 1: Crear `.github/workflows/release.yml`**

```yaml
name: release
on:
  push:
    tags: ["v*"]
permissions:
  contents: write
jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 } # el changelog necesita la historia completa
      - uses: actions/setup-go@v5
        with: { go-version: "1.26" }
      - uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAP_GITHUB_TOKEN: ${{ secrets.TAP_GITHUB_TOKEN }}
```

- [ ] **Step 2: README — sección Install**

Insertar tras "## What is Steer?" (antes de "## Who is it for?"):

```markdown
## Install

```bash
# macOS & Linux (Homebrew)
brew install juanMaAV92/tap/steer

# Any platform with Go installed
go install github.com/juanMaAV92/steer/cmd/steer@latest
```

**Windows:** download `steer_*_windows_amd64.zip` from the
[latest release](https://github.com/juanMaAV92/steer/releases/latest), unzip and add
`steer.exe` to your `PATH`. Use **Windows Terminal** — the TUI (mouse included) degrades
in the legacy `cmd.exe` console.

**macOS direct download note:** binaries from the releases page (not brew) are not
notarized; clear the quarantine flag once: `xattr -d com.apple.quarantine ./steer`.
```

(El bloque interior usa fences: anidarlos correctamente en el markdown real.)
Actualizar también el item 5 del roadmap del README: "⏳ Distribution (Homebrew) &
onboarding wizard — next up" → "✅ Distribution — Homebrew tap, 5-platform releases ·
⏳ onboarding wizard — next up" (ajustar numeración si hace falta).

- [ ] **Step 3: Roadmap — 07 ✅**

En `docs/superpowers/plans/2026-06-15-roadmap.md`: fila 07 → `✅ hecho (plan
2026-07-06-distribucion.md)` describiendo lo entregado (GoReleaser, tap, release por
tag, 5 plataformas, Windows zip-only con Scoop diferido); moverlo de **Pendiente** a
**Mergeado a main** en la sección de estado.

- [ ] **Step 4: Gates + commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add .github/workflows/release.yml README.md docs/superpowers/plans/2026-06-15-roadmap.md
git commit -m "feat(release): workflow de release por tag y docs de instalación"
```

---

### Task 3: Primer release `v0.1.0` (CHECKPOINT con el usuario — la ejecuta el controlador, no un subagente)

**Files:** ninguno en el repo de steer (merge previo a main + tag + verificación).

**Interfaces:**
- Consumes: todo lo anterior mergeado a main; el repo `juanMaAV92/homebrew-tap` (existe).
- Produces: Release `v0.1.0` publicado, `Formula/steer.rb` en el tap, `brew install juanMaAV92/tap/steer` funcionando.

- [ ] **Step 1: Merge de la rama a main** (flujo finishing-a-development-branch normal).

- [ ] **Step 2: PAT del usuario (manual, ~2 min)** — pedirle al usuario:
  1. github.com → Settings → Developer settings → Personal access tokens →
     **Fine-grained tokens** → Generate new token.
  2. Nombre `steer-tap-releases`; Repository access: **Only select repositories** →
     `juanMaAV92/homebrew-tap`; Permissions → Repository permissions → **Contents:
     Read and write**. Expiración a gusto (1 año recomendado).
  3. Copiar el token y en la terminal del repo steer:
     `gh secret set TAP_GITHUB_TOKEN` (pega el token cuando lo pida).
  Verificar: `gh secret list` muestra `TAP_GITHUB_TOKEN`.

- [ ] **Step 3: Tag y push (SOLO con OK explícito del usuario)**

```bash
git tag v0.1.0 && git push origin v0.1.0
gh run watch --exit-status   # seguir el workflow de release
```

- [ ] **Step 4: Verificación end-to-end**

```bash
gh release view v0.1.0 --json assets --jq '.assets[].name'   # 5 archivos + checksums.txt
gh api repos/juanMaAV92/homebrew-tap/contents/Formula/steer.rb --jq .name
brew install juanMaAV92/tap/steer && steer --version          # esperar 0.1.0
```

Si el workflow falla: leer el log (`gh run view --log-failed`), corregir en una rama,
mergear, **borrar tag y release** (`gh release delete v0.1.0 --cleanup-tag -y`) y
re-taggear. Documentar cualquier corrección en el ledger.
