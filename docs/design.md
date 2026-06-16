# Steer — Diseño V1

**Estado:** Aprobado para escribir plan de implementación

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

| Comando | Capacidad | Subcomandos (funciones) |
|---|---|---|
| `steer service` | Servicios / contenedores | `ls` · `status` · `watch` · `deploy` · `redeploy` · `rollback` · `scale` · `promote` |
| `steer registry` | Registro de imágenes | `ls` · `images` · `build` · `prune` |
| `steer db` | Base de datos | `status` · `slow-queries` · `tunnel` |
| `steer storage` | Almacenamiento de objetos | `ls` |
| `steer queue` | Colas | `ls` · `watch` |
| `steer host` | Hosts / instancias | `ls` · `connect` |
| `steer env` | Entornos (encender/apagar) | `ls` · `up` · `down` |
| `steer assets` | Estático / CDN | `deploy` · `url` · `info` · `invalidate` |

Global:
- `steer tui` — abre el dashboard interactivo.
- `steer config init|validate` — crea/valida `steer.toml`.
- `-e/--env` — selecciona el entorno (resuelve perfil/sesión AWS).
- Alias de camino caliente: `steer deploy` → `steer service deploy`.

---

## 5. Componentes (paquetes Go)

| Paquete | Responsabilidad | Depende de |
|---|---|---|
| `internal/config` | Cargar/validar `steer.toml`; resolver entorno → sesión AWS | — |
| `internal/core` | Interfaces de capacidad (`Deployer`, `Registry`, `ObjectStore`, `LogSource`) | — |
| `internal/providers/aws` | Implementación AWS (un subpaquete por capacidad: service, registry, db, storage, queue, host, env, assets) | `core`, `config` |
| `cmd/` | Comandos Cobra (service, registry, db…) | `core`, `config`, `render` |
| `internal/tui` | App Bubble Tea: dashboard híbrido + paleta ⌘k + pickers interactivos | `core`, `config`, `render` |
| `internal/render` | Estilos compartidos (Lipgloss): tablas, colores de estado, spinners | — |

Cada unidad tiene un propósito claro y se entiende sin leer las internas de las demás.

---

## 6. Configuración (`steer.toml`)

Toda la información específica de un entorno (cuentas, roles, mapeo entorno→perfil,
convenciones de naming) vive en config, no en código. Se busca en el repo actual o en
`~/.config/steer/steer.toml`. Valores de ejemplo:

```toml
[providers.aws.environments.staging]
profile    = "staging"
account_id = "000000000000"
role_arn   = "arn:aws:iam::000000000000:role/your-deployer-role"
writable   = true

[providers.aws.environments.prod]
profile  = "prod"
writable = false          # solo lectura: bloquea comandos mutantes en prod

[providers.aws.naming]
# plantillas que resuelven un nombre corto a un recurso AWS real
cluster_template = "{env}-cluster"
service_template = "{name}"
```

El `steer.toml` con valores reales **no** se commitea (está en `.gitignore`); el repo
incluye `steer.example.toml`.

---

## 7. Experiencia de usuario

### CLI (scriptable, CI-friendly)
```
steer -e stg service deploy -s my-service -t v1.2.3 -y    # cero prompts
steer registry build -s my-service
steer service status
```

### Deploy interactivo (cuando faltan argumentos)
Flujo de 3 pasos:
1. **Selección de servicios** — picker con fuzzy-filter + multi-select, poblado en vivo
   desde el cluster; muestra estado running/desired por servicio.
2. **Selección de tag** — lista los tags reales del registry con antigüedad; filtrable.
   El tag se elige **por servicio**.
3. **Confirmación + progreso en vivo** — resumen de lo que se va a desplegar y progreso
   por servicio.

El modo no-interactivo (`-s ... -t ... -y`) queda intacto para CI/scripts.

### TUI (`steer tui`) — home híbrido
- **Dashboard** como pantalla principal: salud de todo (servicios, db, colas) de un
  vistazo con colores de estado; `enter` para drill-down.
- **Paleta de comandos (⌘k)** siempre disponible para saltar directo a cualquier acción.
  Todo accesible por teclado.

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

- **Guard de producción global:** cualquier comando con `-e prod` pide confirmación.
  Operaciones prohibidas en prod (p.ej. `env up/down`) añaden su propio guard, gobernado
  por `writable=false` en la config del entorno.
- La config con datos sensibles **no** se commitea; el repo incluye `steer.example.toml`.
