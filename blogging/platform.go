package blogging

import "context"

type Platform interface {
	Post(ctx context.Context, chatID ChatID, post *MicroblogPost) error
	Config(chatID ChatID) (ClientConfig, error)
}

type AuthedPlatform interface {
	Platform
	Authorizer
}
