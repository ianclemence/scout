package sources

import "strings"

// NewAdapterFor builds the best OpportunitySource for a configured connector.
// Most MCP servers use the generic adapter; providers whose MCP has a
// non-generic tool contract get a dedicated "dialect" adapter. Upwork is the
// first such dialect. New providers (LinkedIn, JobsDB, …) add a matcher here
// plus their adapter, without touching the generic path.
func NewAdapterFor(id, name, endpoint string, conn mcpCaller) OpportunitySource {
	if isUpwork(endpoint, name) {
		return NewUpworkAdapter(id, name, conn)
	}
	return NewMCPAdapter(id, name, conn)
}

func isUpwork(endpoint, name string) bool {
	return strings.Contains(strings.ToLower(endpoint+" "+name), "upwork")
}
