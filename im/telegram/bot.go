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

	"github.com/perrito666/chat2world/config"
	"github.com/perrito666/chat2world/im"
)

// Bot wraps the underlying bot.Bot and holds state.
type Bot struct {
	bot                  *bot.Bot
	postsMutex           sync.Mutex
	commands             map[string]bot.HandlerFunc
	flowSchedulers       map[uint64]*im.FlowScheduler
	flowSchedulerFactory im.SchedulerFactoryFN

	authFlowOngoing map[int64]map[config.AvailableBloggingPlatform]bool
}

// SendMessage sends a im.Message to telegram (with all the needed translation)
func (tb *Bot) SendMessage(ctx context.Context, message *im.Message) error {
	params := &bot.SendMessageParams{
		ChatID: message.ChatID,
		Text:   message.Text,
	}
	if message.InReplyTo != 0 {
		params.ReplyParameters = &models.ReplyParameters{
			MessageID: int(message.InReplyTo),
		}
	}
	_, err := tb.bot.SendMessage(ctx, params)
	if err != nil {
		return fmt.Errorf("telegram send message: %w", err)
	}
	return nil
}

var _ im.Messenger = (*Bot)(nil)

// New creates a new Telegram bot instance.
// You can pass additional bot.Options if needed.
func New(ctx context.Context,
	token string, webhookSecret string, webhookURL *url.URL,
	schedulerFn im.SchedulerFactoryFN) (*Bot, error) {
	// Create the underlying bot.
	opt := bot.WithWebhookSecretToken(webhookSecret)
	b, err := bot.New(token, opt)
	if err != nil {
		return nil, err
	}

	tb := &Bot{
		bot:                  b,
		flowSchedulerFactory: schedulerFn,
		flowSchedulers:       make(map[uint64]*im.FlowScheduler),
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

// Stop is wishful thinking for now.
func (tb *Bot) Stop() {
}

// defaultHandler processes any non-command (or unmatched) messages.
// If a chat is in "writing mode", the message content is appended to the post.
func (tb *Bot) defaultHandler(ctx context.Context, b *bot.Bot, u *models.Update) {
	log.Printf("telegram default handler from chat ID %d: %s", u.Message.Chat.ID, u.Message.Text)
	if u.Message == nil {
		return
	}

	message, err := messageFromTelegramMessage(ctx, b, u)
	if err != nil {
		log.Printf("telegram message from telegram message err: %v", err)
		return
	}

	var sched = tb.flowSchedulers[message.UserID]

	if sched == nil {
		sched, err = tb.flowSchedulerFactory(message.UserID)
		if err != nil {
			log.Printf("telegram flow scheduler factory err: %v", err)
			return
		}
		tb.flowSchedulers[message.UserID] = sched
	}

	err = sched.HandleMessage(ctx, message, tb)
	if err != nil {
		log.Printf("telegram handle message err: %v", err)
		return
	}
	return
}
