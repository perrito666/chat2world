package blogging

import "context"

type Platform interface {
	Post(ctx context.Context, userID UserID, post *MicroblogPost) (string, error)
	Config(userID UserID) (ClientConfig, error)
}

type AuthedPlatform interface {
	Platform
	Authorizer
}
