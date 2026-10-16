package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/skills"
)

func TestSkillResponseBudgetAndReadOnlyFailure(t *testing.T) {
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A valid large catalog must not become an unknown write solely because
		// the shared client's older general-purpose response limit is 1 MiB.
		if r.URL.Path == "/control/v1/skills" {
			_ = json.NewEncoder(w).Encode(skills.SkillWorkspace{Revision: 1, Warnings: []string{strings.Repeat("x", (1<<20)+1)}})
			return
		}
		_, _ = w.Write([]byte("not json"))
	}))
	defer web.Close()
	c := &Client{endpoint: web.URL, token: "synthetic", instance: "synthetic", http: web.Client()}
	ctx := context.Background()
	if _, err := c.SkillWorkspace(ctx); err != nil {
		t.Fatal(err)
	}
	var raw any
	if err := c.call(ctx, "GET", "/control/v1/skills", nil, &raw); err == nil {
		t.Fatal("generic limit silently expanded")
	}
	_, err := c.SkillDetail(ctx, skills.SkillQuery{})
	var re *RequestError
	if !errors.As(err, &re) || re.Unknown {
		t.Fatal(err)
	}
	_, err = c.SkillAction(ctx, skills.SkillAction{})
	if !errors.As(err, &re) || !re.Unknown {
		t.Fatal("actual write lost unknown-result flag", err)
	}
}
