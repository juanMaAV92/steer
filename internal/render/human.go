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

// CPULabel formatea mili-vCPU en unidades humanas ("0.25 vCPU", "1 vCPU").
func CPULabel(milli int) string {
	return strconv.FormatFloat(float64(milli)/1000, 'f', -1, 64) + " vCPU"
}

// MemLabel formatea MiB en la unidad natural ("512 MB", "1.5 GB", "2 GB").
func MemLabel(mib int) string {
	if mib < 1024 {
		return strconv.Itoa(mib) + " MB"
	}
	return strconv.FormatFloat(float64(mib)/1024, 'f', -1, 64) + " GB"
}
