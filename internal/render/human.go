package render

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Age formatea la antigüedad de t respecto a now ("2h ago", "3d ago").
func Age(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	}
}

// Size formatea bytes como MB/GB legibles (registro de imágenes: MB es la unidad natural).
func Size(b int64) string {
	const mb = 1024 * 1024
	if b >= 1024*mb {
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1024*mb))
	}
	return strconv.FormatInt(b/mb, 10) + " MB"
}

// ShortDigest acorta un digest sha256 a 12 caracteres para mostrar.
func ShortDigest(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		return d[:12]
	}
	return d
}
