package sources

import (
	"context"
	"fmt"
	"strings"

	"github.com/ianclemence/scout/internal/domain"
	imat "github.com/ianclemence/scout/internal/match"
)

// FakeSource is a deterministic test adapter: scripted listings, failures,
// duplicates, and capability subsets. Never touches the network.
type FakeSource struct {
	IDValue     string
	NameValue   string
	Caps        []Capability
	Listings    []domain.Opportunity
	FailSearch  bool
	FailGet     bool
	SearchCalls int
}

func NewFake(id, name string, caps []Capability, listings []domain.Opportunity) *FakeSource {
	return &FakeSource{IDValue: id, NameValue: name, Caps: caps, Listings: listings}
}

func (f *FakeSource) ID() string                 { return f.IDValue }
func (f *FakeSource) Name() string               { return f.NameValue }
func (f *FakeSource) Capabilities() []Capability { return f.Caps }
func (f *FakeSource) Has(c Capability) bool      { return Has(f.Caps, c) }

func (f *FakeSource) Search(ctx context.Context, fl SearchFilter) ([]domain.Opportunity, error) {
	f.SearchCalls++
	if f.FailSearch {
		return nil, fmt.Errorf("fake source %s: search unavailable", f.IDValue)
	}
	lim := fl.Limit
	if lim <= 0 || lim > len(f.Listings) {
		lim = len(f.Listings)
	}
	var out []domain.Opportunity
	for _, l := range f.Listings {
		if fl.Query != "" && !strings.Contains(strings.ToLower(l.Title+" "+l.Description), strings.ToLower(fl.Query)) {
			continue
		}
		c := l
		c.Source = f.IDValue
		if c.Fingerprint == "" {
			c.Fingerprint = imat.Fingerprint(f.IDValue, c.SourceOppID, c.Title, c.Description)
		}
		out = append(out, c)
		if len(out) >= lim {
			break
		}
	}
	return out, nil
}

func (f *FakeSource) Get(ctx context.Context, sourceID string) (*domain.Opportunity, error) {
	if f.FailGet {
		return nil, fmt.Errorf("fake source %s: get unavailable", f.IDValue)
	}
	for _, l := range f.Listings {
		if l.SourceOppID == sourceID {
			c := l
			c.Source = f.IDValue
			return Normalize(f.IDValue, &c), nil
		}
	}
	return nil, fmt.Errorf("fake source %s: listing %s not found", f.IDValue, sourceID)
}

func (f *FakeSource) Status(ctx context.Context, sourceID string) (string, error) {
	o, err := f.Get(ctx, sourceID)
	if err != nil {
		return "unknown", err
	}
	if o.LiveStatus != "" && o.LiveStatus != "unknown" {
		return o.LiveStatus, nil
	}
	return "active", nil
}

func (f *FakeSource) Health(ctx context.Context) Health {
	if f.FailSearch {
		return Health{State: "unavailable", Detail: "scripted failure"}
	}
	return Health{State: "connected", Detail: "fake source"}
}

func (f *FakeSource) Close() error { return nil }
