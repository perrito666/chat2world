package blogging

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/perrito666/chat2world/im"
)

// AuthorizerFlow implements flow for authorizing a blogging account through a telegram bot, instantiate them with
// the Authorizer you want to use for each platform.
type AuthorizerFlow struct {
	authorizer        Authorizer
	authorizationChan chan string
}

// StartCommandParser implements im.Flow and will do a simple split.
func (a *AuthorizerFlow) StartCommandParser(s string) (string, []string, error) {
	parts := strings.Split(s, " ")
	if len(parts) < 1 {
		return "", nil, im.ErrNotACommand
	}
	return parts[0], parts[1:], nil
}

// Start implements im.Flow and will start the authorization flow for the chat that sent the message.
func (a *AuthorizerFlow) Start(ctx context.Context, message *im.Message, messenger im.Messenger) error {
	authorization, err := a.authorizer.StartAuthorization(ctx, UserID(message.UserID), nil)
	if err != nil {
		return fmt.Errorf("starting authorization: %w", err)
	}
	log.Printf("telegram authorizer: started authorization for chat: %d", message.UserID)
	a.authorizationChan = authorization
	// we invoke handle message because the flow begins with us responding to a message
	return a.HandleMessage(ctx, message, messenger)
}

// HandleMessage will handle messages during the authorization flow, messages will be sent and received through the
// authorization channel, the flow will finish when the channel is closed. The flow begins with us listening for the
// first message on the channel. If the message we are handling is not empty AND the channel is not closed, we send the
// message to the chat before anything. If the channel is closed, we return an error to indicate the flow is finished.
func (a *AuthorizerFlow) HandleMessage(ctx context.Context, message *im.Message, messenger im.Messenger) error {
	if a.authorizationChan == nil {
		return fmt.Errorf("no authorization channel")
	}
	// extract the message to be sent through the channel if not a command
	if !message.IsCommand() && !message.IsEmpty() {
		log.Printf("%s authorizer: sending message from chat ID %d for user %d: %s", messenger.Name(), message.ChatID, message.UserID, message.Text)
		select {
		case a.authorizationChan <- message.Text:
		case <-ctx.Done():
			return nil
		}
	}
	log.Printf("%s authorizer: handling message from chat ID %d for user %d: %s", messenger.Name(), message.ChatID, message.UserID, message.Text)

	select {
	case msg, ok := <-a.authorizationChan:
		if !ok {
			log.Printf("%s authorizer: finished authorization for chat ID %d for user %d", messenger.Name(), message.ChatID, message.UserID)
			return im.ErrFlowFinished
		}
		err := messenger.SendMessage(ctx, message.Reply(msg))
		if err != nil {
			return fmt.Errorf("sending message: %w", err)
		}
		return nil
	case <-ctx.Done():
		return nil
	}
}

var _ im.Flow = &AuthorizerFlow{}

func NewAuthorizerFlow(authorizer Authorizer) *AuthorizerFlow {
	return &AuthorizerFlow{
		authorizer: authorizer,
	}
}
