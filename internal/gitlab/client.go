package gitlab

import (
	"errors"
	"fmt"
	"net/http"

	gogitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/config"
)

var (
	ErrMissingURL   = errors.New("gitlab: url is required")
	ErrMissingToken = errors.New("gitlab: token is required")
)

// Client wraps the upstream gogitlab client with optional stale-while-revalidate
// caching (ADR 009). A nil cache means pass-through — convenient for tests that
// exercise the network path directly.
type Client struct {
	api   *gogitlab.Client
	cache *cache.Cache
}

type Option func(*options)

type options struct {
	httpClient *http.Client
	cache      *cache.Cache
}

func WithHTTPClient(c *http.Client) Option {
	return func(o *options) { o.httpClient = c }
}

// WithCache routes read-only calls (e.g. ListProjects) through cache.Do[T] so
// repeat calls hit the in-memory / disk cache and surface cached data
// immediately while refreshing in the background.
func WithCache(c *cache.Cache) Option {
	return func(o *options) { o.cache = c }
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

	return &Client{api: api, cache: o.cache}, nil
}

func (c *Client) API() *gogitlab.Client {
	return c.api
}
