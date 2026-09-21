// Package domain defines marketplace-independent core types.
// Upwork-specific concepts live in pkg/upwork, never here.
package domain

import "time"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ProfessionalProfile struct {
	ID                 string       `json:"id"`
	DisplayName        string       `json:"display_name"`
	Title              string       `json:"title,omitempty"`
	Summary            string       `json:"summary,omitempty"`
	Skills             []string     `json:"skills"`
	Technologies       []string     `json:"technologies"`
	Experience         []Experience `json:"experience"`
	Education          []Education  `json:"education"`
	Languages          []string     `json:"languages"`
	PortfolioURLs      []string     `json:"portfolio_urls"`
	GitHubURL          string       `json:"github_url,omitempty"`
	WebsiteURL         string       `json:"website_url,omitempty"`
	LinkedInURL        string       `json:"linkedin_url,omitempty"`
	MinHourlyRate      float64      `json:"min_hourly_rate"`
	MinProjectBudget   float64      `json:"min_project_budget"`
	MaxConnectsPerApp  int          `json:"max_connects_per_app"`
	MaxAppsPerDay      int          `json:"max_apps_per_day"`
	PreferredJobTypes  []string     `json:"preferred_job_types"`
	ExcludedWork       []string     `json:"excluded_work"`
	PreferredCountries []string     `json:"preferred_countries"`
	Availability       string       `json:"availability,omitempty"`
	ProposalStyle      string       `json:"proposal_style,omitempty"`
	UpdatedAt          time.Time    `json:"updated_at"`
}

type Experience struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Company      string   `json:"company,omitempty"`
	Description  string   `json:"description,omitempty"`
	Technologies []string `json:"technologies,omitempty"`
	URL          string   `json:"url,omitempty"`
}

type Education struct {
	School string `json:"school"`
	Degree string `json:"degree,omitempty"`
	Year   string `json:"year,omitempty"`
}

type PortfolioItem struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	URL          string   `json:"url,omitempty"`
	Technologies []string `json:"technologies,omitempty"`
}

type Evidence struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"` // cv_section, project, portfolio, employment, credential
	Reference string    `json:"reference"`
	Content   string    `json:"content"`
	Source    string    `json:"source,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Preference struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Reason    string    `json:"reason,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Client struct {
	ID             string  `json:"id"`
	Source         string  `json:"source"`
	SourceClientID string  `json:"source_client_id,omitempty"`
	DisplayName    string  `json:"display_name"`
	Rating         float64 `json:"rating"`
	TotalHires     int     `json:"total_hires"`
	TotalSpend     float64 `json:"total_spend"`
	Country        string  `json:"country,omitempty"`
	HistoryNote    string  `json:"history_note,omitempty"`
}

