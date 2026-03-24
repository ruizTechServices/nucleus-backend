package desktop

const ToolGetState = "desktop.get_state"

type GetStateRequest struct{}

type ActiveWindow struct {
	Title   string `json:"title"`
	AppName string `json:"app_name"`
}

type Display struct {
	DisplayID string `json:"display_id"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type GetStateResponse struct {
	ActiveWindow *ActiveWindow `json:"active_window,omitempty"`
	Displays     []Display     `json:"displays,omitempty"`
}
