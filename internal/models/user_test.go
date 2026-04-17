package models_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
)

type UserSuite struct {
	suite.Suite
}

func (s *UserSuite) TestUser_JSONRoundTrip() {
	u := models.User{
		ID:        42,
		Username:  "alice",
		Name:      "Alice Doe",
		WebURL:    "https://gitlab.example/alice",
		AvatarURL: "https://gitlab.example/avatars/alice.png",
	}

	data, err := json.Marshal(u)
	s.Require().NoError(err)

	var decoded models.User
	s.Require().NoError(json.Unmarshal(data, &decoded))
	s.Require().Equal(u, decoded)
}

func (s *UserSuite) TestUser_Unmarshal_FromFixture() {
	payload := `{"id": 7, "username": "bob", "name": "Bob", "web_url": "https://example/bob"}`

	var u models.User
	s.Require().NoError(json.Unmarshal([]byte(payload), &u))

	s.Require().Equal(7, u.ID)
	s.Require().Equal("bob", u.Username)
	s.Require().Empty(u.AvatarURL)
}

func TestUserSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(UserSuite))
}
