package sources

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ianclemence/scout/pkg/domain"
	"github.com/ianclemence/scout/pkg/mcpclient"
)

// Registry holds configured sources. The agent reasons about capabilities;
// adapters handle source mechanics.
type Registry struct {
	mu      sync.RWMutex
	sources map[string]OpportunitySource
}

func NewRegistry() *Registry { return &Registry{sources: map[string]OpportunitySource{}} }

func (r *Registry) Add(s OpportunitySource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sources[s.ID()] = s
}

func (r *Registry) Get(id string) (OpportunitySource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sources[id]
	return s, ok
}

func (r *Registry) All() []OpportunitySource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []OpportunitySource
	for _, s := range r.sources {
		out = append(out, s)
	}
	return out
}

// WithCapability returns sources advertising a capability.
func (r *Registry) WithCapability(c Capability) []OpportunitySource {
	var out []OpportunitySource
	for _, s := range r.All() {
		if s.Has(c) {
			out = append(out, s)
		}
	}
	return out
}

// SearchAll queries every source with search capability in sequence,
// isolating per-source failures (one dead source never fails discovery).
func (r *Registry) SearchAll(ctx context.Context, f SearchFilter) ([]domain.Opportunity, map[string]error) {
	var out []domain.Opportunity
	errs := map[string]error{}
	for _, s := range r.WithCapability(CapSearch) {
		res, err := s.Search(ctx, f)
		if err != nil {
			errs[s.ID()] = err
			continue
		}
		out = append(out, res...)
	}
	return out, errs
}

// ConnectorFor builds an MCP connector from stored source config.
func ConnectorFor(kind, endpoint, command, token string) (*mcpclient.Connector, error) {
	if kind == "mcp-stdio" {
		if strings.TrimSpace(command) == "" {
			return nil, fmt.Errorf("stdio source has no command")
		}
		return &mcpclient.Connector{Command: strings.Fields(command)}, nil
	}
	if endpoint == "" {
		return nil, fmt.Errorf("remote source has no endpoint")
	}
	return &mcpclient.Connector{Endpoint: endpoint, Token: token}, nil
}
