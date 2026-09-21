package sources

import (
	"context"
	"strings"

	"github.com/ianclemence/scout/pkg/domain"
)

// ManualSource exposes locally stored opportunities as a source adapter.
// SearchFunc is injected by the runtime (which owns the database).
type ManualSource struct {
	IDValue    string
	SearchFunc func(ctx context.Context, f SearchFilter) ([]domain.Opportunity, error)
	GetFunc    func(ctx context.Context, id string) (*domain.Opportunity, error)
}

func (m *ManualSource) ID() string   { return m.IDValue }
func (m *ManualSource) Name() string { return "Local Scout store" }
func (m *ManualSource) Capabilities() []Capability {
	return []Capability{CapSearch, CapReadListing, CapStatus}
}
func (m *ManualSource) Has(c Capability) bool { return Has(m.Capabilities(), c) }

func (m *ManualSource) Search(ctx context.Context, f SearchFilter) ([]domain.Opportunity, error) {
	out, err := m.SearchFunc(ctx, f)
	if err != nil {
		return nil, err
	}
	var filtered []domain.Opportunity
	for _, o := range out {
		if f.Query != "" && !strings.Contains(strings.ToLower(o.Title+" "+o.Description), strings.ToLower(f.Query)) {
			continue
		}
		if f.MinBudget > 0 && o.BudgetMax > 0 && o.BudgetMax < f.MinBudget {
			continue
		}
		if f.Remote != "" && o.RemoteStatus != "" && o.RemoteStatus != "unknown" && !strings.EqualFold(o.RemoteStatus, f.Remote) {
			continue
		}
		filtered = append(filtered, o)
		if f.Limit > 0 && len(filtered) >= f.Limit {
			break
		}
	}
	return filtered, nil
}

func (m *ManualSource) Get(ctx context.Context, sourceID string) (*domain.Opportunity, error) {
	return m.GetFunc(ctx, sourceID)
}

func (m *ManualSource) Status(ctx context.Context, sourceID string) (string, error) {
	o, err := m.Get(ctx, sourceID)
	if err != nil {
		return "unknown", err
	}
	if o.LiveStatus != "" {
		return o.LiveStatus, nil
	}
	return "unknown", nil
}

func (m *ManualSource) Health(ctx context.Context) Health {
	return Health{State: "connected", Detail: "local database"}
}

func (m *ManualSource) Close() error { return nil }
