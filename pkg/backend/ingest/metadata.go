package ingest

// buildEntityMetadata returns a fresh map seeded with the caller-supplied
// labels, then overlays the well-known cluster/namespace/name keys when set.
// Those three keys always take precedence over any same-named label.
func buildEntityMetadata(cluster, namespace, name string, labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels)+3)
	for k, v := range labels {
		out[k] = v
	}
	if cluster != "" {
		out["cluster"] = cluster
	}
	if namespace != "" {
		out["namespace"] = namespace
	}
	if name != "" {
		out["name"] = name
	}
	return out
}
