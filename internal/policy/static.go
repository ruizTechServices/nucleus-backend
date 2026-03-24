package policy

import (
	"context"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"runtime"
	"strings"
)

type Rule struct {
	Decision Decision
	Reason   string
}

type PathRule struct {
	Prefix string
	Rule   Rule
}

type Config struct {
	ActionRules              map[string]Rule
	PathRules                []PathRule
	MaxTimeoutMS             int
	DeniedCommandFragments   []string
	ApprovalCommandFragments []string
}

type StaticEngine struct {
	config Config
}

func NewStaticEngine(config Config) *StaticEngine {
	return &StaticEngine{config: config}
}

func DefaultConfig() Config {
	return Config{
		ActionRules: map[string]Rule{
			"tools.list": {
				Decision: DecisionAllow,
				Reason:   "tool discovery is allowed",
			},
			"filesystem.list": {
				Decision: DecisionAllow,
				Reason:   "filesystem directory listing is allowed within approved scope",
			},
			"filesystem.read": {
				Decision: DecisionAllow,
				Reason:   "filesystem file read is allowed within approved scope",
			},
			"terminal.start_session": {
				Decision: DecisionApprovalRequired,
				Reason:   "terminal session start requires approval",
			},
			"terminal.exec": {
				Decision: DecisionApprovalRequired,
				Reason:   "terminal command execution requires approval",
			},
			"terminal.end_session": {
				Decision: DecisionAllow,
				Reason:   "terminal session end is allowed",
			},
			"screenshot.capture": {
				Decision: DecisionApprovalRequired,
				Reason:   "screenshot capture requires approval",
			},
			"desktop.get_state": {
				Decision: DecisionAllow,
				Reason:   "desktop state is allowed",
			},
		},
		MaxTimeoutMS:             30000,
		DeniedCommandFragments:   []string{"&&", "||", ";"},
		ApprovalCommandFragments: []string{"rm ", "del ", "format", "shutdown"},
	}
}

func (e *StaticEngine) Evaluate(ctx context.Context, input Input) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	result := e.evaluateAction(input)

	if pathResult, ok := e.evaluatePath(input); ok {
		result = mergeResults(result, pathResult)
	}

	if timeoutResult, ok := e.evaluateTimeout(input); ok {
		result = mergeResults(result, timeoutResult)
	}

	if commandResult, ok := e.evaluateCommand(input); ok {
		result = mergeResults(result, commandResult)
	}

	if result.Decision == DecisionApprovalRequired && result.ApprovalID == "" {
		result.ApprovalID = approvalID(input, result.Reason)
	}

	return result, nil
}

func (e *StaticEngine) evaluateAction(input Input) Result {
	rule, ok := e.config.ActionRules[input.Action]
	if !ok {
		return Result{
			Decision: DecisionDeny,
			Reason:   "action denied by default",
		}
	}

	return Result{
		Decision: rule.Decision,
		Reason:   rule.Reason,
	}
}

func (e *StaticEngine) evaluatePath(input Input) (Result, bool) {
	path, ok := stringAttribute(input.Attributes, "path")
	if !ok {
		return Result{}, false
	}

	if len(e.config.PathRules) == 0 {
		return Result{
			Decision: DecisionDeny,
			Reason:   "path access denied by default",
		}, true
	}

	normalizedPath := normalizePath(path)
	for _, rule := range e.config.PathRules {
		if pathWithinScope(normalizedPath, normalizePath(rule.Prefix)) {
			return Result{
				Decision:   rule.Rule.Decision,
				Reason:     rule.Rule.Reason,
				ApprovalID: approvalID(input, rule.Rule.Reason),
			}, true
		}
	}

	return Result{
		Decision: DecisionDeny,
		Reason:   "path out of allowed scope",
	}, true
}

func (e *StaticEngine) evaluateTimeout(input Input) (Result, bool) {
	if e.config.MaxTimeoutMS <= 0 {
		return Result{}, false
	}

	timeoutMS, ok := intAttribute(input.Attributes, "timeout_ms")
	if !ok {
		return Result{}, false
	}

	if timeoutMS > e.config.MaxTimeoutMS {
		return Result{
			Decision: DecisionDeny,
			Reason:   fmt.Sprintf("timeout exceeds max of %dms", e.config.MaxTimeoutMS),
		}, true
	}

	return Result{}, false
}

func (e *StaticEngine) evaluateCommand(input Input) (Result, bool) {
	command, ok := stringAttribute(input.Attributes, "command")
	if !ok {
		return Result{}, false
	}

	normalizedCommand := strings.ToLower(command)
	for _, fragment := range e.config.DeniedCommandFragments {
		if strings.Contains(normalizedCommand, strings.ToLower(fragment)) {
			return Result{
				Decision: DecisionDeny,
				Reason:   fmt.Sprintf("command contains denied fragment %q", fragment),
			}, true
		}
	}

	for _, fragment := range e.config.ApprovalCommandFragments {
		if strings.Contains(normalizedCommand, strings.ToLower(fragment)) {
			return Result{
				Decision:   DecisionApprovalRequired,
				Reason:     fmt.Sprintf("command requires approval due to fragment %q", fragment),
				ApprovalID: approvalID(input, fragment),
			}, true
		}
	}

	return Result{}, false
}

func mergeResults(current Result, next Result) Result {
	if next.Decision == "" {
		return current
	}

	if next.Decision == DecisionDeny || current.Decision == DecisionDeny {
		if next.Decision == DecisionDeny {
			return next
		}
		return current
	}

	if next.Decision == DecisionApprovalRequired {
		return next
	}

	return current
}

func stringAttribute(attributes map[string]any, key string) (string, bool) {
	if attributes == nil {
		return "", false
	}

	value, ok := attributes[key]
	if !ok {
		return "", false
	}

	stringValue, ok := value.(string)
	if !ok || strings.TrimSpace(stringValue) == "" {
		return "", false
	}

	return stringValue, true
}

func intAttribute(attributes map[string]any, key string) (int, bool) {
	if attributes == nil {
		return 0, false
	}

	value, ok := attributes[key]
	if !ok {
		return 0, false
	}

	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func approvalID(input Input, seed string) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(input.Action))
	_, _ = hasher.Write([]byte("|"))
	_, _ = hasher.Write([]byte(input.Resource))
	_, _ = hasher.Write([]byte("|"))
	_, _ = hasher.Write([]byte(seed))
	return fmt.Sprintf("approval_%08x", hasher.Sum32())
}

func normalizePath(value string) string {
	normalized := filepath.ToSlash(filepath.Clean(value))
	if runtime.GOOS == "windows" {
		return strings.ToLower(normalized)
	}
	return normalized
}

func pathWithinScope(path string, prefix string) bool {
	if path == prefix {
		return true
	}

	return strings.HasPrefix(path, prefix+"/")
}
