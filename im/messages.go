package im

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/perrito666/chat2world/config"
)

// Image holds the data and caption of an image as we send it to chats and we receive it from them.
type Image struct {
	Data    []byte
	Caption string
}

// Message holds the kind of messages we send and receive from chats.
type Message struct {
	IM        config.AvailableIM
	ChatID    int64
	UserID    uint64
	MsgID     uint64
	InReplyTo uint64

	Text   string
	Images []*Image
}

// Reply takes a new text and images and returns a new message replying to the original message.
func (m *Message) Reply(text string, images ...*Image) *Message {
	return &Message{
		IM:        m.IM,
		ChatID:    m.ChatID,
		UserID:    m.UserID,
		InReplyTo: m.MsgID,
		Text:      text,
		Images:    images,
	}
}

// IsCommand returns true if the message is a command, a command is a message that starts with a /
func (m *Message) IsCommand() bool {
	return len(m.Text) > 0 && m.Text[0] == '/'
}

// IsEmpty returns true if the message is empty
func (m *Message) IsEmpty() bool {
	return m.Text == "" && len(m.Images) == 0
}

// CommandParser represents a function that knows how to parse a given command.
type CommandParser func(string) (string, []string, error)

// ErrNotACommand is returned when a message is not a command
var ErrNotACommand = errors.New("not a command")

// AsCommand returns the command part of a message split and any other args as params
func (m *Message) AsCommand(parser CommandParser) (string, []string, error) {
	if !m.IsCommand() {
		return "", nil, fmt.Errorf("%s: %w", m.Text, ErrNotACommand)
	}
	if parser == nil {
		parts := strings.Split(m.Text, " ")
		if len(parts) == 0 {
			return "", nil, fmt.Errorf("%s: %w", m.Text, ErrNotACommand)
		}
		return parts[0], parts[1:], nil
	}
	return parser(m.Text)
}

// Messenger is the interface that wraps the SendMessage method in an agnostic version
// it was modeled after Telegram's send message, but hopefully we can adapt others..
type Messenger interface {
	SendMessage(ctx context.Context, message *Message) error
	Name() string
}
