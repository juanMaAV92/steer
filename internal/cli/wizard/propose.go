package wizard

import "strings"

// ProposeName deduce el nombre del contexto desde el cluster ("-cluster" fuera).
func ProposeName(cluster string) string {
	return strings.TrimSuffix(cluster, "-cluster")
}

// ProposeServiceTemplate propone "<nombre>-{name}" (patrón observado: los servicios
// llevan el prefijo del ambiente).
func ProposeServiceTemplate(cluster string) string {
	return ProposeName(cluster) + "-{name}"
}

// ProposeWritable default false si el nombre huele a producción.
func ProposeWritable(name string) bool {
	l := strings.ToLower(name)
	for _, marker := range []string{"prod", "prd"} {
		if strings.Contains(l, marker) {
			return false
		}
	}
	return true
}

// ProposeRepoTemplate quita el último segmento del prefijo del service_template
// (los registries suelen ser compartidos entre ambientes: nao-v2-dev-{name} →
// nao-v2-{name}); sin segmentos suficientes devuelve "{name}".
func ProposeRepoTemplate(serviceTemplate string) string {
	prefix := strings.TrimSuffix(serviceTemplate, "{name}")
	prefix = strings.TrimSuffix(prefix, "-")
	if prefix == "" {
		return "{name}"
	}
	parts := strings.Split(prefix, "-")
	if len(parts) <= 1 {
		return "{name}"
	}
	return strings.Join(parts[:len(parts)-1], "-") + "-{name}"
}
