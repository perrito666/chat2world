package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"

	"github.com/perrito666/chat2world/blogging"
	"github.com/perrito666/chat2world/blogging/mastodon"
	"github.com/perrito666/chat2world/config"
	"github.com/perrito666/chat2world/im"
	"github.com/perrito666/chat2world/im/telegram" // update this import path to match your module layout
)

func main() {
	// Create a cancelable context that ends when an interrupt is received.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Create the bot instance.
	u, err := url.Parse(os.Getenv("CHAT2WORLD_URL"))
	if err != nil {
		log.Fatal(err)
		return
	}
	tb, err := telegram.New(ctx, os.Getenv("TELEGRAM_BOT_TOKEN"), os.Getenv("TELEGRAM_WEBHOOK_SECRET"), u,
		func(userID uint64) (*im.FlowScheduler, error) {
			sched := im.NewScheduler()

			cm, err := mastodon.NewClient()
			if err != nil {
				log.Printf("mastodon new client err: %v", err)
				return nil, fmt.Errorf("mastodon new client: %w", err)
			}
			maf := mastodon.NewMastodonAuthorizerFlow(cm)
			if err = sched.RegisterFlow(maf, "mastodon_auth", []string{"/mastodon_auth"}); err != nil {
				log.Printf("mastodon auth flow err: %v", err)
				return nil, fmt.Errorf("mastodon auth flow: %w", err)
			}
			// done only for effect, this will trigger a load of user config
			cm.IsAuthorized(blogging.UserID(userID))
			if err = sched.RegisterFlow(blogging.NewPostingFlow(map[config.AvailableBloggingPlatform]blogging.AuthedPlatform{config.MBPMastodon: cm}),
				"microblog_post", []string{"/new"}); err != nil {
				log.Printf("microblog post flow err: %v", err)
				return nil, fmt.Errorf("microblog post flow: %w", err)
			}
			return sched, nil
		})
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	// Start the bot.
	go func() {
		if err := tb.Start(ctx, os.Getenv("TELEGRAM_LISTEN_ADDR")); err != nil {
			log.Printf("bot stopped with error: %v", err)
		}
	}()

	// Block until context is canceled.
	<-ctx.Done()

	// Stop the bot (if not already stopped).
	tb.Stop()
	log.Println("Bot stopped.")

}
