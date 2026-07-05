# Steer — Diseño V1

**Estado:** Documento vivo — visión original de 2026-06, secciones 4/6/7 realineadas con
lo construido el 2026-07-05. El estado real por hito vive en
[`docs/superpowers/plans/2026-06-15-roadmap.md`](superpowers/plans/2026-06-15-roadmap.md);
la paridad CLI↔TUI en [`docs/parity.md`](parity.md).

---

## 1. Visión y alcance

**Steer** es una herramienta open source de operaciones cloud: despliega, escala y
monitorea infraestructura desde la terminal.

El nombre evoca *gobernar el rumbo* (timón): tú diriges el movimiento de tu cloud. Se lee
como verbo-comando: `steer service deploy`.

- **Nombre / binario / comando:** `steer`
- **Config:** `steer.toml`
- **Repo / module path:** `github.com/juanMaAV92/steer`
- **Lenguaje:** Go (binario único estático)
- **Alcance V1:** AWS-only, con las costuras (interfaces agnósticas) listas para multi-cloud
- **Distribución:** binario único multiplataforma

### Objetivos
1. **Que cualquiera lo use** → genericización total vía config; cero acoplamiento a una infra concreta.
2. **CLI + TUI** → dashboard interactivo híbrido sobre el mismo core que el CLI.
3. **Adopción OSS sin fricción** → binario estático, sin dependencias de runtime.

### No-objetivos del V1 (YAGNI)
- API pública de plugins de terceros (los dominios vienen compilados; extensibilidad = config).
- Implementaciones reales de Azure / GCP (solo se deja la costura/interfaces).
- Migrar comandos imperativos a "solo TUI" (el CLI no-interactivo se mantiene íntegro).

### Frontera de alcance
Steer incluye **solo capacidades genéricas y reutilizables**. Regla: si una capacidad no
aplica a un usuario externo cualquiera de AWS (porque depende de una infra propietaria),
no entra a Steer.

---

## 1.b Usuarios objetivo y posicionamiento

**Wedge:** *hacer que desplegar deje de dar miedo* a personas que no dominan cloud, sin
sacarlas de su propio AWS. La competencia no es k9s (herramienta de power-users), sino el
hueco entre "la consola de AWS asusta" y los PaaS (Vercel/Railway/Render) que resuelven la
simplicidad pero imponen su plataforma.

**Pitch:** *"La simplicidad de un PaaS, sobre tu propio AWS. Configúralo una vez; luego
cualquiera despliega sin tocar la consola ni memorizar comandos."*

**Dos perfiles de usuario:**
1. **Quien sabe cloud** (lead / plataforma) configura `steer.toml` una vez.
2. **El resto del equipo** despliega/escala/consulta con comandos simples + TUI, sin
   entender AWS por debajo.

Patrón lazygit: no enseña el motor, quita el miedo. **Sweet spot:** equipos pequeños con 1
persona técnica + N que solo quieren shippear. **Fuera de sweet spot (honesto):** el dev
solo sin nadie que haga el setup → mejor un PaaS gestionado.

### Re-ponderación de prioridades (derivada del target)
El diseño técnico no cambia; cambia el peso de las features:
1. **Guardrails > potencia.** Prod read-only, confirmaciones, **preview de "qué va a pasar"**
   antes de actuar, y **rollback a un comando**. Que no se pueda romper algo importa más que
   mil flags. La seguridad real viene del preview+rollback, no de esconder complejidad.
2. **TUI y pickers = EL producto**, no un extra. El dev no-cloud vive en la TUI; el CLI
   scriptable es para el lead y CI (sigue siendo de primera clase — ambos sobre el mismo core).
3. **Mensajes de error que enseñan:** *falló X porque Y, prueba Z.*
4. **Onboarding impecable** y **defaults sensatos:** el primer minuto decide la adopción.
   Nota: el wizard de onboarding completo (`config init` que detecta perfiles AWS, valida y
   da errores amables) es **scope de un plan futuro** (ver roadmap, Plan 08), no del Hito 1.

---

## 2. Decisiones de diseño (con racional)

| Decisión | Elección | Por qué |
|---|---|---|
| Lenguaje | Go | Binario único estático → mata la fricción de instalación. Ecosistema TUI (Charm) y AWS SDK v2 de primera. |
| Multi-cloud | AWS-only + interfaces agnósticas | Máximo valor ahora; costura barata lista para extender. Construir 3 backends sin necesidad = desperdicio. |
| Extensibilidad | Config-driven (`steer.toml`) | Cubre "que cualquiera lo use" sin el coste de diseñar/versionar una API de plugins pública. |
| CLI vs TUI | Ambos sobre un mismo core | Comandos de observación brillan en TUI; los imperativos son ideales como CLI scriptable/CI. |
| Modelo de dominios | Registro en tiempo de compilación | Go no hace carga dinámica por entry-points; el registro compilado es más simple y robusto. |
| Home de la TUI | Híbrido (dashboard + paleta ⌘k) | Patrón de k9s/lazygit: valor inmediato al abrir + velocidad para ejecutar. |
| Estilo de comandos | Sustantivo-verbo | Estándar de la industria (kubectl, gh, docker); escala limpio al añadir capacidades/providers. |
| Deploy interactivo | Picker fuzzy multi-select → tag → confirmar | Elimina el dolor de memorizar/escribir nombres de servicio. No rompe el modo no-interactivo. |

