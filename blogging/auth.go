package blogging

import "context"

type ChatID string

type Authorizer interface {
	IsAuthorized(id ChatID) bool
	// StartAuthorization should begin an authorization chat that is a convo via the chan
	// the im client is agnostic to it.
	StartAuthorization(ctx context.Context, id ChatID) (chan string, error)
}
type Authorization struct {
	registeredAuthorizationMechanisms map[string]Authorizer
}

func NewAuthorization() *Authorization {
	return &Authorization{
		registeredAuthorizationMechanisms: make(map[string]Authorizer),
	}
}

func (a *Authorization) RegisterAuthorizationMechanism(name string, auth Authorizer) {
	a.registeredAuthorizationMechanisms[name] = auth
}
