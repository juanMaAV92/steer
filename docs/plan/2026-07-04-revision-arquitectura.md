# Revisión de arquitectura — 2026-07-04

Síntesis de una revisión a fondo (3 revisiones paralelas: arquitectura/costuras, TUI,
CLI/config/infra) del estado del proyecto en `main` tras los PRs #5–#8.

## Veredicto

Fronteras bien diseñadas: grafo de dependencias limpio y acíclico (`render` y `core` hojas
puras), costuras de test honestas (fakes contra `core`, no contra AWS), buena disciplina de
tests (~1.7k LOC de test / ~2.7k de producción, tests anclados al render). **Pero las
costuras se diseñaron para UNA capacidad (`Deployer`)** y el roadmap pide 8: hay que
generalizarlas ahora, con una sola capacidad que migrar, no tras copiar el patrón 5 veces.

## Crítico (corregido el mismo día)

- `main` tenía los tests rotos: los PRs #7 y #8 añadieron cada uno su `stripANSI` a
  `app_test.go`; git los auto-mergeó sin conflicto textual y el paquete no compilaba.
  Hotfix directo a main (`b336f37`). Lección: correr tests con `-count=1` tras merges y
  poner lint en CI.

## Hallazgos principales

1. **Fábrica por capacidad no escala** — `DeployerFactory` carga la sesión AWS en cada
   llamada; con N capacidades × M switches = N×M cargas (y prompts SSO). → `Provider`
   bundle: una fábrica por contexto, `aws.Config` cacheada, capacidades memoizadas.
2. **`context.Background()` en fábrica, comandos TUI y watch CLI** — un SSO expirado o una
   llamada colgada no se puede cancelar. → enhebrar `ctx`.
3. **ECS-ismos en la interface "agnóstica"** — `cluster` como parámetro en cada método;
   `Rollout` como strings mágicos comparados en 3+ sitios; `rolloutColored` duplicado
   CLI/TUI. → cluster al constructor; `RolloutState` tipado; helper compartido en render.
4. **`Context` plano sin sitio para crecer** (repo_template, log groups, db…) y sin
   `Config.Validate()` global (esquema legacy decodifica a 0 contextos en silencio). →
   validación global ya; config anidada por capacidad en el hito registry.
5. **TUI sin costura multi-capacidad** — Model cableado a un `dep`; sin abstracción de
   sección ni registro de comandos para la paleta ⌘k; tercer overlay a mano; hit-testing
   triplicado; `detailsButtonRowY` mágica. → plan de TUI previo a la paleta.
6. **CLI/infra**: `--env` y `--context` comparten variable sin aviso; `scale --count`
   default 1 (peligroso); rama muerta de deploy en `runActionCmd`; estado de deploy en 4
   flags dispersos; CI sin golangci-lint/gofmt; falta `STEER_CONTEXT` y stanza `brews`
   para el hito de distribución.

## Plan de acción

| # | Qué | Estado |
|---|-----|--------|
| 0 | Hotfix stripANSI | ✅ `b336f37` |
| 1 | Ola de higiene (rollout tipado, rama muerta, deployState, flags CLI, lint CI) | ✅ mergeado (T1–T5, `2e8ceb0..f73b06e`) |
| 2 | Costuras (cluster al constructor, Provider bundle + ctx, Validate, IsError) + cancelación cableada en la raíz | ✅ mergeado (T6–T10 + fix final, `5368a59..8827f4d`) |
| 3 | Config anidada por capacidad | junto al hito registry |
| 4 | Registry (spec revisado: `Provider.Registry()` en vez de `RegistryFactory`) | siguiente — revisar spec primero |
| 5 | TUI: overlay + sidebar por secciones + hit-testing unificado | ✅ mergeado (plan `2026-07-04-refactor-tui-overlays.md`, 7/7 + barrida, `b491799..09a1ab1`). El registro de comandos enumerable queda para el hito de la paleta ⌘k (nota: capturar foco previo al cerrar overlays). |

Minors diferidos del ledger (cosméticos, triados en la revisión final): campo `ctx config.Context`
en `aws/provider.go` (renombrar a `cfgCtx`), `HasPrefix("providers")` sin límite de palabra,
assert duplicado en `rollout_test.go`, mensaje estático del guard `-y`.