type Opportunity struct {
	ID             string    `json:"id"`
	Source         string    `json:"source"`
	SourceOppID    string    `json:"source_opp_id"`
	CanonicalURL   string    `json:"canonical_url,omitempty"`
	SourceURL      string    `json:"source_url,omitempty"`
	Title          string    `json:"title"`
	Company        string    `json:"company,omitempty"`
	Description    string    `json:"description"`
	EmploymentType string    `json:"employment_type,omitempty"`
	EngagementType string    `json:"engagement_type,omitempty"`
	Location       string    `json:"location,omitempty"`
	RemoteStatus   string    `json:"remote_status,omitempty"`
	Skills         []string  `json:"skills"`
	Technologies   []string  `json:"technologies,omitempty"`
	Requirements   []string  `json:"requirements,omitempty"`
	Category       string    `json:"category,omitempty"`
	BudgetMin      float64   `json:"budget_min"`
	BudgetMax      float64   `json:"budget_max"`
	BudgetType     string    `json:"budget_type,omitempty"`
	Currency       string    `json:"currency,omitempty"`
	HourlyRateMin  float64   `json:"hourly_rate_min"`
	HourlyRateMax  float64   `json:"hourly_rate_max"`
	ConnectsCost   int       `json:"connects_cost"`
	PostedAt       time.Time `json:"posted_at"`
	Deadline       string    `json:"deadline,omitempty"`
	Fingerprint    string    `json:"fingerprint"`
	Client         *Client   `json:"client,omitempty"`
	RawSnapshot    string    `json:"raw_snapshot,omitempty"`
	Provenance     string    `json:"provenance,omitempty"`
	Status         string    `json:"status"`
	LiveStatus     string    `json:"live_status,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type MatchDimension struct {
	Name   string `json:"name"`
	Rating string `json:"rating"` // strong, good, moderate, weak, unacceptable
	Detail string `json:"detail"`
}

type MatchEvaluation struct {
	ID             string           `json:"id"`
	OpportunityID  string           `json:"opportunity_id"`
	Dimensions     []MatchDimension `json:"dimensions"`
	EvidenceIDs    []string         `json:"evidence_ids"`
	Risks          []string         `json:"risks"`
	Recommendation string           `json:"recommendation"` // apply, review, ignore
	Reason         string           `json:"reason"`
	CreatedAt      time.Time        `json:"created_at"`
}

type Proposal struct {
	ID            string    `json:"id"`
	OpportunityID string    `json:"opportunity_id"`
	ApplicationID string    `json:"application_id,omitempty"`
	CoverLetter   string    `json:"cover_letter"`
	Rate          float64   `json:"rate"`
	RateType      string    `json:"rate_type,omitempty"`
	DurationEst   string    `json:"duration_est,omitempty"`
	EvidenceIDs   []string  `json:"evidence_ids"`
	Questions     []string  `json:"questions,omitempty"`
	Status        string    `json:"status"` // draft, pending_approval, approved, rejected, executed, failed
	CreatedAt     time.Time `json:"created_at"`
}

type Application struct {
	ID            string    `json:"id"`
	OpportunityID string    `json:"opportunity_id"`
	Source        string    `json:"source"`
	Stage         string    `json:"stage"` // submitted, viewed, replied, interview, offer, contract, won, rejected, lost, withdrawn, expired
	CostConnects  int       `json:"cost_connects"`
	SubmittedAt   time.Time `json:"submitted_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Message struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	ThreadID  string    `json:"thread_id,omitempty"`
	FromParty string    `json:"from_party,omitempty"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type AgentRun struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"` // discovery, analysis, proposal, refresh
	Status    string    `json:"status"`
	Summary   string    `json:"summary,omitempty"`
	DryRun    bool      `json:"dry_run"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

type PendingAction struct {
	ID         string    `json:"id"`
	Source     string    `json:"source"`
	ActionType string    `json:"action_type"` // submit_proposal, send_message, accept_offer, fund_milestone, other
	Target     string    `json:"target"`
	Payload    string    `json:"payload"`
	RiskLevel  string    `json:"risk_level"` // low, medium, high
	Status     string    `json:"status"`     // draft, pending_approval, approved, rejected, executed, failed, cancelled
	CreatedAt  time.Time `json:"created_at"`
	DecidedAt  time.Time `json:"decided_at,omitempty"`
}

type ActivityEvent struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Capability names a WorkSource advertises.
type Capability string

const (
	CapSearchOpportunities Capability = "search_opportunities"
	CapReadOpportunity     Capability = "read_opportunity"
	CapSearchClients       Capability = "search_clients"
	CapReadMessages        Capability = "read_messages"
	CapDraftApplication    Capability = "draft_application"
	CapSubmitApplication   Capability = "submit_application"
	CapReadContract        Capability = "read_contract"
	CapReadEarnings        Capability = "read_earnings"
)

type WorkSource struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"` // mcp (remote), mcp-stdio (local command)
	Endpoint     string       `json:"endpoint,omitempty"`
	Command      string       `json:"command,omitempty"`
	Enabled      bool         `json:"enabled"`
	Capabilities []Capability `json:"capabilities"`
}
