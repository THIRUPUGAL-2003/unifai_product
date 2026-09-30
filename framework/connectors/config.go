package connectors

// ConnectorNames lists workspace data connector identifiers.
var ConnectorNames = []string{"datadog", "kafka", "bigquery", "pubsub", "newrelic"}

// IsKnown reports whether name is a supported connector identifier.
func IsKnown(name string) bool {
	for _, candidate := range ConnectorNames {
		if candidate == name {
			return true
		}
	}
	return false
}

// Settings is the runtime view of a saved connector row.
type Settings struct {
	Name    string
	Enabled bool
	Config  map[string]string
}
