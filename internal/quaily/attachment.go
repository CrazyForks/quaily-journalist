package quaily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type attachmentResponse struct {
	Data struct {
		ViewURL string `json:"view_url"`
	} `json:"data"`
}

// UploadAttachment uploads a file to Quaily and returns the hosted view URL.
func (c *Client) UploadAttachment(ctx context.Context, filePath string, encrypted bool) (string, error) {
	if c == nil {
		return "", errors.New("nil quaily client")
	}
	if strings.TrimSpace(filePath) == "" {
		return "", errors.New("empty file path")
	}
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open attachment: %w", err)
	}
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", fmt.Errorf("write form file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	url := c.baseURL + "/attachments?encrypted=" + strconv.FormatBool(encrypted)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload attachment failed: status=%d body=%s", resp.StatusCode, string(b))
	}
	var out attachmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode attachment response: %w", err)
	}
	if strings.TrimSpace(out.Data.ViewURL) == "" {
		return "", errors.New("attachment response missing view_url")
	}
	return out.Data.ViewURL, nil
}
