package providerapi

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"time"
)

func (a *Adapter) ListUsageRecords(ctx context.Context, q Query) (*Records, error) {
	page, size := q.Page, q.PageSize
	if page == 0 {
		page = 1
	}
	if size == 0 {
		size = 50
	}
	scope := q.Scope
	if scope == "" {
		scope = "account"
	}
	out := &Records{Scope: scope, History: "paginated_retained", Page: page, PageSize: size, Items: []Record{}}
	query := url.Values{}
	path := ""
	user := true
	switch a.Config.Platform {
	case "sub2api":
		if err := a.user(); err != nil {
			return nil, err
		}
		path = "/api/v1/usage"
		query.Set("page", strconv.Itoa(page))
		query.Set("page_size", strconv.Itoa(size))
		if scope == "key" {
			if q.RemoteKeyID < 1 {
				return nil, failure("remote_key_required")
			}
			query.Set("api_key_id", remoteID(q.RemoteKeyID))
		}
	case "newapi":
		if scope == "key" {
			if err := a.key(); err != nil {
				return nil, err
			}
			if page != 1 {
				return nil, failure("unsupported_pagination")
			}
			path = "/api/log/token"
			user = false
			out.History = "recent"
			out.PageSize = 0 // endpoint determines the retained recent batch
		} else {
			if err := a.user(); err != nil {
				return nil, err
			}
			path = "/api/log/self"
			query.Set("p", strconv.Itoa(page))
			query.Set("page_size", strconv.Itoa(size))
			query.Set("type", "2")
		}
	case "oneapi":
		if err := a.user(); err != nil {
			return nil, err
		}
		path = "/api/log/self"
		query.Set("p", strconv.Itoa(page-1))
		query.Set("type", "2")
		// OneAPI ignores requested page size; retain the server page intact so
		// records are never lost between offsets. page_size=0 means server-defined.
		out.PageSize = 0
		if scope == "key" {
			// Original OneAPI filters by nonunique token name, not token ID.
			return nil, failure("unsupported_key_filter")
		}
	default:
		return nil, failure("unsupported")
	}
	raw, err := a.get(ctx, path, user, query)
	if err != nil {
		return nil, err
	}
	var rows []object
	if a.Config.Platform == "sub2api" || (a.Config.Platform == "newapi" && scope == "account") {
		d := data(raw)
		if d == nil {
			return nil, failure("invalid_response")
		}
		rows, err = array(d["items"])
		out.Total = integer(d, "total")
		if out.Total == nil || *out.Total < 0 {
			return nil, failure("invalid_response")
		}
		out.HasMore = int64((page-1)*size+len(rows)) < *out.Total
	} else {
		rows, err = array(raw["data"])
		// Original OneAPI has no total/page size. An empty terminal page is needed
		// to establish exhaustion; recent Key logs have no next page at all.
		out.HasMore = a.Config.Platform == "oneapi" && len(rows) > 0
	}
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		// Recent NewAPI logs may include other log types. Only consumption records
		// are requests; top-ups and administrative changes are not model bills.
		if a.Config.Platform != "sub2api" {
			kind := integer(r, "type")
			if kind == nil {
				return nil, failure("invalid_response")
			}
			if *kind != 2 {
				continue
			}
		}
		record, err := a.parseRecord(r)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, record)
	}
	return out, nil
}

