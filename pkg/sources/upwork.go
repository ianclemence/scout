package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ianclemence/scout/pkg/domain"
	"github.com/ianclemence/scout/pkg/match"
	"github.com/ianclemence/scout/pkg/mcpclient"
)

// UpworkAdapter drives Upwork's official MCP server, whose tool contract differs
// from a generic MCP server: every call needs an org_uid (resolved via
// list_accounts) and action-specific parameters nested under `params`. It is
// the first "dialect" adapter; other providers (LinkedIn, JobsDB, …) add their
// own adapter and matcher in NewAdapterFor.
type UpworkAdapter struct {
	IDValue   string
	NameValue string
	Conn      mcpCaller

	mu     sync.Mutex
	tools  []mcpclient.ToolInfo
	caps   []Capability
	orgUID string
}

// NewUpworkAdapter builds the Upwork dialect adapter.
func NewUpworkAdapter(id, name string, conn mcpCaller) *UpworkAdapter {
	return &UpworkAdapter{IDValue: id, NameValue: name, Conn: conn}
}

func (u *UpworkAdapter) ID() string   { return u.IDValue }
func (u *UpworkAdapter) Name() string { return u.NameValue }

func (u *UpworkAdapter) Capabilities() []Capability {
	if len(u.caps) == 0 {
		if err := u.ensureTools(context.Background()); err != nil {
			return nil
		}
	}
	return u.caps
}

func (u *UpworkAdapter) Has(c Capability) bool { return Has(u.Capabilities(), c) }

// Discover refreshes the tool list and capabilities.
func (u *UpworkAdapter) Discover(ctx context.Context) error {
	tools, err := u.Conn.ListTools(ctx)
	if err != nil {
		return err
	}
	u.mu.Lock()
	u.tools = tools
	u.caps = upworkCapabilities(tools)
	u.mu.Unlock()
	return nil
}

func (u *UpworkAdapter) ensureTools(ctx context.Context) error {
	u.mu.Lock()
	have := len(u.tools) > 0
	u.mu.Unlock()
	if have {
		return nil
	}
	return u.Discover(ctx)
}

// upworkCapabilities maps Upwork's tool list to Scout capabilities truthfully.
func upworkCapabilities(tools []mcpclient.ToolInfo) []Capability {
	set := map[Capability]bool{}
	for _, t := range tools {
		switch t.Name {
		case "upwork__find_jobs":
			set[CapSearch] = true
			set[CapReadListing] = true
			set[CapStatus] = true
		case "upwork__manage_proposals", "upwork__confirm_preview", "upwork__confirm_draft":
			set[CapDraft] = true
			set[CapSubmit] = true
		case "upwork__send_message":
			set[CapMessage] = true
		}
	}
	var out []Capability
	for c := range set {
		out = append(out, c)
	}
	return out
}

