package screenshot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ruizTechServices/nucleus-backend/internal/tools"
)

type CaptureProvider interface {
	Capture(ctx context.Context, request CaptureRequest) (CaptureResponse, error)
}

type Config struct {
	CaptureDirectory  string
	GenerateCaptureID func() (string, error)
	Provider          CaptureProvider
}

type Service struct {
	provider CaptureProvider
}

func NewService(config Config) (*Service, error) {
	provider := config.Provider
	if provider == nil {
		defaultProvider, err := newDefaultProvider(config)
		if err != nil {
			return nil, err
		}

		provider = defaultProvider
	}

	return &Service{provider: provider}, nil
}

func (s *Service) Entries() []tools.Entry {
	if s == nil {
		return nil
	}

	return []tools.Entry{
		{Handler: captureHandler{service: s}},
	}
}

type captureHandler struct {
	service *Service
}

func (h captureHandler) Definition() tools.Definition {
	return tools.Definition{
		Name:             ToolCapture,
		Risk:             tools.RiskHigh,
		RequiresApproval: true,
		InputSchema:      CaptureRequest{},
		OutputSchema:     CaptureResponse{},
	}
}

func (h captureHandler) Invoke(ctx context.Context, call tools.Call) (tools.Result, error) {
	if err := ctx.Err(); err != nil {
		return tools.Result{}, err
	}

	request, err := decodeArguments[CaptureRequest](call.Arguments)
	if err != nil {
		return tools.Result{}, err
	}

	response, err := h.service.provider.Capture(ctx, request)
	if err != nil {
		return tools.Result{}, err
	}

	return tools.Result{
		ToolName: ToolCapture,
		Payload:  response,
		Metadata: response.Metadata,
	}, nil
}

type defaultProvider struct {
	captureDirectory  string
	generateCaptureID func() (string, error)
}

func newDefaultProvider(config Config) (*defaultProvider, error) {
	captureDirectory := strings.TrimSpace(config.CaptureDirectory)
	if captureDirectory == "" {
		captureDirectory = filepath.Join(os.TempDir(), "nucleus", "captures")
	}

	if err := os.MkdirAll(captureDirectory, 0o755); err != nil {
		return nil, err
	}

	generateCaptureID := config.GenerateCaptureID
	if generateCaptureID == nil {
		generateCaptureID = newCaptureID
	}

	return &defaultProvider{
		captureDirectory:  captureDirectory,
		generateCaptureID: generateCaptureID,
	}, nil
}

func (p *defaultProvider) Capture(ctx context.Context, request CaptureRequest) (CaptureResponse, error) {
	if runtime.GOOS != "windows" {
		return CaptureResponse{}, fmt.Errorf("screenshot capture is unsupported on this platform")
	}

	captureID, err := p.generateCaptureID()
	if err != nil {
		return CaptureResponse{}, err
	}

	outputPath := filepath.Join(p.captureDirectory, captureID+".png")
	script := `
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$outputPath = $args[0]
$displayId = $args[1]
$screens = [System.Windows.Forms.Screen]::AllScreens
$selected = $null
if ([string]::IsNullOrWhiteSpace($displayId) -or $displayId -eq 'primary') {
	$selected = $screens | Where-Object { $_.Primary } | Select-Object -First 1
	if (-not $selected) {
		$selected = $screens | Select-Object -First 1
	}
} else {
	if ($displayId -match '^display_(\d+)$') {
		$index = [int]$Matches[1] - 1
		if ($index -ge 0 -and $index -lt $screens.Length) {
			$selected = $screens[$index]
		}
	} else {
		$selected = $screens | Where-Object { $_.DeviceName -eq $displayId } | Select-Object -First 1
	}
}
if (-not $selected) {
	[Console]::Error.WriteLine('DISPLAY_NOT_FOUND')
	exit 2
}
$bounds = $selected.Bounds
$bitmap = New-Object System.Drawing.Bitmap $bounds.Width, $bounds.Height
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
try {
	$graphics.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size)
	$bitmap.Save($outputPath, [System.Drawing.Imaging.ImageFormat]::Png)
} finally {
	$graphics.Dispose()
	$bitmap.Dispose()
}
$displayLabel = if ($selected.Primary) { 'primary' } else { 'display_' + ([Array]::IndexOf($screens, $selected) + 1) }
@{
	display_id = $displayLabel
	width = $bounds.Width
	height = $bounds.Height
	path = $outputPath
} | ConvertTo-Json -Compress`

	output, stderr, err := runPowerShellJSON(ctx, script, outputPath, strings.TrimSpace(request.DisplayID))
	if err != nil {
		if strings.Contains(stderr, "DISPLAY_NOT_FOUND") {
			return CaptureResponse{}, tools.NewValidationError("display not found", map[string]any{
				"display_id": request.DisplayID,
			})
		}

		return CaptureResponse{}, err
	}

	var parsed struct {
		DisplayID string `json:"display_id"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Path      string `json:"path"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return CaptureResponse{}, err
	}

	return CaptureResponse{
		CaptureID: captureID,
		MIMEType:  "image/png",
		Width:     parsed.Width,
		Height:    parsed.Height,
		Metadata: map[string]any{
			"display_id": parsed.DisplayID,
			"path":       filepath.ToSlash(parsed.Path),
		},
	}, nil
}

func runPowerShellJSON(ctx context.Context, script string, args ...string) ([]byte, string, error) {
	commandArgs := []string{"-NoProfile", "-NonInteractive", "-Command", script}
	commandArgs = append(commandArgs, args...)

	cmd := exec.CommandContext(ctx, "powershell", commandArgs...)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, stderr.String(), context.DeadlineExceeded
		}

		return nil, stderr.String(), fmt.Errorf("powershell screenshot capture failed: %w", err)
	}

	return []byte(stdout.String()), stderr.String(), nil
}

func decodeArguments[T any](arguments map[string]any) (T, error) {
	if arguments == nil {
		var zero T
		return zero, tools.NewValidationError("tool arguments are required", nil)
	}

	payload, err := json.Marshal(arguments)
	if err != nil {
		var zero T
		return zero, tools.NewValidationError("tool arguments could not be decoded", nil)
	}

	var decoded T
	if err := json.Unmarshal(payload, &decoded); err != nil {
		var zero T
		return zero, tools.NewValidationError("tool arguments could not be decoded", nil)
	}

	return decoded, nil
}

func newCaptureID() (string, error) {
	var payload [12]byte
	if _, err := rand.Read(payload[:]); err != nil {
		return "", err
	}

	return "cap_" + hex.EncodeToString(payload[:]), nil
}
