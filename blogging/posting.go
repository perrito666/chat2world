package blogging

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/perrito666/chat2world/config"
	"github.com/perrito666/chat2world/im"
)

// PostingFlow is a struct that represents the flow of posting a message to one or several blogging platforms
type PostingFlow struct {
	postsMutex sync.Mutex
	posts      map[uint64]*MicroblogPost
	// I'll mix authed and non authed platforms here for now, I would expect user to auth
	platforms map[config.AvailableBloggingPlatform]AuthedPlatform
}

// Start implements im.Flow and will start the posting flow by simply delegating to HandleMessage
func (p *PostingFlow) Start(ctx context.Context, message *im.Message, messenger im.Messenger) error {
	return p.HandleMessage(ctx, message, messenger)
}

// StartCommandParser implements im.Flow and will do a simple split.
func (p *PostingFlow) StartCommandParser(s string) (string, []string, error) {
	parts := strings.Split(s, " ")
	if len(parts) < 1 {
		return "", nil, im.ErrNotACommand
	}
	return parts[0], parts[1:], nil
}

func (p *PostingFlow) HandleMessage(ctx context.Context, message *im.Message, messenger im.Messenger) error {
	if !message.IsCommand() {
		err := p.defaultHandler(ctx, message, messenger)
		if err != nil {
			return fmt.Errorf("default handler: %w", err)
		}
		return nil
	}
	command, _, err := message.AsCommand(p.StartCommandParser)
	if err != nil {
		return fmt.Errorf("parsing message (%s): %w", message.Text, err)
	}

	switch command {
	case "/new":
		return p.newCommandHandler(ctx, message, messenger)
	case "/send":
		return p.sendCommandHandler(ctx, message, messenger)
	case "/cancel":
		return p.cancelCommandHandler(ctx, message, messenger)

	}

	return p.defaultHandler(ctx, message, messenger)
}

// argsIntoMaps receives the string slice obtained from AsCommand for the arguments and returns a map of the arguments
// which are of the form key=value. It also returns a slice of the arguments that are not in the form key=value.
func argsIntoMaps(args []string) (map[string]string, []string) {
	argMap := make(map[string]string)
	var remainingArgs []string
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			argMap[parts[0]] = parts[1]
		} else {
			remainingArgs = append(remainingArgs, arg)
		}
	}
	return argMap, remainingArgs
}

// newCommandHandler starts a new post (i.e. enters the writing state).
func (p *PostingFlow) newCommandHandler(ctx context.Context, message *im.Message, messenger im.Messenger) error {
	userID := message.UserID
	_, args, err := message.AsCommand(p.StartCommandParser)
	if err != nil {
		return fmt.Errorf("parsing /new message (%s): %w", message.Text, err)
	}

	kv, positional := argsIntoMaps(args)

	var langs []string
	if lang, ok := kv["langs"]; ok {
		langs = strings.Split(lang, ",")
	} else if len(positional) > 0 {
		langs = strings.Split(positional[0], ",")
	}

	p.postsMutex.Lock()
	defer p.postsMutex.Unlock()

	if _, exists := p.posts[userID]; exists {
		err := messenger.SendMessage(ctx, message.Reply("You already have an active post. Use /send to post it or /cancel to discard it."))
		if err != nil {
			log.Printf("messenger send message err: %v", err)
			return fmt.Errorf("messenger send message err: %w", err)
		}
		// Already have an active post, not a showstopper
		return nil
	}

	p.posts[userID] = &MicroblogPost{
		Langs: langs,
	}
	err = messenger.SendMessage(ctx, message.Reply("Started a new post. Now send text or images to add content. Use /send when ready or /cancel to discard."))
	if err != nil {
		log.Printf("messenger send message err: %v", err)
		return fmt.Errorf("messenger send message err: %w", err)
	}
	return nil
}

// sendCommandHandler sends the message to mastodon
func (p *PostingFlow) sendCommandHandler(ctx context.Context, message *im.Message, messenger im.Messenger) error {
	userID := message.UserID

	p.postsMutex.Lock()
	post, exists := p.posts[userID]
	if exists {
		delete(p.posts, userID)
	}
	p.postsMutex.Unlock()

	if !exists {
		err := messenger.SendMessage(ctx, message.Reply("No active post to send. Use /new to start a post."))
		if err != nil {
			log.Printf("messenger send message err: %v", err)
			return fmt.Errorf("messenger send message err: %w", err)
		}
		return nil
	}

	// Here you would integrate with Mastodon.
	log.Printf("Sending post for chat %d: %+v", userID, post)
	var postErrs []error
	for pname, platform := range p.platforms {
		postURL, err := platform.Post(ctx, UserID(userID), post)
		if err != nil {
			log.Printf("posting failed: %v", err)
			terr := messenger.SendMessage(ctx, message.Reply(fmt.Sprintf("Post Not sent to %s: %v", pname, err)))
			if terr != nil {
				log.Printf("messenger send message err: %v", err)
				postErrs = append(postErrs, terr)
			}
			continue
		}
		err = messenger.SendMessage(ctx, message.Reply(fmt.Sprintf("Post sent to %s (%s)", pname, postURL)))
		if err != nil {
			log.Printf("messenger send message err: %v", err)
		}
	}
	if len(postErrs) > 0 {
		return fmt.Errorf("posting errors: %v", errors.Join(postErrs...))
	}
	return nil
}

// cancelCommandHandler discards the pending post.
func (p *PostingFlow) cancelCommandHandler(ctx context.Context, message *im.Message, messenger im.Messenger) error {
	userID := message.UserID

	p.postsMutex.Lock()
	_, exists := p.posts[userID]
	if exists {
		delete(p.posts, userID)
	}
	p.postsMutex.Unlock()

	var response string
	if exists {
		response = "Post canceled."
	} else {
		response = "No active post to cancel."
	}
	err := messenger.SendMessage(ctx, message.Reply(response))
	if err != nil {
		log.Printf("messenger send message err: %v", err)
		return fmt.Errorf("messenger send message err: %w", err)
	}
	return nil
}

// defaultHandler processes any non-command (or unmatched) messages.
// If a chat is in "writing mode", the message content is appended to the post.
func (p *PostingFlow) defaultHandler(ctx context.Context, message *im.Message, messenger im.Messenger) error {
	if message.IsEmpty() {
		return nil
	}

	userID := message.UserID

	p.postsMutex.Lock()
	post, active := p.posts[userID]
	p.postsMutex.Unlock()

	if !active {
		err := messenger.SendMessage(ctx, message.Reply("No active post. Use /new to start writing a new post."))
		if err != nil {
			return fmt.Errorf("messenger, sending no active post message: %w", err)
		}
		return nil
	}

	added := false
	// Append text content.
	if message.Text != "" {
		if len(post.Text) != 0 {
			post.Text += "\n"
		}
		post.Text += message.Text
		added = true
	}

	for _, img := range message.Images {
		post.AddImage(NewBlogImage(img.Data, img.Caption))
		added = true
	}

	var err error
	if added {
		err = messenger.SendMessage(ctx, message.Reply("Content added to your post"))
	} else {
		err = messenger.SendMessage(ctx, message.Reply("Received message, but no content was added."))
	}
	if err != nil {
		return fmt.Errorf("responding after content add: %w", err)
	}
	return nil
}

var _ im.Flow = (*PostingFlow)(nil)

// NewPostingFlow creates a new PostingFlow
func NewPostingFlow(platforms map[config.AvailableBloggingPlatform]AuthedPlatform) *PostingFlow {
	return &PostingFlow{
		posts:     make(map[uint64]*MicroblogPost),
		platforms: platforms,
	}
}
