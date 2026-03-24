package runtime

import (
	"context"
	"encoding/json"

	"github.com/ruizTechServices/nucleus-backend/internal/policy"
	"github.com/ruizTechServices/nucleus-backend/internal/rpc"
	"github.com/ruizTechServices/nucleus-backend/internal/session"
)

func (r *Runtime) authorizeRequest(ctx context.Context, request rpc.Request, metadata session.Metadata) *rpc.Response {
	if r.dependencies.Policy == nil {
		response := rpc.NewErrorResponse(request.ID, rpc.CodeInternalError, "policy engine unavailable", nil)
		return &response
	}

	operation, ok := operationFromRequest(request)
	if !ok {
		response := rpc.NewErrorResponse(request.ID, rpc.CodeValidationError, "invalid request attributes", nil)
		return &response
	}

	input := policy.Input{
		Session:    metadata,
		Action:     operation.Action,
		Resource:   operation.Resource,
		Attributes: operation.Attributes,
	}

	result, err := r.dependencies.Policy.Evaluate(ctx, input)
	if err != nil {
		response := rpc.NewErrorResponse(request.ID, rpc.CodeInternalError, "failed to evaluate policy", map[string]any{
			"reason": err.Error(),
		})
		return &response
	}

	switch result.Decision {
	case policy.DecisionAllow:
		return nil
	case policy.DecisionDeny:
		if response := r.persistPolicyDenied(ctx, request.ID, metadata, request.Method, result.Reason); response != nil {
			return response
		}

		response := rpc.NewErrorResponse(request.ID, rpc.CodePolicyDenied, "policy denied request", map[string]any{
			"reason": result.Reason,
		})
		return &response
	case policy.DecisionApprovalRequired:
		if response := r.persistApprovalRequired(ctx, request.ID, metadata, request.Method, result); response != nil {
			return response
		}

		response := rpc.NewErrorResponse(request.ID, rpc.CodeApprovalRequired, "approval required", map[string]any{
			"reason":      result.Reason,
			"approval_id": result.ApprovalID,
		})
		return &response
	default:
		response := rpc.NewErrorResponse(request.ID, rpc.CodeInternalError, "unknown policy decision", nil)
		return &response
	}
}

type operation struct {
	Action     string
	Resource   string
	Attributes map[string]any
}

func operationFromRequest(request rpc.Request) (operation, bool) {
	switch request.Method {
	case methodToolsCall:
		params, ok := decodeParams[toolCallParams](request)
		if !ok || params.ToolName == "" {
			return operation{}, false
		}

		if params.Arguments == nil {
			params.Arguments = map[string]any{}
		}

		return operation{
			Action:     params.ToolName,
			Resource:   params.ToolName,
			Attributes: params.Arguments,
		}, true
	case methodTerminalStart:
		attributes := requestAttributes(request.Params)
		if workingDirectory, ok := attributes["working_directory"]; ok {
			attributes["path"] = workingDirectory
		}

		return operation{
			Action:     request.Method,
			Resource:   request.Method,
			Attributes: attributes,
		}, true
	default:
		return operation{
			Action:     request.Method,
			Resource:   request.Method,
			Attributes: requestAttributes(request.Params),
		}, true
	}
}

func requestAttributes(raw json.RawMessage) map[string]any {
	attributes, err := rpc.DecodeParams[map[string]any](raw)
	if err != nil || attributes == nil {
		return map[string]any{}
	}

	delete(attributes, "session_token")
	return attributes
}
