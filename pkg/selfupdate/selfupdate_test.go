package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		cand, cur string
		want      bool
	}{
		{"v0.6.0", "v0.5.0", true},
		{"v0.6.0", "v0.6.0", false},
		{"v0.6.1", "v0.6.0", true},
		{"v0.5.9", "v0.6.0", false},
		{"v1.0.0", "v0.6.0", true},
		{"v0.6.0", "dev", true},
		{"dev", "v0.6.0", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.cand, c.cur); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.cand, c.cur, got, c.want)
		}
	}
}

func TestChecksumsAndVerify(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "scout_linux_arm64.tar.gz")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := SHA256File(f)
	if err != nil {
		t.Fatal(err)
	}
	// 2baf1f40105d9501bd5f4b8a4a1c0e5f... verify against a known digest of "hello".
	const hello = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if sum != hello {
		t.Fatalf("sha256 of hello = %s", sum)
	}
	if err := VerifySHA256(f, hello); err != nil {
		t.Fatalf("verify should pass: %v", err)
	}
	if err := VerifySHA256(f, "deadbeef"); err == nil {
		t.Fatal("verify should fail on a bad digest")
	}

	parsed := Checksums(hello + "  scout_linux_arm64.tar.gz\n")
	if parsed["scout_linux_arm64.tar.gz"] != hello {
		t.Fatalf("checksums parse failed: %+v", parsed)
	}
}

func TestAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "bin", "scout")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := AtomicReplace(src, dst); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "#!/bin/sh\necho hi\n" {
		t.Fatalf("content wrong: %q", b)
	}
	fi, _ := os.Stat(dst)
	if fi.Mode().Perm()&0o100 == 0 {
		t.Fatalf("installed binary not executable: %v", fi.Mode())
	}
	if _, err := os.Stat(dst + ".new"); !os.IsNotExist(err) {
		t.Fatal("temp .new file should not remain")
	}
}

func TestParseChangelogAndNewEntries(t *testing.T) {
	md := `# Changelog

## [0.6.0] - 2026-01-02
Six changes.

## [0.5.0] - 2026-01-01
Five changes.

## [0.4.0] - 2025-12-31
Four changes.
`
	entries := ParseChangelog(md)
	if len(entries) != 3 || entries[0].Version != "0.6.0" || entries[2].Version != "0.4.0" {
		t.Fatalf("parse wrong: %+v", entries)
	}
	if entries[0].Body != "Six changes." {
		t.Fatalf("body wrong: %q", entries[0].Body)
	}
	newer := NewEntries(entries, "0.5.0")
	if len(newer) != 1 || newer[0].Version != "0.6.0" {
		t.Fatalf("NewEntries wrong: %+v", newer)
	}
	if all := NewEntries(entries, ""); len(all) != 3 {
		t.Fatalf("empty lastSeen should return all, got %d", len(all))
	}
}
