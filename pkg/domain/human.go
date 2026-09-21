package domain

import (
	"fmt"
	"strings"
)

// This file formats opportunities and their evaluations for humans: labeled
// sections, human-readable values, and markdown that the terminal renderer
// styles (bold labels, real headings). Machine tokens — the raw source key,
// the internal status, and low-level auth states — are deliberately hidden
// here and remain available in the JSON forms of these types.

// HumanListDetail is a compact, human-readable one-line summary for list rows:
// source and status in plain words, plus the pay when known. Machine tokens
// are never shown here.
func (o *Opportunity) HumanListDetail() string {
	parts := []string{}
	if s := sourceLabel(o.Source); s != "" {
		parts = append(parts, s)
	}
	if st := o.HumanStatus(); st != "" {
		parts = append(parts, st)
	}
	if b := o.HumanBudget(); b != "not stated" {
		parts = append(parts, b)
	}
	return strings.Join(parts, " · ")
}

// HumanBudget renders an opportunity's pay as a readable phrase, or
// "not stated" when the listing gives no amount. It never prints a raw field
// name or a zero that means "unknown".
func (o *Opportunity) HumanBudget() string {
	switch {
	case o.BudgetType == "hourly":
		// Some sources carry the hourly rate in the generic budget fields; fall
		// back to them so a stated rate is never shown as "not stated".
		lo, hi := o.HourlyRateMin, o.HourlyRateMax
		if hi <= 0 {
			lo, hi = o.BudgetMin, o.BudgetMax
		}
		if hi > 0 {
			return fmt.Sprintf("$%.0f–%.0f/hr", lo, hi)
		}
		return "not stated"
	case o.BudgetType == "fixed" && o.BudgetMax > 0:
		if o.BudgetMin == o.BudgetMax {
			return fmt.Sprintf("$%s fixed", compactMoney(o.BudgetMax))
		}
		return fmt.Sprintf("$%s–%s fixed", compactMoney(o.BudgetMin), compactMoney(o.BudgetMax))
	case o.BudgetMax > 0:
		return fmt.Sprintf("$%s–%s", compactMoney(o.BudgetMin), compactMoney(o.BudgetMax))
	default:
		return "not stated"
	}
}

// HumanStatus renders the internal lifecycle status for humans. Internal
// statuses are machine states; the human view says what it means.
func (o *Opportunity) HumanStatus() string {
	switch o.Status {
	case "", "discovered":
		return "not yet reviewed"
	case "analyzed", "reviewed":
		return "reviewed"
	case "applied":
		return "applied"
	case "rejected":
		return "passed"
	default:
		return strings.ReplaceAll(o.Status, "_", " ")
	}
}

// compactMoney folds a trailing ".00" so amounts read as "$500", not "$500.00".
func compactMoney(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.2f", v)
}

// HumanOpportunity renders an opportunity as markdown for the transcript: a
// bold title, a labeled facts block, and the posting body. Machine tokens are
// omitted. The output is designed to pass through the markdown renderer, so it
// uses ** for labels and blank lines between blocks.
func HumanOpportunity(o *Opportunity) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n", strings.TrimSpace(o.Title))

	// Facts block: one labeled line per fact, omitting unknowns so the reader
	// sees signal rather than a wall of empty fields.
	var facts []string
	add := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		facts = append(facts, fmt.Sprintf("- **%s:** %s", label, value))
	}
	add("Budget", o.HumanBudget())
	if o.ConnectsCost > 0 {
		add("Connects", fmt.Sprintf("%d", o.ConnectsCost))
	}
	if o.Client != nil && strings.TrimSpace(o.Client.DisplayName) != "" {
		add("Client", o.Client.DisplayName)
	}
	if o.Company != "" {
		add("Company", o.Company)
	}
	if o.Location != "" {
		add("Location", o.Location)
	}
	if o.RemoteStatus != "" {
		add("Remote", o.RemoteStatus)
	}
	if o.EngagementType != "" {
		add("Engagement", o.EngagementType)
	}
	if n := len(o.Skills); n > 0 {
		add("Skills", strings.Join(o.Skills, ", "))
	}
	add("Source", sourceLabel(o.Source))
	add("Status", o.HumanStatus())
	if o.SourceURL != "" {
		add("Link", fmt.Sprintf("[open posting](%s)", o.SourceURL))
	}
	if len(facts) > 0 {
		b.WriteString(strings.Join(facts, "\n"))
		b.WriteString("\n\n")
	}

	if d := strings.TrimSpace(o.Description); d != "" {
		b.WriteString("**Description**\n\n")
		b.WriteString(d)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// HumanEvaluation renders a match evaluation as markdown: a labeled verdict,
// the deterministic dimensions as a compact list, and the model's enrichment
// as rendered prose. Literal markdown from the model is preserved (the
// renderer styles it); nothing is truncated mid-word here.
func HumanEvaluation(ev *MatchEvaluation, filterPass bool, filterReason string) string {
	if ev == nil {
		return "_Not analyzed yet._"
	}
	var b strings.Builder

	fmt.Fprintf(&b, "**Match:** %s", humanVerdict(ev.Recommendation))
	if r := strings.TrimSpace(ev.Reason); r != "" {
		fmt.Fprintf(&b, " — %s", r)
	}
	b.WriteString("\n\n")

	if len(ev.Dimensions) > 0 {
		b.WriteString("**Assessment**\n\n")
		for _, d := range ev.Dimensions {
			fmt.Fprintf(&b, "- **%s:** %s — %s\n", titleCase(d.Name), d.Rating, d.Detail)
		}
		b.WriteString("\n")
	}
	if len(ev.Risks) > 0 {
		b.WriteString("**Risks:** ")
		b.WriteString(strings.Join(ev.Risks, "; "))
		b.WriteString("\n\n")
	}
	if a := strings.TrimSpace(ev.Analysis); a != "" {
		b.WriteString("**Model notes**\n\n")
		b.WriteString(a)
		b.WriteString("\n\n")
	}
	if !filterPass {
		fmt.Fprintf(&b, "**Gate:** rejected%s\n\n", filterReasonSuffix(filterReason))
	}
	return strings.TrimRight(b.String(), "\n")
}

func filterReasonSuffix(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return ""
	}
	return " — " + reason
}

// humanVerdict maps the machine recommendation to a reader-facing word.
func humanVerdict(rec string) string {
	switch strings.ToLower(rec) {
	case "apply":
		return "worth applying"
	case "review":
		return "worth a look"
	case "ignore":
		return "pass"
	default:
		return rec
	}
}

// sourceLabel strips the internal "src-" prefix and title-cases a source key
// for display (src-upwork -> Upwork).
func sourceLabel(src string) string {
	s := strings.TrimPrefix(strings.TrimSpace(src), "src-")
	if s == "" {
		return ""
	}
	return titleCase(s)
}

func titleCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '_' || r == '-' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
