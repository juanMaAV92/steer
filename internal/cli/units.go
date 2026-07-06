package cli

import (
	"fmt"
	"strconv"
	"strings"
)

// parseCPU acepta vCPU decimal ("0.5", "1") o mili ("500m") y devuelve mili-vCPU.
func parseCPU(s string) (int, error) {
	s = strings.TrimSpace(s)
	if m, ok := strings.CutSuffix(s, "m"); ok {
		n, err := strconv.Atoi(m)
		if err != nil {
			return 0, fmt.Errorf("invalid cpu %q (use 0.5, 1 or 500m)", s)
		}
		return n, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cpu %q (use 0.5, 1 or 500m)", s)
	}
	return int(f * 1000), nil
}

// parseMemory acepta MiB ("2048") o humano ("2GB", "512MB", case-insensitive)
// y devuelve MiB.
func parseMemory(s string) (int, error) {
	t := strings.ToUpper(strings.TrimSpace(s))
	mult := 1.0
	switch {
	case strings.HasSuffix(t, "GB"):
		t, mult = strings.TrimSuffix(t, "GB"), 1024
	case strings.HasSuffix(t, "MB"):
		t = strings.TrimSuffix(t, "MB")
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory %q (use 2048, 2GB or 512MB)", s)
	}
	return int(f * mult), nil
}