func (a *Adapter) parseRecord(m object) (Record, error) {
	out := Record{ID: identifier(m, "id"), IDStable: a.Config.Platform != "newapi", RequestID: text(m, "request_id"), Details: map[string]json.Number{}}
	if a.Config.Platform == "sub2api" {
		out.Model = text(m, "model")
		out.CreatedAt = text(m, "created_at")
		out.Group = identifier(m, "group_id")
		out.RemoteKeyID = identifier(m, "api_key_id")
		out.Tokens = Tokens{Input: integer(m, "input_tokens"), Output: integer(m, "output_tokens"), CacheRead: integer(m, "cache_read_tokens"), CacheWrite: integer(m, "cache_creation_tokens"), CacheWrite5m: integer(m, "cache_creation_5m_tokens"), CacheWrite1h: integer(m, "cache_creation_1h_tokens"), Semantics: "reported_tokens"}
		out.ChargedAmount = amount(m, "actual_cost", "USD")
		out.BaseAmount = amount(m, "total_cost", "USD")
		out.RateMultiplier = number(m, "rate_multiplier")
		copyNumbers(out.Details, m, []string{"input_cost", "output_cost", "cache_creation_cost", "cache_read_cost", "image_input_cost", "image_output_cost", "image_input_tokens", "image_output_tokens", "image_count", "billing_type"})
	} else {
		out.Model = text(m, "model_name")
		out.Group = text(m, "group")
		out.RemoteKeyID = identifier(m, "token_id")
		if ts := integer(m, "created_at"); ts != nil {
			out.CreatedAt = time.Unix(*ts, 0).UTC().Format(time.RFC3339)
		}
		out.Tokens = Tokens{Input: integer(m, "prompt_tokens"), Output: integer(m, "completion_tokens"), Semantics: "reported_tokens"}
		out.ChargedAmount = amount(m, "quota", "quota")
		if a.Config.Platform == "oneapi" {
			out.Tokens.Semantics = "platform_fields_unverified"
		} else {
			var other object
			// NewAPI stores other as a JSON string. Also accept object form used by
			// forks; malformed metadata must not silently erase billing details.
			if raw, ok := m["other"]; ok && string(raw) != "null" {
				var s string
				if json.Unmarshal(raw, &s) == nil {
					if s != "" && json.Unmarshal([]byte(s), &other) != nil {
						return Record{}, failure("invalid_response")
					}
				} else if json.Unmarshal(raw, &other) != nil {
					return Record{}, failure("invalid_response")
				}
			}
			if source := text(other, "billing_source"); source == "wallet" || source == "subscription" {
				out.BillingSource = source
			}
			if raw, ok := other["tool_surcharges"]; ok && string(raw) != "null" {
				tools, err := array(raw)
				if err != nil {
					return Record{}, err
				}
				for _, tool := range tools {
					name := text(tool, "name")
					// Only public builtin tool names; never arbitrary upstream strings.
					switch name {
					case "web_search", "web_search_preview", "file_search", "code_interpreter", "image_generation":
					default:
						continue
					}
					count, price := integer(tool, "count"), number(tool, "price")
					if count == nil || *count < 0 || price == nil {
						return Record{}, failure("invalid_response")
					}
					out.ToolCharges = append(out.ToolCharges, ToolCharge{Name: name, Count: count, Price: price})
				}
			}
			// Tiered billing reports its billing token counts separately from raw usage.
			// Keep only known numeric fields, without its expression or request rules.
			if raw, ok := other["billing_tokens"]; ok && string(raw) != "null" {
				var billing object
				if json.Unmarshal(raw, &billing) != nil || billing == nil {
					return Record{}, failure("invalid_response")
				}
				for _, key := range []string{"p", "c", "len", "cr", "cc", "cc1h", "img", "img_cr", "img_o", "ai", "ao"} {
					if n := number(billing, key); n != nil {
						out.Details["billing_tokens."+key] = *n
					}
				}
			}
			out.Tokens.CacheRead = integer(other, "cache_tokens")
			out.Tokens.CacheWrite = firstInteger(other, "cache_write_tokens", "cache_creation_tokens")
			out.Tokens.CacheWrite5m = firstInteger(other, "cache_creation_tokens_5m", "cache_creation_5m_tokens")
			out.Tokens.CacheWrite1h = firstInteger(other, "cache_creation_tokens_1h", "cache_creation_1h_tokens")
			// Preserve the reported historical group ratio and personal ratio separately.
			out.RateMultiplier = number(other, "group_ratio")
			copyNumbers(out.Details, other, []string{"model_ratio", "completion_ratio", "group_ratio", "user_group_ratio", "model_price", "fixed_price", "cache_tokens", "cache_ratio", "cache_creation_tokens", "cache_creation_ratio", "cache_creation_5m_tokens", "cache_creation_5m_ratio", "cache_creation_1h_tokens", "cache_creation_1h_ratio", "cache_creation_tokens_5m", "cache_creation_ratio_5m", "cache_creation_tokens_1h", "cache_creation_ratio_1h", "audio_input", "audio_output", "text_input", "text_output", "audio_ratio", "audio_completion_ratio", "image_cache_tokens", "image_count", "audio_input_tokens", "audio_output_tokens", "audio_input_price", "audio_output_price", "audio_input_token_count", "input_tokens_total", "cache_write_tokens", "image_output", "image_input_tokens", "image_output_tokens", "image_ratio", "web_search_count", "web_search_price", "file_search_count", "file_search_price", "code_interpreter_call_count", "code_interpreter_price", "subscription_id", "subscription_pre_consumed", "subscription_plan_id", "subscription_total", "subscription_used", "subscription_remain", "subscription_consumed", "wallet_quota_deducted", "subscription_pre_consume_quota", "subscription_post_delta", "subscription_total_amount", "subscription_used_amount"})
		}
	}
	if out.Model == "" || out.CreatedAt == "" || out.ChargedAmount == nil {
		return Record{}, failure("invalid_response")
	}
	return out, nil
}
func copyNumbers(dst map[string]json.Number, src object, keys []string) {
	for _, key := range keys {
		if n := number(src, key); n != nil {
			dst[key] = *n
		}
	}
}

func firstInteger(m object, keys ...string) *int64 {
	for _, key := range keys {
		if n := integer(m, key); n != nil {
			return n
		}
	}
	return nil
}
