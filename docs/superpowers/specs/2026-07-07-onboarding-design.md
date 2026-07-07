# Onboarding (hito 08) — Diseño

**Fecha:** 2026-07-07 · **Estado:** aprobado
**Objetivo:** el primer minuto post-instalación sin miedo: `steer config init` es un
wizard que detecta perfiles AWS, propone contextos eligiendo de listas reales, valida,
prueba la conexión, y los errores comunes de AWS enseñan cómo arreglarse. El ciclo de
vida de la config (agregar/quitar cuentas) queda a un comando.

## Decisiones (votadas)

1. **Detección en vivo con fallback.** Tras elegir perfil, el wizard llama
   `ListClusters` con esas credenciales y el cluster se ELIGE de una lista real. Si las
   credenciales fallan, degrada a input manual mostrando el error amable con su
   remedio. Al final, smoke test: lista los servicios del primer contexto
   ("connected: 16 services found").
2. **Ubicación: preguntar, default global.** Último paso: ¿global
   (`~/.config/steer/steer.toml`, default) o este repo (`./steer.toml`)? El buscador de
   config ya soporta ambas; per-repo habilita el caso multi-proyecto.
3. **Ciclo de vida completo, no solo creación:**
   - `config init` sin config → wizard desde cero. Con config existente → muestra lo
     que hay y ofrece **agregar contexto** o **recrear desde cero** (confirmación
     explícita); ya no falla con "steer.toml already exists".
   - `config add` — wizard de UN contexto, anexado al toml existente.
   - `config remove <name>` — con confirmación; avisa si era `default_context` (y lo
     reasigna u ofrece elegir).
   - `config list` — nombre · cloud · cluster · writable · default.
   - Editar es manual (TOML legible + `config validate`); YAGNI el wizard de edición.
   - `config init --example` conserva el dump estático actual (scripts/docs).
4. **Escalabilidad multi-cloud/multi-cuenta/multi-proyecto:** el flujo del wizard es
   agnóstico (detectar credenciales → elegir destino → proponer contexto → validar →
   smoke test); solo el **detector** es por-provider (interface `Detector`; AWS hoy;
   GCP/Azure implementarán el suyo). El paso "cloud" usa `providers.IsImplemented`
   (gcp/azure visibles como "not implemented yet"). Varias cuentas = varios contextos
   (ya soportado); varios proyectos = contextos nombrados o steer.toml per-repo.
5. **Errores que enseñan** en el wizard Y en el arranque normal de CLI/TUI.

## Wizard — flujo (`huh`, la lib de formularios ya en uso)

1. **Cloud**: select (aws; gcp/azure deshabilitados "not implemented yet").
2. **Perfil AWS**: select con los perfiles parseados de `~/.aws/config` (secciones
   `[profile X]`) y `~/.aws/credentials` (`[X]`), deduplicados y ordenados; si no hay
   ninguno, mensaje que enseña (instalar AWS CLI / `aws configure` / SSO) y opción de
   teclearlo.
3. **Región** (opcional): input con default de la config del perfil si se puede leer.
4. **Cluster**: `ListClusters` en vivo → select de nombres reales. Fallo → error amable
   + input manual.
5. **Propuestas deducidas** (editables): nombre del contexto (del cluster, sin sufijo
   `-cluster`), `service_template` (patrón `<cluster-sin-sufijo>-{name}` — el del
   ejemplo real: `nao-v2-dev-cluster` → `nao-v2-dev-{name}`), `writable` (default
   false si el nombre contiene prod/prd/production, true si no).
6. **Images** (opcional): ¿configurar el registry? → `repo_template` con input
   (default: prefijo del service_template sin el segmento de ambiente, editable).
7. **¿Agregar otro contexto?** → loop al paso 2.
8. **default_context**: select entre los creados (default: el primero).
9. **Ubicación** (decisión 2) → escribir con permisos 0600 → `config.Validate()`.
10. **Smoke test**: construir el provider del default y `ListServices`; éxito →
    "✓ connected — N services in <cluster>. Try: steer tui"; fallo → error amable
    (la config queda escrita; el remedio se muestra).

