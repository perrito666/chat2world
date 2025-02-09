package telegram

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/perrito666/chat2world/blogging"
	"github.com/perrito666/chat2world/config"
)

// PostingFlow is a struct that represents the flow of posting a message to one or several blogging platforms
type PostingFlow struct {
	postsMutex sync.Mutex
	posts      map[int64]*blogging.MicroblogPost
	// Ill mix authed and non authed platforms here for now, I would expect user to auth
	platforms map[config.AvailableBloggingPlatform]blogging.AuthedPlatform
}

func (p *PostingFlow) Start(ctx context.Context, b *bot.Bot, u *models.Update) error {
	return p.HandleMessage(ctx, b, u)
}

func (p *PostingFlow) HandleMessage(ctx context.Context, b *bot.Bot, u *models.Update) error {
	command, _, err := messageToFlowParams(u.Message.Text)
	if err != nil {
		return err
	}

	switch command {
	case "/new":
		p.newCommandHandler(ctx, b, u)
	case "/send":
		p.sendCommandHandler(ctx, b, u)
	case "/cancel":
		p.cancelCommandHandler(ctx, b, u)
	default:
		p.defaultHandler(ctx, b, u)
	}

	return nil
}

// newCommandHandler starts a new post (i.e. enters the writing state).
func (p *PostingFlow) newCommandHandler(ctx context.Context, b *bot.Bot, u *models.Update) {
	chatID := u.Message.Chat.ID

	p.postsMutex.Lock()
	defer p.postsMutex.Unlock()

	if _, exists := p.posts[chatID]; exists {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "You already have an active post. Use /send to post it or /cancel to discard it.",
		})
		if err != nil {
			log.Printf("telegram send message err: %v", err)
		}
		return
	}

	p.posts[chatID] = &blogging.MicroblogPost{}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "Started a new post. Now send text or images to add content. Use /send when ready or /cancel to discard.",
	})
	if err != nil {
		log.Printf("telegram send message err: %v", err)
	}
}

// sendCommandHandler simulates sending the pending post.
func (p *PostingFlow) sendCommandHandler(ctx context.Context, b *bot.Bot, u *models.Update) {
	chatID := u.Message.Chat.ID

	p.postsMutex.Lock()
	post, exists := p.posts[chatID]
	if exists {
		delete(p.posts, chatID)
	}
	p.postsMutex.Unlock()

	if !exists {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "No active post to send. Use /new to start a post.",
		})
		if err != nil {
			log.Printf("telegram send message err: %v", err)
		}
		return
	}

	// Here you would integrate with Mastodon.
	log.Printf("Sending post for chat %d: %+v", chatID, post)
	for pname, platform := range p.platforms {
		postURL, err := platform.Post(ctx, blogging.ChatID(chatID), post)
		if err != nil {
			log.Printf("posting failed: %v", err)
			_, terr := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   fmt.Sprintf("Post Not sent to %s: %v", pname, err),
			})
			if terr != nil {
				log.Printf("telegram send message err: %v", err)
			}
			continue
		}
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("Post sent to %s (%s) : %v", pname, postURL, err),
		})
		if err != nil {
			log.Printf("telegram send message err: %v", err)
		}
	}

}

// cancelCommandHandler discards the pending post.
func (p *PostingFlow) cancelCommandHandler(ctx context.Context, b *bot.Bot, u *models.Update) {
	chatID := u.Message.Chat.ID

	p.postsMutex.Lock()
	_, exists := p.posts[chatID]
	if exists {
		delete(p.posts, chatID)
	}
	p.postsMutex.Unlock()

	var response string
	if exists {
		response = "Post canceled."
	} else {
		response = "No active post to cancel."
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   response,
	})
	if err != nil {
		log.Printf("telegram send message err: %v", err)
	}
}

const apiTelegramFileURL = "https://api.telegram.org/file"

func (p *PostingFlow) getFileContents(ctx context.Context, b *bot.Bot, fileID string) ([]byte, error) {
	fLink, err := b.GetFile(ctx, &bot.GetFileParams{
		FileID: fileID,
	})
	if err != nil {
		return nil, fmt.Errorf("telegram get photo file: %w", err)
	}
	if fLink.FilePath == "" {
		return nil, fmt.Errorf("telegram get photo file path is empty")
	}
	tgURI, _ := url.Parse(apiTelegramFileURL)
	tgURI = tgURI.JoinPath("bot"+b.Token(), fLink.FilePath)

	// https://api.telegram.org/file/bot<token>/<file_path>
	res, err := http.Get(tgURI.String())
	if err != nil {
		return nil, fmt.Errorf("telegram GET photo file: %w", err)
	}
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}

// defaultHandler processes any non-command (or unmatched) messages.
// If a chat is in "writing mode", the message content is appended to the post.
func (p *PostingFlow) defaultHandler(ctx context.Context, b *bot.Bot, u *models.Update) {
	if u.Message == nil {
		return
	}

	chatID := u.Message.Chat.ID

	p.postsMutex.Lock()
	post, active := p.posts[chatID]
	p.postsMutex.Unlock()

	if !active {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "No active post. Use /new to start writing a new post.",
		})
		return
	}

	added := false
	// Append text content.
	if u.Message.Text != "" {
		if len(post.Text) != 0 {
			post.Text += "\n"
		}
		post.Text += u.Message.Text
		added = true
	}

	log.Printf("Message Contents: %v", u.Message)
	// Append photo content (if any).
	if len(u.Message.Photo) > 0 {
		// Use the largest photo available (the last element).
		photo := u.Message.Photo[len(u.Message.Photo)-1]
		rawPhotoBytes, err := p.getFileContents(ctx, b, photo.FileID)
		if err != nil {
			log.Printf("telegram get file err: %v", err)
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   fmt.Sprintf("telegram get file err: %v", err),
			})
			return
		}

		post.AddImage(blogging.NewBlogImage(rawPhotoBytes, u.Message.Caption))
		added = true
	}

	if added {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Content added to your post.",
		})
	} else {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Received message, but no content was added.",
		})
	}
}

var _ flow = &PostingFlow{}

// NewPostingFlow creates a new PostingFlow
func NewPostingFlow(platforms map[config.AvailableBloggingPlatform]blogging.AuthedPlatform) *PostingFlow {
	return &PostingFlow{
		posts:     make(map[int64]*blogging.MicroblogPost),
		platforms: platforms,
	}
}
