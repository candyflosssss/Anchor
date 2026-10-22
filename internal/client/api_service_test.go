package client

import (
	"errors"
	"github.com/candyflosssss/Anchor/internal/state"
	"testing"
)

func TestAPIServiceRejectsUnsupportedCoreBeforeDispatch(t *testing.T) {
	c := &Client{}
	check := func(err error) {
		t.Helper()
		var request *RequestError
		if !errors.As(err, &request) || request.Code != "unsupported" {
			t.Fatal(err)
		}
	}
	_, err := c.APIService(t.Context())
	check(err)
	enabled := true
	_, err = c.SaveAPIService(t.Context(), state.SaveAPIServiceSettings{Enabled: &enabled, AllowLAN: &enabled, Port: 17862, ExpectedRevision: 1})
	check(err)
	_, err = c.IssueAPIKey(t.Context(), state.IssueAPIKey{Label: "phone"})
	check(err)
	_, err = c.RevokeAPIKey(t.Context(), "id")
	check(err)
}
