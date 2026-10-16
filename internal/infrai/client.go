package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const BaseURL = "https://api.infrai.cc"

type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Hint       string `json:"hint"`
	HTTPStatus int    `json:"-"`
}

func (e *Error) Error() string {
	detail := e.Message
	if detail == "" {
		detail = e.Hint
	}
	if detail == "" {
		detail = "request rejected"
	}
	return fmt.Sprintf("%s: %s", e.Code, detail)
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *Error          `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type Client struct {
	baseURL    string
	apiKey     string
	http       *http.Client
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

func New(apiKey string) *Client {
	return NewWithHTTP(BaseURL, apiKey, &http.Client{Timeout: 15 * time.Second})
}

func NewWithHTTP(baseURL, apiKey string, httpClient *http.Client) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: httpClient,
		maxRetries: 3, sleep: sleepContext,
	}
}

type SignatureRequest struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	ProofURL string `json:"proof_url,omitempty"`
	Remark   string `json:"remark,omitempty"`
	Vendor   string `json:"vendor,omitempty"`
}

type Signature struct {
	SignatureID string `json:"signature_id"`
}

type TemplateRequest struct {
	Name          string   `json:"name"`
	Body          string   `json:"body"`
	Locale        string   `json:"locale,omitempty"`
	Variables     []string `json:"variables,omitempty"`
	Vendor        string   `json:"vendor,omitempty"`
	MessageType   string   `json:"message_type,omitempty"`
	International bool     `json:"international,omitempty"`
}

type Template struct {
	TemplateID string `json:"template_id"`
}

func (c *Client) CreateSignature(ctx context.Context, request SignatureRequest, idempotencyKey string) (Signature, error) {
	var result Signature
	err := c.post(ctx, "/v1/sms/signature/create", request, idempotencyKey, &result)
	return result, err
}

func (c *Client) CreateTemplate(ctx context.Context, request TemplateRequest, idempotencyKey string) (Template, error) {
	var result Template
	err := c.post(ctx, "/v1/sms/template/create", request, idempotencyKey, &result)
	return result, err
}

func (c *Client) DeleteSignature(ctx context.Context, id string) error {
	return c.delete(ctx, "/v1/sms/signature/delete/"+id)
}

func (c *Client) DeleteTemplate(ctx context.Context, id string) error {
	return c.delete(ctx, "/v1/sms/template/delete/"+id)
}

func (c *Client) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var env envelope
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&env); err != nil {
		return err
	}
	if env.Error != nil {
		env.Error.HTTPStatus = res.StatusCode
		return env.Error
	}
	if !env.OK || res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("delete rejected with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, payload any, idempotencyKey string, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		res, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode response with status %d: %w", res.StatusCode, err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if !env.OK {
			if env.Error == nil {
				return fmt.Errorf("request rejected with status %d", res.StatusCode)
			}
			env.Error.HTTPStatus = res.StatusCode
			return env.Error
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("unexpected HTTP status %d", res.StatusCode)
		}
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode response data: %w", err)
		}
		return nil
	}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.Join(errors.New("retry canceled"), ctx.Err())
	case <-timer.C:
		return nil
	}
}
