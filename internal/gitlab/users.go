package gitlab

import (
	"context"
	"fmt"

	gogitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/niklod/lazylab/internal/models"
)

const currentUserCacheNamespace = "current_user"

// GetCurrentUser fetches the authenticated user (used for author/reviewer
// filters). Cached as a singleton — the token identifies exactly one user.
func (c *Client) GetCurrentUser(ctx context.Context) (*models.User, error) {
	loader := func(ctx context.Context) (*models.User, error) {
		u, _, err := c.api.Users.CurrentUser(gogitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: current user: %w", err)
		}

		return toDomainUserFromFull(u), nil
	}

	return doCached(ctx, c, currentUserCacheNamespace, "current user", loader)
}

func toDomainUserFromFull(u *gogitlab.User) *models.User {
	if u == nil {
		return nil
	}

	return &models.User{
		ID:        int(u.ID),
		Username:  u.Username,
		Name:      u.Name,
		WebURL:    u.WebURL,
		AvatarURL: u.AvatarURL,
	}
}
