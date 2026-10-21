package catalog

// ProbeRequest starts a confirmed inference or reads/cancels its transient run.
type ProbeRequest struct {
	ProviderID       string `json:"provider_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Action           string `json:"action"`
	KeyID            string `json:"key_id"`
	Model            string `json:"model"`
	RunID            string `json:"run_id"`
	Approved         bool   `json:"approved"`
}

// ProbeProgress contains generated text only in memory; it is never persisted in logs.
type ProbeProgress struct {
	ProviderID  string `json:"provider_id"`
	Revision    int64  `json:"revision"`
	RunID       string `json:"run_id"`
	KeyID       string `json:"key_id"`
	Model       string `json:"model"`
	Status      string `json:"status"`
	Phase       string `json:"phase"`
	Text        string `json:"text"`
	ElapsedMS   int64  `json:"elapsed_ms"`
	FirstByteMS int64  `json:"first_byte_ms,omitempty"`
	Result      *Probe `json:"result"`
}
