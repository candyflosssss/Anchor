package providerapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func dataGroup(key object) object {
	var group object
	_ = json.Unmarshal(key["group"], &group)
	return group
}

// Bound automatic lookup to four account-list pages (100 rows each) within
// the same eight-request / 2 MiB adapter budget. Secrets only enter headers.
func (a *Adapter) getKeyRate(ctx context.Context) (*Rate, error) {
	if a.Config.Platform == "oneapi" {
		return nil, failure("unsupported")
	}
	if err := a.key(); err != nil {
		return nil, err
	}
	if err := a.user(); err != nil {
		return nil, err
	}
	path := "/api/token/"
	if a.Config.Platform == "sub2api" {
		path = "/api/v1/keys"
	}
	for page := 1; page <= 4; page++ {
		params := url.Values{"page": {strconv.Itoa(page)}, "page_size": {"100"}}
		if a.Config.Platform == "newapi" {
			params = url.Values{"p": {strconv.Itoa(page)}, "size": {"100"}}
		}
		raw, err := a.get(ctx, path, true, params)
		if err != nil {
			return nil, err
		}
		body := data(raw)
		rowsRaw := body["items"]
		if len(rowsRaw) == 0 {
			rowsRaw = raw["data"]
		}
		rows, err := array(rowsRaw)
		if err != nil {
			return nil, err
		}
		if len(rows) > 100 {
			return nil, failure("too_large")
		}
		for _, row := range rows {
			remote := integer(row, "id")
			candidate := text(row, "key")
			if remote == nil || *remote < 1 {
				return nil, failure("invalid_response")
			}
			saved := a.Key
			if a.Config.Platform == "newapi" {
				candidate = strings.TrimPrefix(candidate, "sk-")
				saved = strings.TrimPrefix(saved, "sk-")
			}
			matched := candidate != "" && candidate == saved
			if !matched && a.Config.Platform == "newapi" && maskedKeyCandidate(candidate, saved) {
				// NewAPI masks account-list keys. Verify the full key through
				// its fixed read endpoint; a masked match is never identity proof.
				full, err := a.read(ctx, http.MethodPost, "/api/token/"+remoteID(*remote)+"/key", true, false, nil)
				if err != nil {
					return nil, err
				}
				matched = strings.TrimPrefix(text(data(full), "key"), "sk-") == saved
			}
			if matched {
				rate, err := a.GetEffectiveRate(ctx, *remote)
				if err == nil && rate != nil && rate.Multiplier != nil {
					value, parseErr := strconv.ParseFloat(rate.Multiplier.String(), 64)
					if parseErr != nil || value < 0 {
						return nil, failure("invalid_response")
					}
				}
				return rate, err
			}
		}
		total := integer(body, "total")
		if total != nil && *total < 0 {
			return nil, failure("invalid_response")
		}
		if len(rows) < 100 || (total != nil && int64(page*100) >= *total) {
			return nil, failure("key_not_found")
		}
	}
	return nil, failure("too_large")
}

func maskedKeyCandidate(masked, saved string) bool {
	start := strings.IndexByte(masked, '*')
	end := strings.LastIndexByte(masked, '*')
	return start > 0 && end < len(masked)-1 && strings.Trim(masked[start:end+1], "*") == "" &&
		strings.HasPrefix(saved, masked[:start]) && strings.HasSuffix(saved, masked[end+1:]) && len(saved) > start+len(masked)-end-1
}
