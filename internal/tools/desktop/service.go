package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/ruizTechServices/nucleus-backend/internal/tools"
)

type StateProvider interface {
	GetState(ctx context.Context) (GetStateResponse, error)
}

type Config struct {
	Provider StateProvider
}

type Service struct {
	provider StateProvider
}

func NewService(config Config) *Service {
	provider := config.Provider
	if provider == nil {
		provider = defaultProvider{}
	}

	return &Service{provider: provider}
}

func (s *Service) Entries() []tools.Entry {
	if s == nil {
		return nil
	}

	return []tools.Entry{
		{Handler: stateHandler{service: s}},
	}
}

type stateHandler struct {
	service *Service
}

func (h stateHandler) Definition() tools.Definition {
	return tools.Definition{
		Name:             ToolGetState,
		Risk:             tools.RiskLow,
		RequiresApproval: false,
		InputSchema:      GetStateRequest{},
		OutputSchema:     GetStateResponse{},
	}
}

func (h stateHandler) Invoke(ctx context.Context, call tools.Call) (tools.Result, error) {
	if err := ctx.Err(); err != nil {
		return tools.Result{}, err
	}

	response, err := h.service.provider.GetState(ctx)
	if err != nil {
		return tools.Result{}, err
	}

	return tools.Result{
		ToolName: ToolGetState,
		Payload:  response,
	}, nil
}

type defaultProvider struct{}

func (defaultProvider) GetState(ctx context.Context) (GetStateResponse, error) {
	if runtime.GOOS != "windows" {
		return GetStateResponse{}, fmt.Errorf("desktop state is unsupported on this platform")
	}

	script := `
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type @"
using System;
using System.Runtime.InteropServices;
using System.Text;
public static class NucleusUser32 {
	[DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
	[DllImport("user32.dll", SetLastError=true)] public static extern int GetWindowText(IntPtr hWnd, StringBuilder text, int count);
	[DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint processId);
}
"@
$screens = [System.Windows.Forms.Screen]::AllScreens
$displays = @()
for ($i = 0; $i -lt $screens.Length; $i++) {
	$screen = $screens[$i]
	$displayId = if ($screen.Primary) { 'primary' } else { 'display_' + ($i + 1) }
	$displays += @{
		display_id = $displayId
		width = $screen.Bounds.Width
		height = $screen.Bounds.Height
	}
}
$active = $null
$window = [NucleusUser32]::GetForegroundWindow()
if ($window -ne [IntPtr]::Zero) {
	$builder = New-Object System.Text.StringBuilder 1024
	[void][NucleusUser32]::GetWindowText($window, $builder, $builder.Capacity)
	$processId = 0
	[void][NucleusUser32]::GetWindowThreadProcessId($window, [ref]$processId)
	$appName = ''
	if ($processId -gt 0) {
		try {
			$appName = (Get-Process -Id $processId -ErrorAction Stop).ProcessName
		} catch {
			$appName = ''
		}
	}
	$title = $builder.ToString()
	if (-not [string]::IsNullOrWhiteSpace($title) -or -not [string]::IsNullOrWhiteSpace($appName)) {
		$active = @{
			title = $title
			app_name = $appName
		}
	}
}
@{
	active_window = $active
	displays = $displays
} | ConvertTo-Json -Compress -Depth 4`

	commandArgs := []string{"-NoProfile", "-NonInteractive", "-Command", script}
	cmd := exec.CommandContext(ctx, "powershell", commandArgs...)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return GetStateResponse{}, fmt.Errorf("powershell desktop state query failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var response GetStateResponse
	if err := json.Unmarshal([]byte(stdout.String()), &response); err != nil {
		return GetStateResponse{}, err
	}

	return response, nil
}
