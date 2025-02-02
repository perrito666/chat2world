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

// Bot wraps the underlying bot.Bot and holds state.
type Bot struct {
	bot                     *bot.Bot
	posts                   map[int64]*blogging.MicroblogPost // pending post per chat (by Chat.ID)
	bloggingPlatforms       map[config.AvailableBloggingPlatform]blogging.Platform
	authedBloggingPlatforms map[config.AvailableBloggingPlatform]blogging.AuthedPlatform
	postsMutex              sync.Mutex
	commands                map[string]bot.HandlerFunc

	// done channel communicates when the bot has stopped.
	done chan struct{}
}

// New creates a new Telegram bot instance.
// You can pass additional bot.Options if needed.
func New(ctx context.Context, token string, webhookSecret string, webhookURL *url.URL) (*Bot, error) {
	// Create the underlying bot.
	opt := bot.WithWebhookSecretToken(webhookSecret)
	b, err := bot.New(token, opt)
	if err != nil {
		return nil, err
	}

	tb := &Bot{
		bot:   b,
		posts: make(map[int64]*blogging.MicroblogPost),
		done:  make(chan struct{}),
	}
	// TODO: Add ways to only reply to certain users.
	tb.commands = map[string]bot.HandlerFunc{
		"/set":    tb.setCommandHandler,
		"/new":    tb.newCommandHandler,
		"/send":   tb.sendCommandHandler,
		"/cancel": tb.cancelCommandHandler,
	}
	tb.registerCommands()

	wasSet, err := tb.bot.SetWebhook(ctx, &bot.SetWebhookParams{
		URL:         webhookURL.String(),
		SecretToken: webhookSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("telegram set https webhook: %w", err)
	}
	if !wasSet {
		return nil, fmt.Errorf("telegram set webhook")
	}

	return tb, nil
}

func (tb *Bot) RegisterBloggingPlatform(name config.AvailableBloggingPlatform, platform blogging.Platform) {
	tb.bloggingPlatforms[name] = platform
}

func (tb *Bot) RegisterAuthorizedBloggingPlatform(name config.AvailableBloggingPlatform, platform blogging.AuthedPlatform) {
	tb.authedBloggingPlatforms[name] = platform
}

func (tb *Bot) registerCommands() {
	for command, handler := range tb.commands {
		tb.bot.RegisterHandlerMatchFunc(tb.commandMatcher(command), handler)
	}

	tb.bot.RegisterHandlerMatchFunc(tb.matchDefault, tb.defaultHandler)
}

// Start runs the bot until the given context is canceled.
func (tb *Bot) Start(ctx context.Context, addr string) error {

	go func() {
		err := http.ListenAndServe(addr, tb.bot.WebhookHandler())
		if err != nil {
			log.Printf("telegram http listen err: %v", err)
		}
	}()

	// Use StartWebhook instead of Start
	tb.bot.StartWebhook(ctx)
	return nil
}

// Stop stops the bot by canceling its context.
// (Typically you would cancel the context passed to Start.)
func (tb *Bot) Stop() {
	// In this design, Stop() is a helper if you want to close the underlying bot immediately.
	close(tb.done) // wait until the bot has finished
}

// --- Handler Match Functions ---

func (tb *Bot) commandMatcher(command string) func(*models.Update) bool {
	return func(u *models.Update) bool {
		if u.Message == nil || u.Message.Text == "" {
			return false
		}
		return u.Message.Text == command
	}
}

func (tb *Bot) matchDefault(u *models.Update) bool {
	if u.Message == nil || u.Message.Text == "" {
		return false
	}
	_, ok := tb.commands[u.Message.Text]
	return !ok
}

// --- Command Handlers ---

func (tb *Bot) setCommandHandler(ctx context.Context, b *bot.Bot, u *models.Update) {
	panic("Not implemented")
}

// newCommandHandler starts a new post (i.e. enters the writing state).
func (tb *Bot) newCommandHandler(ctx context.Context, b *bot.Bot, u *models.Update) {
	chatID := u.Message.Chat.ID

	tb.postsMutex.Lock()
	defer tb.postsMutex.Unlock()

	if _, exists := tb.posts[chatID]; exists {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "You already have an active post. Use /send to post it or /cancel to discard it.",
		})
		if err != nil {
			log.Printf("telegram send message err: %v", err)
		}
		return
	}

	tb.posts[chatID] = &blogging.MicroblogPost{}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "Started a new post. Now send text or images to add content. Use /send when ready or /cancel to discard.",
	})
	if err != nil {
		log.Printf("telegram send message err: %v", err)
	}
}

// sendCommandHandler simulates sending the pending post.
func (tb *Bot) sendCommandHandler(ctx context.Context, b *bot.Bot, u *models.Update) {
	chatID := u.Message.Chat.ID

	tb.postsMutex.Lock()
	post, exists := tb.posts[chatID]
	if exists {
		delete(tb.posts, chatID)
	}
	tb.postsMutex.Unlock()

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
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "Post sent to Mastodon (simulated).",
	})
	if err != nil {
		log.Printf("telegram send message err: %v", err)
	}
}

// cancelCommandHandler discards the pending post.
func (tb *Bot) cancelCommandHandler(ctx context.Context, b *bot.Bot, u *models.Update) {
	chatID := u.Message.Chat.ID

	tb.postsMutex.Lock()
	_, exists := tb.posts[chatID]
	if exists {
		delete(tb.posts, chatID)
	}
	tb.postsMutex.Unlock()

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

func (tb *Bot) getFileContents(ctx context.Context, fileID string) ([]byte, error) {
	fLink, err := tb.bot.GetFile(ctx, &bot.GetFileParams{
		FileID: fileID,
	})
	if err != nil {
		return nil, fmt.Errorf("telegram get photo file: %w", err)
	}
	if fLink.FilePath == "" {
		return nil, fmt.Errorf("telegram get photo file path is empty")
	}
	tgURI, _ := url.Parse(apiTelegramFileURL)
	tgURI = tgURI.JoinPath("bot"+tb.bot.Token(), fLink.FilePath)

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
func (tb *Bot) defaultHandler(ctx context.Context, b *bot.Bot, u *models.Update) {
	if u.Message == nil {
		return
	}

	chatID := u.Message.Chat.ID

	tb.postsMutex.Lock()
	post, active := tb.posts[chatID]
	tb.postsMutex.Unlock()

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

	// Append photo content (if any).
	if len(u.Message.Photo) > 0 {
		// Use the largest photo available (the last element).
		photo := u.Message.Photo[len(u.Message.Photo)-1]
		rawPhotoBytes, err := tb.getFileContents(ctx, photo.FileID)
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
