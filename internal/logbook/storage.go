package logbook

import (
	"encoding/json"
	"reflect"
	"strings"
)

// The compact representation is private to logs.db. Public JSON keeps its
// explicit nulls, empty fields and per-attempt cost snapshots.
func marshalStoredRequest(r Request) ([]byte, error) {
	derived := Request{Attempts: r.Attempts}
	derived.fillKnownUsage()
	if reflect.DeepEqual(r.KnownUsage, derived.KnownUsage) {
		r.KnownUsage = nil
	}
	sharedCost := r.AttemptCount == 1 && !r.AttemptsTruncated && len(r.Attempts) == 1 &&
		r.ReferenceCost != nil && r.ReferenceCost.Adjustment != nil &&
		reflect.DeepEqual(r.ReferenceCost.Adjustment, r.Attempts[0].CostAdjustment)
	if sharedCost {
		// Never change the caller's attempt slice while preparing the snapshot.
		r.Attempts = append([]Attempt(nil), r.Attempts...)
		r.Attempts[0].CostAdjustment = nil
	}
	document := compactLogValue(reflect.ValueOf(r)).(map[string]any)
	if sharedCost {
		document["_attempt_cost_from_root"] = true
	}
	return json.Marshal(document)
}

func unmarshalStoredRequest(raw []byte) (Request, error) {
	var stored struct {
		Request
		AttemptCostFromRoot bool `json:"_attempt_cost_from_root"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return Request{}, err
	}
	r := stored.Request
	if stored.AttemptCostFromRoot && r.AttemptCount == 1 && !r.AttemptsTruncated && len(r.Attempts) == 1 &&
		r.Attempts[0].CostAdjustment == nil && r.ReferenceCost != nil && r.ReferenceCost.Adjustment != nil {
		// Decode an independent copy, matching the ownership of old full rows.
		cost, err := json.Marshal(r.ReferenceCost.Adjustment)
		if err != nil {
			return Request{}, err
		}
		if err := json.Unmarshal(cost, &r.Attempts[0].CostAdjustment); err != nil {
			return Request{}, err
		}
	}
	r.fillKnownUsage()
	return r, nil
}

// Omit only struct fields whose absence decodes to the same typed value.
// Maps (especially evidence), non-nil scalar pointers, zero and false retain
// their values. Empty slices retain [] rather than becoming null on read.
func compactLogValue(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return compactLogValue(v.Elem())
	case reflect.Struct:
		out := make(map[string]any, v.NumField())
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
			if field.PkgPath != "" || name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			value := v.Field(i)
			if value.Kind() == reflect.String && value.Len() == 0 {
				continue
			}
			if (value.Kind() == reflect.Pointer || value.Kind() == reflect.Map || value.Kind() == reflect.Slice) && value.IsNil() {
				continue
			}
			if options == "omitempty" && (value.IsZero() || (value.Kind() == reflect.Slice || value.Kind() == reflect.Map) && value.Len() == 0) {
				continue
			}
			out[name] = compactLogValue(value)
		}
		return out
	case reflect.Slice:
		if v.IsNil() {
			return nil
		}
		out := make([]any, v.Len())
		for i := range out {
			out[i] = compactLogValue(v.Index(i))
		}
		return out
	default:
		return v.Interface()
	}
}
