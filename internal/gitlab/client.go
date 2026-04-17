package gitlab

import (
	"errors"
	"fmt"
	"net/http"

	gogitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/niklod/lazylab/internal/config"
)

var (
	ErrMissingURL   = errors.New("gitlab: url is required")
	ErrMissingToken = errors.New("gitlab: token is required")
)

type Client struct {
	api *gogitlab.Client
}

type Option func(*options)

type options struct {
	httpClient *http.Client
}

func WithHTTPClient(c *http.Client) Option {
	return func(o *options) { o.httpClient = c }
}

func New(cfg config.GitLabConfig, opts ...Option) (*Client, error) {
	if cfg.URL == "" {
		return nil, ErrMissingURL
	}
	if cfg.Token == "" {
		return nil, ErrMissingToken
	}

	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	apiOpts := []gogitlab.ClientOptionFunc{gogitlab.WithBaseURL(cfg.URL)}
	if o.httpClient != nil {
		apiOpts = append(apiOpts, gogitlab.WithHTTPClient(o.httpClient))
	}

	api, err := gogitlab.NewClient(cfg.Token, apiOpts...)
	if err != nil {
		return nil, fmt.Errorf("gitlab: new client: %w", err)
	}
	return &Client{api: api}, nil
}

func (c *Client) API() *gogitlab.Client {
	return c.api
}
