package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/puremachinery/gemini-cli-go/internal/llm"
)

type requestOptions struct {
	ctx       context.Context
	client    *http.Client
	url       string
	body      []byte
	accept    string
	headers   map[string]string
	auth      func(*http.Request)
	errPrefix string
}

func doJSONRequest(opts requestOptions) (*http.Response, error) {
	if opts.client == nil {
		return nil, errors.New("http client is nil")
	}
	accept := opts.accept
	if accept == "" {
		accept = "application/json"
	}
	buildReq := func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(opts.ctx, http.MethodPost, opts.url, bytes.NewReader(opts.body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", accept)
		for key, value := range opts.headers {
			httpReq.Header.Set(key, value)
		}
		if opts.auth != nil {
			opts.auth(httpReq)
		}
		return httpReq, nil
	}
	resp, err := doRequestWithRetry(opts.ctx, opts.client, buildReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, readErr
		}
		closeErr := resp.Body.Close()
		httpErr := formatHTTPError(opts.errPrefix, resp.Status, resp.StatusCode, payload)
		if closeErr != nil {
			return nil, fmt.Errorf("%w (also failed to close response body: %v)", httpErr, closeErr)
		}
		return nil, httpErr
	}
	return resp, nil
}

func doStreamRequest(opts requestOptions, name string, parse func([]byte) (llm.ChatChunk, error)) (Stream, error) {
	resp, err := doJSONRequest(opts)
	if err != nil {
		return nil, err
	}
	return newSSEStream(resp.Body, parse, name), nil
}

func decodeJSONResponse(resp *http.Response, out any) (err error) {
	if resp == nil {
		return errors.New("response is nil")
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	return json.NewDecoder(resp.Body).Decode(out)
}
