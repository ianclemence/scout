package runtime

import (
	"testing"
)

// TestResetClearsUserDataKeepsCredentials verifies a normal reset wipes user
// data but preserves provider keys and connector configuration.
func TestResetClearsUserDataKeepsCredentials(t *testing.T) {
	c := testCore(t)
	// Seed a session, an opportunity, and a stored credential.
	if _, err := c.DB.DB.Exec(`INSERT INTO sessions(id,name,provider,model,created_at,updated_at) VALUES('s1','t','p','m','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddOpportunity("Job", "A description", "go"); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveSecret("llm:deepseek", "k"); err != nil {
		t.Fatal(err)
	}

	rep, err := c.Reset(ResetScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Tables) == 0 {
		t.Fatal("reset should report cleared tables")
	}
	assertCount(t, c, "sessions", 0)
	assertCount(t, c, "opportunities", 0)
	// Credentials survive a normal reset.
	if _, err := c.LoadSecret("llm:deepseek"); err != nil {
		t.Fatalf("provider credential must survive a normal reset: %v", err)
	}
}

// TestResetAllClearsCredentials verifies --all also removes stored secrets.
func TestResetAllClearsCredentials(t *testing.T) {
	c := testCore(t)
	if err := c.SaveSecret("llm:deepseek", "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Reset(ResetScope{IncludeCredentials: true}); err != nil {
		t.Fatal(err)
	}
	assertCount(t, c, "secrets", 0)
	assertCount(t, c, "sources", 0)
}

func assertCount(t *testing.T, c *Core, table string, want int) {
	t.Helper()
	var n int
	if err := c.DB.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if n != want {
		t.Fatalf("%s = %d, want %d", table, n, want)
	}
}
