package xmlrpc

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	// maxErrorBodyBytes caps how much of a non-2xx body ends up in the error
	maxErrorBodyBytes = 512
	// maxDrainBytes caps how much is read to allow connection reuse
	maxDrainBytes = 64 << 10
)

// Client implements a basic XMLRPC client
type Client struct {
	addr       string
	httpClient *http.Client

	BasicUser string
	BasicPass string

	log *log.Logger
}

type Config struct {
	Addr          string
	TLSSkipVerify bool

	BasicUser string
	BasicPass string

	Log *log.Logger

	Client *http.Client
}

// NewClient returns a new instance of Client
func NewClient(cfg Config) *Client {
	c := &Client{
		addr:      cfg.Addr,
		BasicUser: cfg.BasicUser,
		BasicPass: cfg.BasicPass,
		log:       log.New(io.Discard, "", log.LstdFlags),
	}
	transport := &http.Transport{}
	if cfg.TLSSkipVerify {
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	c.httpClient = &http.Client{Transport: transport, Timeout: 60 * time.Second}

	if cfg.Client != nil {
		c.httpClient = cfg.Client
	}

	// override logger if we pass one
	if cfg.Log != nil {
		c.log = cfg.Log
	}

	return c
}

// NewClientWithHTTPClient returns a new instance of Client.
// This allows you to use a custom http.Client setup for your needs.
func NewClientWithHTTPClient(addr string, client *http.Client) *Client {
	return &Client{
		addr:       addr,
		httpClient: client,
	}
}

// Call calls the method with "name" with the given args
// Returns the result, and an error for communication errors
func (c *Client) Call(ctx context.Context, name string, args ...interface{}) (interface{}, error) {
	data := bytes.NewBuffer(nil)
	if err := Marshal(data, name, args...); err != nil {
		return nil, errors.Wrap(err, "failed to marshal request")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.addr, data)
	if err != nil {
		return nil, errors.Wrap(err, "creating request failed")
	}

	req.Header.Set("Content-Type", "text/xml")

	c.addBasicAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "POST failed")
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))
		resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		if snippet := strings.TrimSpace(string(body)); snippet != "" {
			return nil, errors.Errorf("unexpected status: %s: %s", resp.Status, snippet)
		}
		return nil, errors.Errorf("unexpected status: %s", resp.Status)
	}

	_, val, fault, err := Unmarshal(resp.Body)
	if fault != nil {
		err = errors.Errorf("Error: %v: %v", err, fault)
	}
	return val, err
}

func (c *Client) addBasicAuth(req *http.Request) {
	if c.BasicUser != "" && c.BasicPass != "" {
		req.SetBasicAuth(c.BasicUser, c.BasicPass)
	}
}
