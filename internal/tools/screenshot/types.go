package screenshot

type CaptureRequest struct {
	DisplayID string `json:"display_id,omitempty"`
}

type CaptureResponse struct {
	CaptureID string         `json:"capture_id"`
	MIMEType  string         `json:"mime_type"`
	Width     int            `json:"width"`
	Height    int            `json:"height"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}
