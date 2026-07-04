package render

// Rollout colorea un estado de rollout: COMPLETED verde, FAILED rojo, resto cian.
// Único punto donde se mapea estado→color (CLI y TUI lo comparten).
func Rollout(s string) string {
	switch s {
	case "COMPLETED":
		return Success(s)
	case "FAILED":
		return Danger(s)
	default:
		return Accent(s)
	}
}
