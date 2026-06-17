package router

type IRRequest struct {
	Model       string            `json:"model"`
	System      string            `json:"system,omitempty"`
	Messages    []IRMessage       `json:"messages,omitempty"`
	Input       string            `json:"input,omitempty"`
	InputParts  []IRContentPart   `json:"input_parts,omitempty"`
	Tools       []map[string]any  `json:"tools,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
	Thinking    map[string]any    `json:"thinking,omitempty"`
	Stop        []string          `json:"stop,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Raw         map[string]any    `json:"raw,omitempty"`
	NoCache     bool              `json:"no_cache,omitempty"`
}

type IRMessage struct {
	Role    string          `json:"role"`
	Content string          `json:"content"`
	Parts   []IRContentPart `json:"parts,omitempty"`
}

type IRContentPart struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ImageURL  string `json:"image_url,omitempty"`
	Detail    string `json:"detail,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	FileID    string `json:"file_id,omitempty"`
}

type IRResponse struct {
	ID          string            `json:"id"`
	Model       string            `json:"model"`
	Text        string            `json:"text"`
	StopReason  string            `json:"stop_reason"`
	Usage       Usage             `json:"usage"`
	Raw         map[string]any    `json:"raw,omitempty"`
	RawResponse bool              `json:"raw_response,omitempty"`
	Warnings    []string          `json:"warnings,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

type Usage struct {
	InputTokens                   int     `json:"input_tokens"`
	OutputTokens                  int     `json:"output_tokens"`
	TotalTokens                   int     `json:"total_tokens"`
	InputImageTokens              int     `json:"input_image_tokens,omitempty"`
	UpstreamReportedInputCostUSD  float64 `json:"upstream_reported_input_cost_usd,omitempty"`
	UpstreamReportedOutputCostUSD float64 `json:"upstream_reported_output_cost_usd,omitempty"`
	UpstreamReportedTotalCostUSD  float64 `json:"upstream_reported_total_cost_usd,omitempty"`
}

func estimateTokens(r *IRRequest) int {
	chars := len(r.System) + len(r.Input)
	for _, p := range r.InputParts {
		chars += len(p.Text)
		if p.Type == "image" {
			chars += 1024
		}
	}
	for _, m := range r.Messages {
		chars += len(m.Role) + len(m.Content)
		for _, p := range m.Parts {
			chars += len(p.Text)
			if p.Type == "image" {
				chars += 1024
			}
		}
	}
	if chars == 0 {
		return 1
	}
	return chars/4 + 1
}
