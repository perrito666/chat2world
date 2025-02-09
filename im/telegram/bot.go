package telegram

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/perrito666/chat2world/blogging"
	"github.com/perrito666/chat2world/blogging/mastodon"
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
	flowSchedulers          map[int64]*FlowScheduler

	authFlowOngoing map[int64]map[config.AvailableBloggingPlatform]bool

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
		bot:            b,
		posts:          make(map[int64]*blogging.MicroblogPost),
		done:           make(chan struct{}),
		flowSchedulers: make(map[int64]*FlowScheduler),
	}

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
	re := regexp.MustCompile(".*")
	tb.bot.RegisterHandlerRegexp(bot.HandlerTypeMessageText, re, tb.defaultHandler)
	tb.bot.RegisterHandlerRegexp(bot.HandlerTypePhotoCaption, re, tb.defaultHandler)
	tb.bot.RegisterHandlerRegexp(bot.HandlerTypeCallbackQueryData, re, tb.defaultHandler)
	tb.bot.RegisterHandlerRegexp(bot.HandlerTypeCallbackQueryGameShortName, re, tb.defaultHandler)
	log.Printf("telegram bot created")
	return tb, nil
}

// Start runs the bot until the given context is canceled.
func (tb *Bot) Start(ctx context.Context, addr string) error {

	go func() {
		log.Printf("telegram http listen on %s", addr)
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

// defaultHandler processes any non-command (or unmatched) messages.
// If a chat is in "writing mode", the message content is appended to the post.
func (tb *Bot) defaultHandler(ctx context.Context, b *bot.Bot, u *models.Update) {
	log.Printf("telegram default handler from chat ID %d: %s", u.Message.Chat.ID, u.Message.Text)
	if u.Message == nil {
		return
	}

	chatID := u.Message.Chat.ID
	var sched = tb.flowSchedulers[chatID]

	if sched == nil {
		sched = NewScheduler()
		cm, err := mastodon.NewClient()
		if err != nil {
			log.Printf("mastodon new client err: %v", err)
			return
		}
		maf := NewMastodonAuthorizerFlow(cm)
		sched.registerFlow(maf, "mastodon_auth", []string{"/mastodon_auth"})
		sched.registerFlow(NewPostingFlow(map[config.AvailableBloggingPlatform]blogging.AuthedPlatform{config.MBPMastodon: cm}),
			"microblog_post", []string{"/new"})
		tb.flowSchedulers[chatID] = sched
	}

	err := sched.handleMessage(ctx, b, u)
	if err != nil {
		log.Printf("telegram handle message err: %v", err)
		return
	}
	return
}
