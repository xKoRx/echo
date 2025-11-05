package handshake

// Equivalent retorna true si ambas evaluaciones representan el mismo estado
// lógico (estatus global, issues y entradas por símbolo). Se ignoran campos
// no idempotentes como EvaluationID o timestamps de evaluación.
func Equivalent(current, previous *Evaluation) bool {
	if current == nil || previous == nil {
		return current == previous
	}

	if current.Status != previous.Status {
		return false
	}
	if !issuesEqual(current.Errors, previous.Errors) {
		return false
	}
	if !issuesEqual(current.Warnings, previous.Warnings) {
		return false
	}
	if !capabilitiesEqual(current.Capabilities, previous.Capabilities) {
		return false
	}
	if !stringSliceEqual(current.RequiredFeatures, previous.RequiredFeatures) {
		return false
	}
	if !stringSliceEqual(current.OptionalFeatures, previous.OptionalFeatures) {
		return false
	}

	return entriesEquivalent(current.Entries, previous.Entries)
}

func entriesEquivalent(current, previous []Entry) bool {
	if len(current) != len(previous) {
		return false
	}

	prevByCanonical := make(map[string]Entry, len(previous))
	for _, entry := range previous {
		prevByCanonical[entry.CanonicalSymbol] = entry
	}

	for _, entry := range current {
		prev, ok := prevByCanonical[entry.CanonicalSymbol]
		if !ok {
			return false
		}
		if entry.Status != prev.Status {
			return false
		}
		if entry.BrokerSymbol != prev.BrokerSymbol {
			return false
		}
		if !issuesEqual(entry.Errors, prev.Errors) {
			return false
		}
		if !issuesEqual(entry.Warnings, prev.Warnings) {
			return false
		}
	}

	return true
}

func issuesEqual(current, previous []Issue) bool {
	if len(current) != len(previous) {
		return false
	}
	for i := range current {
		c := current[i]
		p := previous[i]
		if c.Code != p.Code || c.Message != p.Message {
			return false
		}
		if !metadataEqual(c.Metadata, p.Metadata) {
			return false
		}
	}
	return true
}

func metadataEqual(current, previous map[string]string) bool {
	if len(current) != len(previous) {
		return false
	}
	for key, value := range current {
		if prev, ok := previous[key]; !ok || prev != value {
			return false
		}
	}
	return true
}

func capabilitiesEqual(current, previous CapabilitySet) bool {
	if !stringSliceEqual(current.Features, previous.Features) {
		return false
	}
	if !stringSliceEqual(current.Metrics, previous.Metrics) {
		return false
	}
	return true
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
