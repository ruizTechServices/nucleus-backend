package tools

import "context"

type Risk string

const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

type Definition struct {
	Name             string
	Risk             Risk
	RequiresApproval bool
	InputSchema      any
	OutputSchema     any
}

type Call struct {
	ToolName  string
	Arguments map[string]any
}

type Result struct {
	ExecutionID string
	ToolName    string
	Payload     any
	Metadata    map[string]any
}

type Handler interface {
	Definition() Definition
	Invoke(ctx context.Context, call Call) (Result, error)
}

type Registry interface {
	List() []Definition
	Lookup(name string) (Handler, bool)
}
