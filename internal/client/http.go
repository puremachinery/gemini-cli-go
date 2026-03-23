package client

import (
	"net/http"
	"time"
)

const defaultHTTPTimeout = 5 * time.Minute

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: defaultHTTPTimeout}
}
