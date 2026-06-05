package vega

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// Client is a client for the Einstar Vega API.
type Client struct {
	addr       string
	httpClient *http.Client
}

// NewClient creates a new Client.
func NewClient(addr string) *Client {
	return &Client{
		addr: addr,
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // Timeout for general requests
		},
	}
}

// Connect verifies connection to the device.
func (c *Client) Connect(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s:8080/connect", c.addr), nil)
	if err != nil {
		return fmt.Errorf("creating connect request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to device: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 15))
	if err != nil {
		return fmt.Errorf("reading connect response: %w", err)
	}

	if string(data) != "connect success" {
		return fmt.Errorf("unexpected connect response: %s", string(data))
	}

	return nil
}

// DeviceInfo represents device information returned by the API.
type DeviceInfo map[string]interface{}

// GetDeviceInfo retrieves device information.
func (c *Client) GetDeviceInfo(ctx context.Context) (DeviceInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s:8080/request/deviceInfo", c.addr), nil)
	if err != nil {
		return nil, fmt.Errorf("creating device info request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting device info: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var data struct {
		Result DeviceInfo `cbor:"result"`
	}
	if err := cbor.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding device info: %w", err)
	}

	return data.Result, nil
}

// ProjectInfo represents information about a project.
type ProjectInfo struct {
	Path     string `cbor:"Path"`
	DateTime string `cbor:"DateTime"`
}

// GetProjectsInfo retrieves all projects from the device.
func (c *Client) GetProjectsInfo(ctx context.Context) (map[string]ProjectInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s:8080/request/projectsInfo", c.addr), nil)
	if err != nil {
		return nil, fmt.Errorf("creating projects info request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting projects info: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var data struct {
		Result map[string]ProjectInfo `cbor:"result"`
	}
	if err := cbor.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding projects info: %w", err)
	}

	return data.Result, nil
}

// ListFilePaths retrieves files under a given path.
func (c *Client) ListFilePaths(ctx context.Context, path string) ([]string, error) {
	payload := map[string]string{"path": path}
	b, err := cbor.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling list paths request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://%s:8080/request/listFilePaths", c.addr), bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("creating list paths request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing file paths: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var data struct {
		Result []interface{} `cbor:"result"`
	}
	if err := cbor.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding list paths response: %w", err)
	}

	if len(data.Result) < 2 {
		return nil, fmt.Errorf("unexpected list paths response format")
	}

	pathsRaw, ok := data.Result[1].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected list paths paths format")
	}

	var paths []string
	for _, p := range pathsRaw {
		if str, ok := p.(string); ok {
			paths = append(paths, str)
		}
	}

	return paths, nil
}

// GetFileInfo retrieves size information for a file.
func (c *Client) GetFileInfo(ctx context.Context, path string) (uint64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://%s:8080/file/info", c.addr), bytes.NewReader([]byte(path)))
	if err != nil {
		return 0, fmt.Errorf("creating file info request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("getting file info: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var size uint64
	if err := binary.Read(resp.Body, binary.BigEndian, &size); err != nil {
		return 0, fmt.Errorf("reading file size: %w", err)
	}

	return size, nil
}

// DownloadFile downloads a file to a writer.
// It bypasses the default HTTP client timeout since files can be large.
func (c *Client) DownloadFile(ctx context.Context, path string, size uint64, w io.Writer, progressFn func(downloaded int64, total int64)) error {
	payload := map[string]interface{}{
		"path": path,
		"pos":  0,
		"size": size,
	}
	b, err := cbor.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling download request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://%s:8080/file/content", c.addr), bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("creating download request: %w", err)
	}

	// Use a client without timeout for downloads
	dlClient := &http.Client{}
	resp, err := dlClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading file: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code downloading file: %d", resp.StatusCode)
	}

	buf := make([]byte, 32*1024)
	var downloaded int64
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return fmt.Errorf("writing file content: %w", werr)
			}
			downloaded += int64(n)
			if progressFn != nil {
				progressFn(downloaded, int64(size))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading file content: %w", err)
		}
	}

	return nil
}
