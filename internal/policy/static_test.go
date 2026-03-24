package policy

import (
	"context"
	"testing"

	"github.com/ruizTechServices/nucleus-backend/internal/session"
)

func TestStaticEngineAllowsConfiguredAction(t *testing.T) {
	engine := NewStaticEngine(Config{
		ActionRules: map[string]Rule{
			"tools.list": {
				Decision: DecisionAllow,
				Reason:   "tool discovery is allowed",
			},
		},
	})

	result, err := engine.Evaluate(context.Background(), Input{
		Session: session.Metadata{SessionID: "sess_1"},
		Action:  "tools.list",
	})
	if err != nil {
		t.Fatalf("expected policy evaluation to succeed: %v", err)
	}

	if result.Decision != DecisionAllow {
		t.Fatalf("expected allow decision, got %+v", result)
	}
}

func TestStaticEngineDeniesUnknownActionByDefault(t *testing.T) {
	engine := NewStaticEngine(Config{})

	result, err := engine.Evaluate(context.Background(), Input{
		Action: "tools.list",
	})
	if err != nil {
		t.Fatalf("expected policy evaluation to succeed: %v", err)
	}

	if result.Decision != DecisionDeny {
		t.Fatalf("expected deny-by-default decision, got %+v", result)
	}
}

func TestStaticEngineRequiresApprovalForConfiguredPath(t *testing.T) {
	engine := NewStaticEngine(Config{
		ActionRules: map[string]Rule{
			"filesystem.read": {
				Decision: DecisionAllow,
				Reason:   "filesystem read is allowed",
			},
		},
		PathRules: []PathRule{
			{
				Prefix: "C:/allowed",
				Rule: Rule{
					Decision: DecisionApprovalRequired,
					Reason:   "filesystem path requires approval",
				},
			},
		},
	})

	result, err := engine.Evaluate(context.Background(), Input{
		Action: "filesystem.read",
		Attributes: map[string]any{
			"path": "C:/allowed/file.txt",
		},
	})
	if err != nil {
		t.Fatalf("expected policy evaluation to succeed: %v", err)
	}

	if result.Decision != DecisionApprovalRequired || result.ApprovalID == "" {
		t.Fatalf("expected approval-required decision with approval id, got %+v", result)
	}
}

func TestStaticEngineDeniesTimeoutAboveMax(t *testing.T) {
	engine := NewStaticEngine(Config{
		ActionRules: map[string]Rule{
			"terminal.exec": {
				Decision: DecisionAllow,
				Reason:   "terminal exec allowed for test",
			},
		},
		MaxTimeoutMS: 1000,
	})

	result, err := engine.Evaluate(context.Background(), Input{
		Action: "terminal.exec",
		Attributes: map[string]any{
			"timeout_ms": 5000,
		},
	})
	if err != nil {
		t.Fatalf("expected policy evaluation to succeed: %v", err)
	}

	if result.Decision != DecisionDeny {
		t.Fatalf("expected deny decision for oversized timeout, got %+v", result)
	}
}

func TestStaticEngineAppliesCommandFragments(t *testing.T) {
	engine := NewStaticEngine(Config{
		ActionRules: map[string]Rule{
			"terminal.exec": {
				Decision: DecisionAllow,
				Reason:   "terminal exec allowed for test",
			},
		},
		DeniedCommandFragments:   []string{"&&"},
		ApprovalCommandFragments: []string{"rm "},
	})

	denied, err := engine.Evaluate(context.Background(), Input{
		Action: "terminal.exec",
		Attributes: map[string]any{
			"command": "echo test && whoami",
		},
	})
	if err != nil {
		t.Fatalf("expected command deny evaluation to succeed: %v", err)
	}

	if denied.Decision != DecisionDeny {
		t.Fatalf("expected deny decision for denied fragment, got %+v", denied)
	}

	approval, err := engine.Evaluate(context.Background(), Input{
		Action: "terminal.exec",
		Attributes: map[string]any{
			"command": "rm important.txt",
		},
	})
	if err != nil {
		t.Fatalf("expected command approval evaluation to succeed: %v", err)
	}

	if approval.Decision != DecisionApprovalRequired {
		t.Fatalf("expected approval-required decision for approval fragment, got %+v", approval)
	}
}
