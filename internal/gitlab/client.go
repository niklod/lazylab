package gitlab

import (
	"context"
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

// InvalidateMR drops every cached entry for the given MR so the next fetch
// hits upstream. Safe when the client was built without WithCache — the no-op
// branch keeps callers from having to know the cache exists.
func (c *Client) InvalidateMR(projectID, mrIID int) {
	if c.cache == nil {
		return
	}
	c.cache.InvalidateMR(projectID, mrIID)
}

// doCached wraps the `if c.cache == nil { pass-through } else { cache.Do }`
// boilerplate every read method needs. The `label` is the human-readable op
// name used in the wrapped cache error — e.g. "list merge requests" produces
// `gitlab: cached list merge requests: <loader error>`.
func doCached[T any](
	ctx context.Context,
	c *Client,
	namespace, label string,
	loader func(context.Context) (T, error),
	args ...any,
) (T, error) {
	if c.cache == nil {
		return loader(ctx)
	}
	v, err := cache.Do(ctx, c.cache, namespace, loader, args...)
	if err != nil {
		var zero T

		return zero, fmt.Errorf("gitlab: cached %s: %w", label, err)
	}

	return v, nil
}
