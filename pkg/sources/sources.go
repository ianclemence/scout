// Package sources defines the OpportunitySource interface: capabilities,
// not brands. Adapters translate normalized operations into source-specific
// mechanisms (MCP, API, local). Qualification logic never lives here.
package sources

import (
	"context"

	"github.com/ianclemence/scout/pkg/domain"
)

// Capability names what a source can do. Sources implement subsets.
type Capability string

const (
	CapSearch      Capability = "search"
	CapReadListing Capability = "read_listing"
	CapStatus      Capability = "status"
	CapResearch    Capability = "research_client"
	CapDraft       Capability = "draft_application"
	CapSubmit      Capability = "submit_application"
	CapMessage     Capability = "messaging"
	CapContracts   Capability = "contract_access"
)

type SearchFilter struct {
	Query        string
	Skills       []string
	Location     string
	Remote       string
	MinBudget    float64
	ContractType string
	Limit        int
}

// Health describes source connectivity.
type Health struct {
	State  string `json:"state"` // connected, unauthenticated, unavailable, rate_limited, misconfigured
	Detail string `json:"detail"`
}

// OpportunitySource is the adapter contract. Not every source implements
// every method; Capabilities() declares the truthful subset.
type OpportunitySource interface {
	ID() string
	Name() string
	Capabilities() []Capability
	Has(c Capability) bool
	Search(ctx context.Context, f SearchFilter) ([]domain.Opportunity, error)
	Get(ctx context.Context, sourceID string) (*domain.Opportunity, error)
	Status(ctx context.Context, sourceID string) (string, error)
	Health(ctx context.Context) Health
	Close() error
}

func Has(caps []Capability, c Capability) bool {
	for _, k := range caps {
		if k == c {
			return true
		}
	}
	return false
}

// Normalize fills Scout-managed fields on adapter output.
func Normalize(source string, o *domain.Opportunity) *domain.Opportunity {
	o.Source = source
	if o.BudgetType == "" {
		o.BudgetType = "unknown"
	}
	if o.RemoteStatus == "" {
		o.RemoteStatus = "unknown"
	}
	if o.LiveStatus == "" {
		o.LiveStatus = "unknown"
	}
	return o
}
