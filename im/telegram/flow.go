package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// ErrFlowFinished should be returned by any flow method to indicate the flow is done, users of the flow should handle
// it and remove the flow as the current one.
var ErrFlowFinished = errors.New("flow finished")

// a flow is a state of a chat, only one flow at a time can be enabled and all messages will be processed by it.
type flow interface {
	// Start initializes the flow
	Start(ctx context.Context, b *bot.Bot, u *models.Update) error
	// HandleMessage should know how to handle any message happening during this flow.
	HandleMessage(ctx context.Context, b *bot.Bot, u *models.Update) error
}

type FlowScheduler struct {
	flows                  map[string]flow
	flowCommandEntryPoints map[string]string
	currentFlow            string
}

func NewScheduler() *FlowScheduler {
	return &FlowScheduler{
		flows:                  make(map[string]flow),
		flowCommandEntryPoints: make(map[string]string),
		currentFlow:            "",
	}
}

// registerFlow will take a flow and a list of commands (with leading /) that initiate the flow.
func (fs *FlowScheduler) registerFlow(f flow, name string, commands []string) {
	fs.flows[name] = f
	for _, c := range commands {
		fs.flowCommandEntryPoints[c] = name
	}
}

var ErrEmptyMessage = errors.New("empty message")

// message to flow params will take a message that starts with a / command (such as /new) and will return the command
// and a slice of other words in the message which are assumed to be params.
func messageToFlowParams(message string) (string, []string, error) {
	parts := strings.Split(message, " ")
	if len(parts) == 0 {
		return "", nil, ErrEmptyMessage
	}
	return parts[0], parts[1:], nil
}

// handleMessage will receive a message and either pas it to the active handler's HandleMessage or,if no active handler
// is found, will use the command to flow map to set a current one and invoke start on it with the same message
func (fs *FlowScheduler) handleMessage(ctx context.Context, b *bot.Bot, u *models.Update) error {
	log.Printf("when entering handler, current flow is: %s", fs.currentFlow)
	defer log.Printf("when exiting handler, current flow is: %s", fs.currentFlow)
	if fs.currentFlow != "" {
		if err := fs.flows[fs.currentFlow].HandleMessage(ctx, b, u); err != nil {
			if errors.Is(err, ErrFlowFinished) {
				fs.currentFlow = ""
				return nil
			}
			return fmt.Errorf("handling message: %w", err)
		}
		return nil
	}
	command, _, err := messageToFlowParams(u.Message.Text)
	if err != nil {
		return fmt.Errorf("parsing message: %w", err)
	}
	log.Printf("telegram handle message: command: %s", command)
	if flowName, ok := fs.flowCommandEntryPoints[command]; ok {
		fs.currentFlow = flowName
		log.Printf("telegram handle message: starting flow: %s", flowName)
		return fs.flows[flowName].Start(ctx, b, u)
	}
	return nil
}