// resolveOrgUID returns the freelancer (TALENT) org_uid, caching it. Upwork
// requires it on every call.
func (u *UpworkAdapter) resolveOrgUID(ctx context.Context) (string, error) {
	u.mu.Lock()
	if u.orgUID != "" {
		org := u.orgUID
		u.mu.Unlock()
		return org, nil
	}
	u.mu.Unlock()

	raw, err := u.Conn.CallTool(ctx, "upwork__list_accounts", map[string]any{})
	if err != nil {
		return "", fmt.Errorf("list_accounts: %w", err)
	}
	var resp struct {
		Accounts []struct {
			OrgUID string `json:"org_uid"`
			Role   string `json:"role"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return "", fmt.Errorf("list_accounts returned an unparseable payload: %w", err)
	}
	org := ""
	for _, a := range resp.Accounts {
		if a.OrgUID == "" {
			continue
		}
		if a.Role == "TALENT" {
			org = a.OrgUID
			break
		}
		if org == "" {
			org = a.OrgUID
		}
	}
	if org == "" {
		return "", fmt.Errorf("no Upwork account found; sign in again with /sources login Upwork")
	}
	u.mu.Lock()
	u.orgUID = org
	u.mu.Unlock()
	return org, nil
}

// Search runs find_jobs action=search and normalizes the jobs into Scout's
// opportunity model.
func (u *UpworkAdapter) Search(ctx context.Context, f SearchFilter) ([]domain.Opportunity, error) {
	if err := u.ensureTools(ctx); err != nil {
		return nil, err
	}
	org, err := u.resolveOrgUID(ctx)
	if err != nil {
		return nil, err
	}
	params := map[string]any{"limit": clampLimit(f.Limit), "include_full_details": true}
	switch {
	case strings.TrimSpace(f.Title) != "":
		params["title"] = strings.TrimSpace(f.Title)
	case strings.TrimSpace(f.Query) != "":
		params["query"] = strings.TrimSpace(f.Query)
	}
	if len(f.Skills) > 0 {
		params["skills"] = f.Skills
	}
	if f.JobType != "" {
		params["job_type"] = f.JobType
	}
	if f.MinRate > 0 {
		params["rate_min"] = f.MinRate
	}
	if f.MaxRate > 0 {
		params["rate_max"] = f.MaxRate
	}
	if f.MinBudget > 0 && f.JobType == "fixed" {
		params["budget_min"] = f.MinBudget
	}
	if f.Location != "" {
		params["location"] = f.Location
	}
	raw, err := u.Conn.CallTool(ctx, "upwork__find_jobs", map[string]any{
		"action": "search", "org_uid": org, "params": params,
	})
	if err != nil {
		return nil, fmt.Errorf("source %s search failed: %w", u.NameValue, err)
	}
	return normalizeUpworkJobs(u.IDValue, []byte(raw))
}

// Get fetches one job's full detail.
func (u *UpworkAdapter) Get(ctx context.Context, sourceID string) (*domain.Opportunity, error) {
	if err := u.ensureTools(ctx); err != nil {
		return nil, err
	}
	org, err := u.resolveOrgUID(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := u.Conn.CallTool(ctx, "upwork__find_jobs", map[string]any{
		"action": "get", "org_uid": org, "params": map[string]any{"job_id": sourceID},
	})
	if err != nil {
		return nil, err
	}
	return parseUpworkJobDetail(u.IDValue, sourceID, []byte(raw))
}

func (u *UpworkAdapter) Status(ctx context.Context, sourceID string) (string, error) {
	o, err := u.Get(ctx, sourceID)
	if err != nil {
		return "unknown", err
	}
	if o.LiveStatus != "" {
		return o.LiveStatus, nil
	}
	return "unknown", nil
}

func (u *UpworkAdapter) Health(ctx context.Context) Health {
	if err := u.Discover(ctx); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "401") || strings.Contains(strings.ToLower(msg), "unauthoriz") {
			return Health{State: "unauthenticated", Detail: "authentication required"}
		}
		return Health{State: "unavailable", Detail: msg}
	}
	if _, err := u.resolveOrgUID(ctx); err != nil {
		return Health{State: "misconfigured", Detail: err.Error()}
	}
	return Health{State: "connected", Detail: fmt.Sprintf("%d tools", len(u.tools))}
}

func (u *UpworkAdapter) Close() error { return nil }

// SubmitApplication creates a proposal preview and confirms it in one step,
// because Core has already cleared the approval gate. Upwork's draft-confirm
// model means the create is a draft and the confirm is the binding submit.
func (u *UpworkAdapter) SubmitApplication(ctx context.Context, args map[string]any) (string, error) {
	org, err := u.resolveOrgUID(ctx)
	if err != nil {
		return "", err
	}
	jobID := firstString(args, "job_reference", "job_id", "opportunity_id", "id")
	if jobID == "" {
		return "", fmt.Errorf("job_reference is required")
	}
	cover := firstString(args, "cover_letter", "proposal")
	if cover == "" {
		return "", fmt.Errorf("cover_letter is required")
	}
	// charged_amount is required by the live contract and must be a number,
	// not a string.
	amount, ok := args["charged_amount"].(float64)
	if !ok {
		if v, ok2 := args["bid"].(float64); ok2 {
			amount, ok = v, true
		}
	}
	if !ok || amount <= 0 {
		return "", fmt.Errorf("a numeric charged_amount (bid) greater than 0 is required — set a rate on the proposal or the profile's minimum rate first")
	}

	// MANDATORY pre-check (Upwork rejects create with VJ-JA-10 otherwise):
	// an existing invitation must be accepted, not created against, and an
	// existing proposal must not be duplicated.
	if inv := u.existingInvitation(ctx, org, jobID); inv != "" {
		return "", fmt.Errorf("this job has an invitation (%s); use accept-invitation instead of creating a proposal", inv)
	}
	if prior := u.existingProposal(ctx, org, jobID); prior != "" {
		return "", fmt.Errorf("a proposal already exists for this job (%s); edit or withdraw it instead", prior)
	}

	params := map[string]any{
		"job_reference":  jobID,
		"cover_letter":   cover,
		"charged_amount": amount,
	}
	if ans, ok := args["answers"].([]any); ok && len(ans) > 0 {
		params["answers"] = ans
	}
	created, err := u.Conn.CallTool(ctx, "upwork__manage_proposals", map[string]any{
		"action": "create", "org_uid": org, "params": params,
	})
	if err != nil {
		return "", fmt.Errorf("create proposal preview: %w", err)
	}
	// The server can gate the first proposal behind a policy acknowledgment.
	// Do not confirm it silently: surface the exact step the user must take.
	if needsPolicyAck(created) {
		return created + "\n\nAction needed: Scout must acknowledge Upwork's payment-protection policy once (manage_proposals action=acknowledge_policy) after you confirm you understand it. The proposal was not submitted.", nil
	}
	previewID := extractString(created, "preview_id", "draft_id")
	if previewID == "" {
		return created, nil // already executed server-side
	}
	confirmed, err := u.Conn.CallTool(ctx, "upwork__confirm_preview", map[string]any{
		"action": "confirm", "org_uid": org,
		"params": map[string]any{"type": "proposal", "preview_id": previewID},
	})
	if err != nil {
		return created, fmt.Errorf("confirm proposal: %w", err)
	}
	return confirmed, nil
}

// existingInvitation reports the invitation id for a job, if one exists.
func (u *UpworkAdapter) existingInvitation(ctx context.Context, org, jobID string) string {
	out, err := u.Conn.CallTool(ctx, "upwork__list_freelancer_proposals", map[string]any{
		"action": "invitations", "org_uid": org,
	})
	if err != nil {
		return "" // a failed pre-check must not block a legitimate create
	}
	// Match an invitation whose job reference equals this job.
	if extractString(out, jobID) != "" && strings.Contains(out, jobID) {
		if id := extractString(out, "id"); id != "" {
			return id
		}
		return "present"
	}
	return ""
}

// existingProposal reports a prior proposal id for a job, if one exists.
func (u *UpworkAdapter) existingProposal(ctx context.Context, org, jobID string) string {
	out, err := u.Conn.CallTool(ctx, "upwork__list_freelancer_proposals", map[string]any{
		"action": "list", "org_uid": org,
	})
	if err != nil {
		return ""
	}
	if strings.Contains(out, jobID) {
		if id := extractString(out, "id"); id != "" {
			return id
		}
		return "present"
	}
	return ""
}

// needsPolicyAck reports whether a create response is the one-time policy
// acknowledgment gate rather than a usable preview.
func needsPolicyAck(out string) bool {
	low := strings.ToLower(out)
	return strings.Contains(low, "acknowledge") && strings.Contains(low, "policy")
}

// SendMessage sends a room message (or starts a conversation with a user).
func (u *UpworkAdapter) SendMessage(ctx context.Context, args map[string]any) (string, error) {
	org, err := u.resolveOrgUID(ctx)
	if err != nil {
		return "", err
	}
	body := firstString(args, "body", "message")
	if body == "" {
		return "", fmt.Errorf("message body is required")
	}
	// The live contract has two distinct sends: to a room (send) and to a user
	// (send_to_user, which needs the recipient's user_id and org_id).
	if room := firstString(args, "room_id", "to"); room != "" {
		return u.Conn.CallTool(ctx, "upwork__send_message", map[string]any{
			"action": "send", "org_uid": org,
			"params": map[string]any{"room_id": room, "message": body},
		})
	}
	userID := firstString(args, "user_id")
	orgID := firstString(args, "org_id", "recipient_org_id")
	if userID == "" || orgID == "" {
		return "", fmt.Errorf("room_id, or both user_id and org_id, are required to send a new message")
	}
	params := map[string]any{"user_id": userID, "org_id": orgID, "message": body}
	if jobID := firstString(args, "job_posting_id"); jobID != "" {
		params["job_posting_id"] = jobID
	}
	return u.Conn.CallTool(ctx, "upwork__send_message", map[string]any{
		"action": "send_to_user", "org_uid": org, "params": params,
	})
}

// ---------- normalization ----------

type upworkJob struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Snippet         string   `json:"description_snippet"`
	Budget          string   `json:"budget"`
	JobType         string   `json:"job_type"`
	Skills          []string `json:"skills"`
	URL             string   `json:"url"`
	CreatedDate     string   `json:"created_date"`
	PublishedDate   string   `json:"published_date"`
	ExperienceLevel string   `json:"experience_level"`
	Duration        string   `json:"duration"`
	Applied         bool     `json:"applied"`
	Client          struct {
		Country string  `json:"country"`
		Rating  float64 `json:"rating"`
	} `json:"client"`
}

func normalizeUpworkJobs(source string, raw []byte) ([]domain.Opportunity, error) {
	var resp struct {
		Jobs []upworkJob `json:"jobs"`
		// Upwork reports rejected filters when a value is outside its vocab.
		FiltersRejected map[string]any `json:"filters_rejected"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("source returned an unparseable search payload: %w", err)
	}
	if resp.Jobs == nil && resp.FiltersRejected != nil {
		return nil, fmt.Errorf("source rejected filters: %v", resp.FiltersRejected)
	}
	out := make([]domain.Opportunity, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		out = append(out, upworkJobToDomain(source, j))
	}
	return out, nil
}

func upworkJobToDomain(source string, j upworkJob) domain.Opportunity {
	desc := stripUntrusted(j.Description)
	if desc == "" {
		desc = stripUntrusted(j.Snippet)
	}
	btype, bmin, bmax := parseUpworkBudget(j.Budget, j.JobType)
	o := domain.Opportunity{
		Source:       source,
		SourceOppID:  j.ID,
		CanonicalURL: j.URL,
		Title:        j.Title,
		Description:  desc,
		Skills:       j.Skills,
		BudgetType:   btype,
		BudgetMin:    bmin,
		BudgetMax:    bmax,
		RemoteStatus: "remote",
		LiveStatus:   "active",
		Status:       "discovered",
		RawSnapshot:  mustJSON(j),
		PostedAt:     parseUpworkTime(firstNonEmpty(j.PublishedDate, j.CreatedDate)),
		Category:     j.ExperienceLevel,
	}
	o.Fingerprint = match.Fingerprint(source, j.ID, j.Title, desc)
	return o
}

func parseUpworkJobDetail(source, jobID string, raw []byte) (*domain.Opportunity, error) {
	var resp struct {
		Data struct {
			Job struct {
				Content struct {
					Title       string `json:"title"`
					Description string `json:"description"`
				} `json:"content"`
				ContractTerms struct {
					ContractType        string `json:"contractType"`
					ExperienceLevel     string `json:"experienceLevel"`
					HourlyContractTerms struct {
						HourlyBudgetMin float64 `json:"hourlyBudgetMin"`
						HourlyBudgetMax float64 `json:"hourlyBudgetMax"`
					} `json:"hourlyContractTerms"`
				} `json:"contractTerms"`
				Classification struct {
					Category struct {
						PreferredLabel string `json:"preferredLabel"`
					} `json:"category"`
					SubCategory struct {
						PreferredLabel string `json:"preferredLabel"`
					} `json:"subCategory"`
				} `json:"classification"`
				ClientCompanyPublic struct {
					Country struct {
						Name string `json:"name"`
					} `json:"country"`
				} `json:"clientCompanyPublic"`
				URL string `json:"url"`
			} `json:"marketplaceJobPosting"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("source returned an unparseable job payload: %w", err)
	}
	j := resp.Data.Job
	o := &domain.Opportunity{
		Source:       source,
		SourceOppID:  jobID,
		CanonicalURL: j.URL,
		Title:        j.Content.Title,
		Description:  stripUntrusted(strings.TrimSpace(j.Content.Description)),
		Category:     strings.TrimSpace(j.Classification.Category.PreferredLabel + " / " + j.Classification.SubCategory.PreferredLabel),
		RemoteStatus: "remote",
		LiveStatus:   "active",
		Status:       "discovered",
		RawSnapshot:  string(raw),
	}
	if strings.EqualFold(j.ContractTerms.ContractType, "HOURLY") {
		o.BudgetType = "hourly"
		o.BudgetMin = j.ContractTerms.HourlyContractTerms.HourlyBudgetMin
		o.BudgetMax = j.ContractTerms.HourlyContractTerms.HourlyBudgetMax
	} else if j.ContractTerms.ContractType != "" {
		o.BudgetType = "fixed"
	}
	o.Fingerprint = match.Fingerprint(source, jobID, o.Title, o.Description)
	return o, nil
}

var upworkNumRe = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

// parseUpworkBudget parses Upwork's budget string ("30.00–50.00/hr", "12,000.00",
// "") into a normalized type and bounds.
func parseUpworkBudget(s, jobType string) (btype string, min, max float64) {
	s = strings.TrimSpace(s)
	switch {
	case strings.Contains(s, "/hr") || strings.EqualFold(jobType, "hourly"):
		btype = "hourly"
	case s != "" || strings.EqualFold(jobType, "fixed"):
		btype = "fixed"
	default:
		return "unknown", 0, 0
	}
	s = strings.ReplaceAll(s, ",", "")
	nums := upworkNumRe.FindAllString(s, -1)
	vals := make([]float64, 0, len(nums))
	for _, n := range nums {
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			vals = append(vals, f)
		}
	}
	switch len(vals) {
	case 0:
		return btype, 0, 0
	case 1:
		return btype, vals[0], vals[0]
	default:
		return btype, vals[0], vals[1]
	}
}

// stripUntrusted removes Upwork's untrusted-content wrapper tags, keeping the
// content. The agent loop still labels tool results as untrusted data.
func stripUntrusted(s string) string {
	s = strings.ReplaceAll(s, "<untrusted_participant_content>", "")
	s = strings.ReplaceAll(s, "</untrusted_participant_content>", "")
	return strings.TrimSpace(s)
}

func clampLimit(n int) int {
	if n <= 0 {
		return 10
	}
	if n > 10 {
		return 10
	}
	return n
}

func firstString(args map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := args[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseUpworkTime(s string) time.Time {
	if strings.TrimSpace(s) == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// extractString finds a string value for any of the given keys anywhere in a
// JSON object (Upwork nests preview ids under different shapes).
func extractString(raw string, keys ...string) string {
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return ""
	}
	return findString(m, keys)
}

func findString(m map[string]any, keys []string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	for _, v := range m {
		if child, ok := v.(map[string]any); ok {
			if found := findString(child, keys); found != "" {
				return found
			}
		}
	}
	return ""
}
