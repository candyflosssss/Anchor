package logbook

// Reconstruct old records from retained attempts at read time. Never rewrite
// history or infer zero for missing usage; truncated details yield only the
// subtotal that can still be confirmed.
func (r *Request) fillKnownUsage() {
	if r.KnownUsage != nil {
		return
	}
	var subtotal KnownUsage
	for _, a := range r.Attempts {
		if a.Input == nil || a.Output == nil || *a.Input < 0 || *a.Output < 0 {
			continue
		}
		subtotal.Input += *a.Input
		subtotal.Output += *a.Output
		subtotal.Attempts++
	}
	if subtotal.Attempts > 0 {
		r.KnownUsage = &subtotal
	}
}
