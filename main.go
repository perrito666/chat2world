package main

import (
	"context"
	"log"
	"net/url"
	"os"
	"os/signal"

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
	tb, err := telegram.New(ctx, os.Getenv("TELEGRAM_BOT_TOKEN"), os.Getenv("TELEGRAM_WEBHOOK_SECRET"), u)
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
