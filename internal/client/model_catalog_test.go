package client

import (
	"context"
	"errors"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCatalogReadFailuresNeverUnknownOrReplayed(t *testing.T) {
	for _, transportFailure := range []bool{false, true} {
		calls := 0
		c := &Client{endpoint: "http://127.0.0.1", http: &http.Client{Transport: directoryTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != "POST" {
				t.Fatal(r.Method)
			}
			if transportFailure {
				return nil, errors.New("synthetic-private-detail")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("invalid")), Header: make(http.Header)}, nil
		})}}
		q := modeldev.ModelCatalogQuery{ProviderID: "lab", ModelID: "vision-reasoner"}
		for _, estimate := range []bool{false, true} {
			var e error
			if estimate {
				_, e = c.EstimateModelCost(context.Background(), modeldev.CatalogEstimateInput{Query: q})
			} else {
				_, e = c.ModelCatalog(context.Background(), q)
			}
			var req *RequestError
			if !errors.As(e, &req) || req.Unknown || strings.Contains(e.Error(), "private-detail") {
				t.Fatal(e)
			}
		}
		if calls != 2 {
			t.Fatal(calls)
		}
	}
}

func TestCatalogTimeoutDoesNotChangeOtherCalls(t *testing.T) {
	transport := directoryTransport(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) < 40*time.Second || time.Until(deadline) > 45*time.Second {
			t.Fatal("catalog timeout", deadline)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})
	c := &Client{endpoint: "http://127.0.0.1", http: &http.Client{Timeout: 15 * time.Second, Transport: transport}}
	if _, err := c.ModelCatalog(context.Background(), modeldev.ModelCatalogQuery{}); err != nil {
		t.Fatal(err)
	}
	if c.http.Timeout != 15*time.Second {
		t.Fatal("shared client mutated")
	}
	if _, err := c.ProviderAction(context.Background(), catalog.Action{ProviderID: "p", Action: "price-import"}); err != nil {
		t.Fatal(err)
	}
}