---

## 3. Arquitectura en capas

```
┌─ Frentes ────────────────────────────────────────┐
│  CLI (Cobra)              TUI (Bubble Tea)         │
│  cmd/service/deploy.go    internal/tui/...         │
└───────────────┬───────────────────┬───────────────┘
                │   (mismo core)     │
┌───────────────▼───────────────────▼───────────────┐
│  Capa de capacidades (interfaces agnósticas)       │
│  Deployer · Registry · ObjectStore · LogSource ··· │
└───────────────────────┬───────────────────────────┘
┌───────────────────────▼───────────────────────────┐
│  Provider AWS (implementa las interfaces)          │
│  internal/providers/aws/{service,registry,db,...}  │
└───────────────────────┬───────────────────────────┘
┌───────────────────────▼───────────────────────────┐
│  Config (steer.toml) + Sesión AWS                  │
│  cuentas · roles · entornos→profile · naming       │
└────────────────────────────────────────────────────┘
```

**Regla de oro:** la lógica AWS vive **una sola vez** en el provider. CLI y TUI solo la
invocan. Nunca se duplica una llamada a AWS entre frentes.

### Capa de capacidades (interfaces)
Las capacidades comunes se modelan como interfaces agnósticas de cloud. Los comandos y la
TUI dependen de la interface, nunca de un cloud concreto.

| Capacidad (interface) | AWS (V1) | Azure (futuro) | GCP (futuro) |
|---|---|---|---|
| `Deployer` (deploy/scale) | ECS | Container Apps | Cloud Run |
| `Registry` | ECR | ACR | Artifact Registry |
| `ObjectStore` | S3 | Blob | GCS |
| `LogSource` / métricas | CloudWatch | Monitor | Cloud Monitoring |

---

## 4. Superficie de comandos (V1)

Estilo **sustantivo-verbo**: el recurso primero, la acción después. Los nombres son por
**capacidad**, no por servicio AWS, para que el mismo comando funcione cuando se añadan
otros providers.

| Comando | Capacidad | Subcomandos construidos | Pendientes |
|---|---|---|---|
| `steer service` (alias `svc`) | Servicios / contenedores | `status` (alias `ls`, `-w`) · `deploy` (`-w`, validado contra registry, detección de atasco) · `scale` · `rollback` | `logs` · `events` · `redeploy` · `promote` |
| `steer image` (alias `img`) | Registro de imágenes | `ls` · `tags` (con marcador del tag desplegado) | — (`build`/`prune` fuera de alcance V1: solo lectura) |
| `steer db` | Base de datos | — | `status` · `slow-queries` · `tunnel` |
| `steer storage` | Almacenamiento de objetos | — | `ls` |
| `steer queue` | Colas | — | `ls` · `watch` |
| `steer host` | Hosts / instancias | — | `ls` · `connect` |
| `steer env` | Entornos (encender/apagar) | — | `ls` · `up` · `down` |
| `steer assets` | Estático / CDN | — | `deploy` · `url` · `info` · `invalidate` |

Notas de evolución respecto al diseño original: la capacidad de imágenes se llama
`image` (no `registry`) — el sustantivo que piensa el usuario; `watch` es el flag `-w`
de `status`/`deploy`, no un subcomando; el alias de camino caliente `steer deploy` no se
construyó (YAGNI hasta que alguien lo pida).

Global:
- `steer tui` — abre el dashboard interactivo.
- `steer config init|validate` — crea/valida `steer.toml` (CLI-only por diseño; ver `docs/parity.md`).
- `--context` — selecciona el contexto (cuenta+cluster+credencial); también
  `STEER_CONTEXT` o `default_context` del toml. `-e/--env` quedó como alias deprecado.

---

## 5. Componentes (paquetes Go)

| Paquete | Responsabilidad | Depende de |
|---|---|---|
| `internal/config` | Cargar/validar `steer.toml`; resolver entorno → sesión AWS | — |
| `internal/core` | Interfaces de capacidad (`Deployer`, `Registry`, `ObjectStore`, `LogSource`) | — |
| `internal/providers` | Fábrica de providers por contexto (`Provider` bundle con sesión cacheada: `Deployer()`, `Registry()`, …) | `core`, `config` |
| `internal/providers/aws` | Implementación AWS (un archivo por capacidad: ecs.go, registry.go, …) | `core`, `config` |
| `internal/cli` | Comandos Cobra (service, image, config, tui) — `cmd/steer` solo los registra | `core`, `config`, `render` |
| `internal/tui` | App Bubble Tea: dashboard híbrido + paleta ⌘k + pickers interactivos | `core`, `config`, `render` |
| `internal/render` | Estilos compartidos (Lipgloss): tablas, colores de estado, spinners | — |

