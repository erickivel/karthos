package metrics

type Dialect interface {
	Translate(metricName string) (string, bool)
}

type DialectMap map[string]string

func (d DialectMap) Translate(metricName string) (string, bool) {
	translated, ok := d[metricName]
	return translated, ok
}

func DialectFromMap(m map[string]string) Dialect {
	return DialectMap(m)
}

type DialectRegistry struct {
	dialects []Dialect
	bySource map[string]Dialect
}

func NewDialectRegistry(dialects ...Dialect) *DialectRegistry {
	return &DialectRegistry{dialects: dialects}
}

// NewSourceDialectRegistry binds each compiled source ID to its source-owned
// wire-name dialect. Callers obtain these pairs from pkg/sources; metrics does
// not own a parallel source registry.
func NewSourceDialectRegistry(sourceDialects map[string]Dialect) *DialectRegistry {
	out := &DialectRegistry{bySource: make(map[string]Dialect, len(sourceDialects))}
	for sourceID, dialect := range sourceDialects {
		if dialect == nil {
			continue
		}
		out.dialects = append(out.dialects, dialect)
		out.bySource[sourceID] = dialect
	}
	return out
}

// TranslateForSource translates with one explicitly stamped source. The
// source-built registry never performs ordered dialect matching, preventing a
// raw metric from being claimed by an unrelated module.
func (r *DialectRegistry) TranslateForSource(sourceID, metricName string) string {
	if r == nil {
		return metricName
	}
	if r.bySource != nil {
		dialect := r.bySource[sourceID]
		if dialect == nil {
			return metricName
		}
		if translated, ok := dialect.Translate(metricName); ok {
			return translated
		}
		return metricName
	}
	return r.Translate(metricName)
}

func (r *DialectRegistry) Translate(metricName string) string {
	if r == nil {
		return metricName
	}
	for _, dialect := range r.dialects {
		if translated, ok := dialect.Translate(metricName); ok {
			return translated
		}
	}
	return metricName
}