Cancelar (`esc`/ctrl+c) en cualquier paso: no se escribe nada (o se conserva el toml
original intacto en modo add).

## Errores que enseñan

Traductor `internal/providers/aws/errors.go`: `func FriendlyError(err error) string` —
mapea los fallos comunes al formato *qué pasó · por qué · qué hacer*:

| Error AWS | Mensaje (inglés, formato "X — try: Y") |
|---|---|
| SSO token expirado/inválido | `AWS session expired for profile "dev" — try: aws sso login --profile dev` |
| Sin credenciales / perfil inexistente | `no AWS credentials found for profile "X" — try: aws configure --profile X (or check ~/.aws/config)` |
| AccessDenied | `access denied: your role can't call <operation> — ask whoever manages AWS for ECS read permissions` |
| ClusterNotFoundException | `cluster "X" not found in this account/region — check the cluster name in steer.toml or the region of the profile` |
| Timeout/red | `could not reach AWS — check your network/VPN and try again` |

- Se aplica: en cada fallo del wizard, en el arranque del TUI (pantalla de error) y en
  los `RunE` del CLI (envolviendo el error del provider). Sin flag `--verbose` (YAGNI):
  el mensaje amable incluye una línea final tenue con el error original truncado
  (transparencia sin superficie nueva).
- La detección es por tipos del SDK cuando existen (`errors.As`) y por substring
  documentado cuando no (patrón `IsProvisioningFailure`).

## Arquitectura

- `internal/cli/wizard/` (paquete nuevo): el flujo agnóstico (`Run(detector) (Config,
  error)`) + los formularios `huh`. Sin lógica AWS.
- `Detector` interface (en el paquete wizard): `Profiles() ([]string, error)`,
  `Clusters(ctx, profile, region) ([]string, error)`, `SmokeTest(ctx, config.Context)
  (int, error)`. Implementación AWS en `internal/providers/aws` (reutiliza
  LoadConfigForContext + un cliente ECS ListClusters — el ecsAPI gana ListClusters).
- Escritura del toml: serializador propio pequeño (el formato es estable y comentado)
  o `BurntSushi/toml` encoder — decisión del plan; DEBE preservar contextos existentes
  en modo add/remove (leer → modificar → escribir; los comentarios del usuario en el
  toml NO se preservan — limitación documentada en el output: "rewrote steer.toml").
- `config` gana helpers: `Config.AddContext`, `RemoveContext` (con manejo de
  default_context), y un `Write(path)`.

## Fuera de alcance

Wizard de edición de contextos; detectores GCP/Azure; import desde variables de
entorno/instance roles; TUI embebiendo el wizard (si el TUI arranca sin config, imprime
"run: steer config init" — el gap queda cerrado en parity.md nota 2); soporte de
múltiples archivos de config simultáneos.

## Pruebas

- Wizard: el flujo es testeable inyectando un `Detector` fake y respuestas
  programadas de huh (o factorizando la lógica de propuestas —
  `proposeContext(cluster) Context` — como funciones puras con unit tests: deducción
  de nombre/template/writable).
- Detector AWS: parsing de `~/.aws/config`/`credentials` con fixtures (perfiles,
  dedup, orden); ListClusters con el fake ecsAPI.
- FriendlyError: cada mapeo con errores fabricados del SDK; el fallback deja el error
  original.
- Ciclo de vida: init-desde-cero, init-con-config-existente (agregar/recrear), add,
  remove (incluido remove del default), list — sobre archivos temporales, verificando
  que Validate pasa y que los contextos previos sobreviven intactos.
- CLI/TUI: el arranque con SSO vencido muestra el mensaje amable (fake que devuelve el
  error del SDK).
- Paridad: capacidad CLI-only aceptada (parity.md nota 2 ya lo documenta; se actualiza
  la nota: el TUI sin config ahora apunta al wizard).