Cada unidad tiene un propósito claro y se entiende sin leer las internas de las demás.

---

## 6. Configuración (`steer.toml`)

Toda la información específica de un entorno (cuentas, roles, credenciales,
convenciones de naming) vive en config, no en código. Se busca en el repo actual o en
`~/.config/steer/steer.toml`.

El esquema evolucionó del original `[providers.aws.environments.*]` al modelo de
**contextos** (hito multi-context): cada contexto = cloud + credencial + cluster +
templates + writable, y las capacidades opcionales anidan su bloque (patrón que fijó el
hito images). Valores de ejemplo:

```toml
default_context = "staging"

[contexts.staging]
cloud            = "aws"
profile          = "staging"
role_arn         = "arn:aws:iam::000000000000:role/your-deployer-role"
cluster          = "myteam-staging"
service_template = "myteam-staging-{name}"
writable         = true

  # capacidad opcional: habilita la sección IMAGES y el tag-picker del deploy
  [contexts.staging.images]
  repo_template = "myteam-{name}"

[contexts.prod]
cloud            = "aws"
profile          = "prod"
cluster          = "myteam-prod"
service_template = "myteam-prod-{name}"
writable         = false   # solo lectura: bloquea comandos mutantes en prod
```

El `steer.toml` con valores reales **no** se commitea (está en `.gitignore`); el repo
incluye `steer.example.toml`.

---

## 7. Experiencia de usuario

### CLI (scriptable, CI-friendly)
```
steer --context stg service deploy -s my-service -t v1.2.3 -y    # cero prompts
steer image tags -r my-service
steer service status
```
El deploy **valida el tag contra el registry** antes de tocar el cloud (bloquea tags o
repos inexistentes; degrada con warning si el registry no está disponible) y `-w` sigue
el rollout en vivo, cortando con sugerencia de rollback si detecta un atasco de pull.

### Deploy interactivo (cuando faltan argumentos)
Como se construyó (el multi-select por servicio del diseño original quedó en YAGNI —
nadie lo ha pedido; un servicio por deploy):
1. **Selección de servicio** — picker poblado en vivo desde el cluster.
2. **Tag** — en el TUI, picker filtrable con los tags reales del registry (antigüedad y
   marcador del desplegado); en CLI, prompt de texto validado igual que el modo con flags.
3. **Confirmación + progreso en vivo** — preview de qué va a pasar; eventos del rollout
   en tiempo real.

El modo no-interactivo (`-s ... -t ... -y`) queda intacto para CI/scripts.

### TUI (`steer tui`) — dashboard estilo lazydocker
Como se construyó (rediseño 2026-06-28 + iteraciones):
- **Sidebar por secciones** (SERVICES · IMAGES · DATABASES) colapsables, con filtro `/`
  en vivo y scroll con indicadores; **todo clickeable** (mouse de primera clase) además
  del teclado completo.
- **Panel derecho** con pestañas Details/Events/Logs para servicios y tabla de TAGS para
  repos; acciones como botones + **formulario inline** (no modal): deploy con tag-picker
  y validación, scale, rollback.
- **Switcher de contexto** en vivo (`c` o click en la barra superior).
- **Paleta de comandos (⌘k)** — pendiente (hito 06b); la costura de overlays ya la admite.

---

## 8. Distribución OSS

- **GoReleaser** → binarios multiplataforma en GitHub Releases.
- **Homebrew** (`brew install`), **`go install`**, y descarga directa de binario.
- **CI** en GitHub Actions: build + test en cada PR; release automático por tag.

---

## 9. Estrategia de implementación (incremental)

Construcción por capacidades, priorizando valor:

1. **Core + config + sesión AWS** — la base (interfaces, carga de `steer.toml`).
2. **service** — deploy interactivo + status. Caso estrella: valida CLI + TUI + picker juntos.
3. **Resto de capacidades**, una a una: registry, db, queue, storage, host, env, assets.
4. **Dashboard TUI híbrido** encima de las capacidades ya construidas.

---

## 10. Seguridad

- **Guard de solo-lectura por contexto:** `writable=false` bloquea TODA operación
  mutante en ese contexto (deploy/scale/rollback), en CLI y TUI por igual. Los comandos
  mutantes en contextos writable piden confirmación salvo `-y`. Además el deploy valida
  el tag contra el registry antes de tocar el cloud, y el watch detecta rollouts
  atascados sugiriendo rollback (diseño 2026-07-05-deploy-seguro).
- La config con datos sensibles **no** se commitea; el repo incluye `steer.example.toml`.
