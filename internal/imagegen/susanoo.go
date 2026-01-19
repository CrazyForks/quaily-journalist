package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chai2010/webp"
)

// Generator defines the interface for cover image generation.
type Generator interface {
	GenerateCover(ctx context.Context, prompt, outPath string) error
}

// SusanooConfig holds configuration for the Susanoo image API.
type SusanooConfig struct {
	BaseURL     string
	APIKey      string
	Model       string
	AspectRatio string
	Timeout     time.Duration
	WebPQuality int
}

// Susanoo implements Generator using Susanoo image generation.
type Susanoo struct {
	baseURL     string
	apiKey      string
	model       string
	aspectRatio string
	timeout     time.Duration
	webPQuality int
	httpClient  *http.Client
}

// NewSusanoo creates a Susanoo client from config. Returns nil if essential config is missing.
func NewSusanoo(cfg SusanooConfig) (*Susanoo, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" {
		return nil, nil
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gemini-2.5-flash"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	quality := cfg.WebPQuality
	if quality <= 0 || quality > 100 {
		quality = 85
	}
	return &Susanoo{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:      cfg.APIKey,
		model:       model,
		aspectRatio: strings.TrimSpace(cfg.AspectRatio),
		timeout:     timeout,
		webPQuality: quality,
		httpClient:  &http.Client{Timeout: timeout},
	}, nil
}

type imageGenerationRequest struct {
	Model    string         `json:"model"`
	Prompt   string         `json:"prompt"`
	N        int            `json:"n,omitempty"`
	Provider string         `json:"provider,omitempty"`
	Options  map[string]any `json:"gemini_options,omitempty"`
}

type imageGenerationResponse struct {
	Data struct {
		Error   string `json:"error"`
		Results []struct {
			B64JSON string `json:"b64_json"`
		} `json:"results"`
	} `json:"data"`
}

// GenerateCover generates an image from prompt and writes a WebP file to outPath.
func (s *Susanoo) GenerateCover(ctx context.Context, prompt, outPath string) error {
	if s == nil {
		return errors.New("nil susanoo client")
	}
	if strings.TrimSpace(prompt) == "" {
		return errors.New("prompt is empty")
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	slog.Info("susanoo: generating cover image",
		"model", s.model,
		"aspect_ratio", s.aspectRatio,
		"out_path", outPath,
	)

	body, err := json.Marshal(imageGenerationRequest{
		Model:    s.model,
		Prompt:   prompt,
		N:        1,
		Provider: "gemini",
		Options:  geminiOptions(s.aspectRatio),
	})
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	url := s.baseURL + "/images/generations?async=0"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SUSANOO-KEY", s.apiKey)

	reqStart := time.Now()
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("susanoo request: %w", err)
	}
	defer resp.Body.Close()
	slog.Info("susanoo: response received",
		"status", resp.StatusCode,
		"duration", time.Since(reqStart),
	)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("susanoo status=%d body=%s", resp.StatusCode, string(b))
	}
	var parsed imageGenerationResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if strings.TrimSpace(parsed.Data.Error) != "" {
		return fmt.Errorf("susanoo error: %s", parsed.Data.Error)
	}
	if len(parsed.Data.Results) == 0 || strings.TrimSpace(parsed.Data.Results[0].B64JSON) == "" {
		return errors.New("susanoo returned empty image data")
	}
	raw, err := base64.StdEncoding.DecodeString(parsed.Data.Results[0].B64JSON)
	if err != nil {
		return fmt.Errorf("decode base64 image: %w", err)
	}
	slog.Info("susanoo: image payload decoded", "bytes", len(raw))
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}
	bounds := img.Bounds()
	slog.Info("susanoo: image decoded",
		"width", bounds.Dx(),
		"height", bounds.Dy(),
	)

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create cover dir: %w", err)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create cover file: %w", err)
	}
	defer f.Close()

	slog.Info("susanoo: writing webp", "path", outPath, "quality", s.webPQuality)
	if err := webp.Encode(f, img, &webp.Options{Quality: float32(s.webPQuality)}); err != nil {
		return fmt.Errorf("encode webp: %w", err)
	}
	slog.Info("susanoo: cover image saved", "path", outPath, "duration", time.Since(start))
	return nil
}

func geminiOptions(aspectRatio string) map[string]any {
	aspectRatio = strings.TrimSpace(aspectRatio)
	if aspectRatio == "" {
		return nil
	}
	return map[string]any{
		"aspect_ratio": aspectRatio,
	}
}
