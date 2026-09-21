package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ianclemence/scout/pkg/store"
)

func testReg(t *testing.T, mux http.Handler) (*Registry, func()) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatal(err)
	}
	var srv *httptest.Server
	if mux != nil {
		srv = httptest.NewServer(mux)
	}
	r := &Registry{DB: db, OllamaHost: "http://127.0.0.1:1"}
	if srv != nil {
		r.Endpoint = func(p string) string { return srv.URL }
		r.Key = func(p string) string { return "k" }
	}
	return r, func() {
		db.Close()
		if srv != nil {
			srv.Close()
		}
	}
}

func TestBuiltinCatalog(t *testing.T) {
	r, done := testReg(t, nil)
	defer done()
	all := r.List(context.Background(), "")
	if len(all) < 5 {
		t.Fatalf("expected builtin catalog, got %d", len(all))
	}
	found := false
	for _, m := range all {
		if m.Provider == "moonshot" && m.ID == "kimi-k3" && m.Reasoning == "always" {
			found = true
		}
		if m.Context < 0 {
			t.Fatal("negative context")
		}
	}
	if !found {
		t.Fatal("kimi-k3 missing from catalog")
	}
}

func TestRefreshOpenAICompat(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{"data":[{"id":"m1"},{"id":"m2"}]}`))
	})
	r, done := testReg(t, mux)
	defer done()
	n, err := r.Refresh(context.Background(), "moonshot")
	if err != nil || n != 2 {
		t.Fatalf("refresh failed: %d %v", n, err)
	}
	list := r.List(context.Background(), "moonshot")
	ids := map[string]bool{}
	for _, m := range list {
		ids[m.ID] = true
		if m.Source != "discovered" && (m.ID == "m1" || m.ID == "m2") {
			t.Fatal("discovered model mislabeled")
		}
	}
	if !ids["m1"] || !ids["m2"] {
		t.Fatal("discovered models not listed")
	}
}

func TestRefreshOfflineFallsBack(t *testing.T) {
	r, done := testReg(t, nil)
	defer done()
	r.Endpoint = func(p string) string { return "http://127.0.0.1:1" }
	if _, err := r.Refresh(context.Background(), "openai"); err == nil {
		t.Fatal("expected error offline")
	}
	// Cache/builtin path still works offline.
	if len(r.List(context.Background(), "deepseek")) == 0 {
		t.Fatal("expected builtin fallback")
	}
}
