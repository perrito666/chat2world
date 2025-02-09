package im

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
)

// ErrFlowFinished should be returned by any Flow method to indicate the Flow is done, users of the Flow should handle
// it and remove the Flow as the current one.
var ErrFlowFinished = errors.New("flow finished")

// Flow is a state of a chat, only one Flow at a time can be enabled and all messages will be processed by it.
type Flow interface {
	// Start initializes the Flow
	Start(ctx context.Context, message *Message, messenger Messenger) error
	// HandleMessage should know how to handle any message happening during this Flow.
	HandleMessage(ctx context.Context, message *Message, messenger Messenger) error
	// StartCommandParser implements CommandParser for the start command of the Flow.
	StartCommandParser(string) (string, []string, error)
}

// FlowScheduler is a struct that holds a map of Flows and a map of commands that start each Flow, it will handle
// messages and route them to the correct Flow.
type FlowScheduler struct {
	flows                  map[string]Flow
	flowCommandEntryPoints map[string]string
	currentFlow            string
}

// NewScheduler creates a new FlowScheduler.
func NewScheduler() *FlowScheduler {
	return &FlowScheduler{
		flows:                  make(map[string]Flow),
		flowCommandEntryPoints: make(map[string]string),
		currentFlow:            "",
	}
}

// SchedulerFactoryFN describes a function capable of building a FlowScheduler with registered Flows.
type SchedulerFactoryFN func(userID uint64) (*FlowScheduler, error)

// ErrFlowTriggerConflict is returned when a command is already registered as a trigger for a Flow.
var ErrFlowTriggerConflict = errors.New("flow trigger conflict")

// ErrFlowAlreadyRegistered is returned when a Flow is already registered.
var ErrFlowAlreadyRegistered = errors.New("flow already registered")

// RegisterFlow will take a Flow and a list of commands (with leading /) that initiate the Flow.
func (fs *FlowScheduler) RegisterFlow(f Flow, name string, commands []string) error {
	if _, ok := fs.flows[name]; ok {
		return fmt.Errorf("%s: %w", name, ErrFlowAlreadyRegistered)
	}

	for _, c := range commands {
		if _, ok := fs.flowCommandEntryPoints[c]; ok {
			return fmt.Errorf("%s: %w", c, ErrFlowTriggerConflict)
		}
		fs.flowCommandEntryPoints[c] = name
	}
	fs.flows[name] = f
	return nil
}

// ErrEmptyMessage is returned when a message is empty.
var ErrEmptyMessage = errors.New("empty message")

// messageToFlowParams will take a message that starts with a / command (such as /new) and will return the command
// and a slice of other words in the message which are assumed to be params.
func messageToFlowParams(message string) (string, []string, error) {
	parts := strings.Split(message, " ")
	if len(parts) == 0 {
		return "", nil, ErrEmptyMessage
	}
	return parts[0], parts[1:], nil
}

// HandleMessage will receive a message and either pas it to the active handler's HandleMessage or,if no active handler
// is found, will use the command to Flow map to set a current one and invoke start on it with the same message
func (fs *FlowScheduler) HandleMessage(ctx context.Context, message *Message, messenger Messenger) error {
	log.Printf("when entering handler, current Flow is: %s", fs.currentFlow)
	defer log.Printf("when exiting handler, current Flow is: %s", fs.currentFlow)

	// We have a running flow, let it handle the message
	if fs.currentFlow != "" {
		if err := fs.flows[fs.currentFlow].HandleMessage(ctx, message, messenger); err != nil {
			if errors.Is(err, ErrFlowFinished) {
				fs.currentFlow = ""
				return nil
			}
			return fmt.Errorf("handling message: %w", err)
		}
		return nil
	}

	// We do not, let's see if this is a trigger for a flow
	if !message.IsCommand() {
		return nil
	}

	command, _, err := message.AsCommand(nil)
	if err != nil {
		if errors.Is(err, ErrNotACommand) {
			log.Printf("message is not a command we know how to handle: %s", message.Text)
			return nil
		}
		return fmt.Errorf("parsing message: %w", err)
	}

	log.Printf("telegram handle message: command: %s", command)
	if flowName, ok := fs.flowCommandEntryPoints[command]; ok {
		fs.currentFlow = flowName
		log.Printf("telegram handle message: starting Flow: %s", flowName)
		return fs.flows[flowName].Start(ctx, message, messenger)
	}
	fmt.Printf("telegram handle message: command not recognized: %s", command)
	return nil
}
