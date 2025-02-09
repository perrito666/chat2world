package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/perrito666/chat2world/blogging"
)

// AuthorizerFlow implements flow for authorizing a blogging account through a telegram bot, instantiate them with
// the Authorizer you want to use for each platform.
type AuthorizerFlow struct {
	authorizer        blogging.Authorizer
	authorizationChan chan string
}

func (a *AuthorizerFlow) Start(ctx context.Context, b *bot.Bot, u *models.Update) error {
	authorization, err := a.authorizer.StartAuthorization(ctx, blogging.ChatID(u.Message.Chat.ID), nil)
	if err != nil {
		return fmt.Errorf("starting authorization: %w", err)
	}
	log.Printf("telegram authorizer: started authorization for chat: %d", u.Message.Chat.ID)
	a.authorizationChan = authorization
	// we invoke handle message because the flow begins with us responding to a message
	return a.HandleMessage(ctx, b, u)
}

// HandleMessage will handle messages during the authorization flow, messages will be sent and received through the
// authorization channel, the flow will finish when the channel is closed. The flow begins with us listening for the
// first message on the channel. If the message we are handling is not empty AND the channel is not closed, we send the
// message to the chat before anything. If the channel is closed, we return an error to indicate the flow is finished.
func (a *AuthorizerFlow) HandleMessage(ctx context.Context, b *bot.Bot, u *models.Update) error {
	if a.authorizationChan == nil {
		return fmt.Errorf("no authorization channel")
	}
	// extract the message to be sent through the channel if not a command
	if !strings.HasPrefix(u.Message.Text, "/") && u.Message.Text != "" {
		log.Printf("telegram authorizer: sending message from chat ID %d: %s", u.Message.Chat.ID, u.Message.Text)
		select {
		case a.authorizationChan <- u.Message.Text:
		}
	}
	log.Printf("telegram authorizer: handling message from chat ID %d: %s", u.Message.Chat.ID, u.Message.Text)

	select {
	case msg, ok := <-a.authorizationChan:
		if !ok {
			log.Printf("telegram authorizer: finished authorization for chat ID %d", u.Message.Chat.ID)
			return ErrFlowFinished
		}
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: u.Message.Chat.ID,
			Text:   msg,
		})
		if err != nil {
			return fmt.Errorf("sending message: %w", err)
		}
		return nil
	}
}

var _ flow = &AuthorizerFlow{}

// NewMastodonAuthorizerFlow creates a new AuthorizerFlow for Mastodon.
func NewMastodonAuthorizerFlow(authorizer blogging.Authorizer) *AuthorizerFlow {
	return &AuthorizerFlow{
		authorizer: authorizer,
	}
}
