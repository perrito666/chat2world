package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"

	"github.com/perrito666/chat2world/blogging"
	"github.com/perrito666/chat2world/blogging/bluesky"
	"github.com/perrito666/chat2world/blogging/mastodon"
	"github.com/perrito666/chat2world/config"
	"github.com/perrito666/chat2world/im"
	"github.com/perrito666/chat2world/im/telegram" // update this import path to match your module layout
	"github.com/perrito666/chat2world/secrets"
)

// uint64Slice is a custom flag type that accumulates uint64 values.
type uint64Slice []uint64

func (s *uint64Slice) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *uint64Slice) Set(value string) error {
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return fmt.Errorf("parse uint64 %v: %w", value, err)
	}
	*s = append(*s, n)
	return nil
}

type strSlice []string

func (s *strSlice) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *strSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// onlyDecryptFiles takes a slice of strings representing file paths and a store and opens each file then writes it
// decrypted to a file with the same name but with the .clear extension.
func onlyDecryptFiles(files []string, store *secrets.EncryptedStore) error {
	log.Printf("files: %v", files)
	for _, f := range files {
		err := func() error {
			// Open the file to read.
			r, err := store.OpenReader(f)
			if err != nil {
				return fmt.Errorf("opening file to read: %w", err)
			}
			defer r.Close()

			// Open the encrypted file to write.
			fmt.Printf("f: %s\n", f)
			w, err := os.OpenFile(f+".clear", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				return fmt.Errorf("opening encrypted file to write: %w", err)
			}
			defer w.Close()

			// Copy the file contents to the encrypted file.
			var written int64
			if written, err = io.Copy(w, r); err != nil {
				return fmt.Errorf("writing to clear file: %w", err)
			}
			log.Printf("wrote %d bytes to %s", written, f+".clear")
			return nil
		}()
		if err != nil {
			return fmt.Errorf("decrypting file %s: %w", f, err)
		}
	}
	return nil
}

// onlyEncryptFiles takes a slice of strings representing file paths and a store and opens each file then writes it
// encrypted to a file with the same name but with the .enc extension.
func onlyEncryptFiles(files []string, store *secrets.EncryptedStore) error {
	for _, f := range files {
		err := func() error {
			// Open the file to read.
			r, err := os.Open(f)
			if err != nil {
				return fmt.Errorf("opening file to read: %w", err)
			}
			defer r.Close()

			// Open the encrypted file to write.
			w, err := store.OpenWriter(f + ".enc")
			if err != nil {
				return fmt.Errorf("opening encrypted file to write: %w", err)
			}
			defer w.Close()

			// Copy the file contents to the encrypted file.
			if _, err := r.WriteTo(w); err != nil {
				return fmt.Errorf("writing to encrypted file: %w", err)
			}
			return nil
		}()
		if err != nil {
			return fmt.Errorf("encrypting file %s: %w", f, err)
		}
	}
	return nil
}

func main() {
	// Create a cancelable context that ends when an interrupt is received.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Define and parse the allowed Telegram user ID flags.
	var allowedTelegramUsers uint64Slice
	var encryptFiles strSlice
	var decryptFiles strSlice
	flag.Var(&allowedTelegramUsers, "with-allowed-telegram-user", "Allowed Telegram user ID (can be specified multiple times)")
	flag.Var(&encryptFiles, "encrypt-file", "File to encrypt")
	flag.Var(&decryptFiles, "decrypt-file", "File to decrypt")
	flag.Parse()

	pasword := os.Getenv("CHAT2WORLD_PASSWORD")
	store := &secrets.EncryptedStore{Password: pasword}
	if len(encryptFiles) > 0 {
		if err := onlyEncryptFiles(encryptFiles, store); err != nil {
			log.Fatalf("failed to encrypt files: %v", err)
		}
		log.Printf("files encrypted")
		return
	}

	if len(decryptFiles) > 0 {
		if err := onlyDecryptFiles(decryptFiles, store); err != nil {
			log.Fatalf("failed to decrypt files: %v", err)
		}
		log.Printf("files decrypted")
		return
	}

	// Try and load the secrets from the environment.
	telegramSecrets := map[string]string{}
	resave := false
	for _, k := range []string{"TELEGRAM_BOT_TOKEN", "TELEGRAM_WEBHOOK_SECRET", "TELEGRAM_LISTEN_ADDR", "CHAT2WORLD_URL"} {
		telegramSecrets[k] = os.Getenv(k)
		if telegramSecrets[k] != "" {
			resave = true
		}
	}
	if resave {
		f, err := store.OpenWriter("telegram.config")
		if err != nil {
			log.Fatal(fmt.Errorf("opening encrypted file to write: %w", err))
			return
		}
		err = json.NewEncoder(f).Encode(&telegramSecrets)
		f.Close()
		if err != nil {
			log.Fatal(fmt.Errorf("writing encrypted file: %w", err))
		}
	} else {
		f, err := store.OpenReader("telegram.config")
		if err != nil {
			log.Fatal(fmt.Errorf("opening encrypted file to read: %w", err))
			return
		}

		err = json.NewDecoder(f).Decode(&telegramSecrets)
		f.Close()
		if err != nil {
			log.Fatal(fmt.Errorf("reading encrypted file: %w", err))
		}
	}

	u, err := url.Parse(telegramSecrets["CHAT2WORLD_URL"])
	if err != nil {
		log.Fatal(err)
		return
	}

	// Create the bot instance.
	tb, err := telegram.New(ctx, telegramSecrets["TELEGRAM_BOT_TOKEN"], telegramSecrets["TELEGRAM_WEBHOOK_SECRET"], u,
		allowedTelegramUsers,
		func(userID uint64) (*im.FlowScheduler, error) {
			sched := im.NewScheduler()

			// mastodon
			cm, err := mastodon.NewClient(store)
			if err != nil {
				log.Printf("mastodon new client err: %v", err)
				return nil, fmt.Errorf("mastodon new client: %w", err)
			}
			maf := blogging.NewAuthorizerFlow(cm)
			if err = sched.RegisterFlow(maf, "mastodon_auth", []string{"/mastodon_auth"}); err != nil {
				log.Printf("mastodon auth flow err: %v", err)
				return nil, fmt.Errorf("mastodon auth flow: %w", err)
			}
			// done only for effect, this will trigger a load of user config
			cm.IsAuthorized(blogging.UserID(userID))

			// bluesky
			bskyCM, err := bluesky.NewClient(store)
			if err != nil {
				log.Printf("bluesky new client err: %v", err)
				return nil, fmt.Errorf("bluesky new client: %w", err)
			}
			bskyAF := blogging.NewAuthorizerFlow(bskyCM)
			if err = sched.RegisterFlow(bskyAF, "bluesky_auth", []string{"/bluesky_auth"}); err != nil {
				log.Printf("bluesky auth flow err: %v", err)
				return nil, fmt.Errorf("bluesky auth flow: %w", err)
			}
			// done only for effect, this will trigger a load of user config
			bskyCM.IsAuthorized(blogging.UserID(userID))

			if err = sched.RegisterFlow(blogging.NewPostingFlow(map[config.AvailableBloggingPlatform]blogging.AuthedPlatform{config.MBPMastodon: cm, config.MBPBsky: bskyCM}),
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
		if err := tb.Start(ctx, telegramSecrets["TELEGRAM_LISTEN_ADDR"]); err != nil {
			log.Printf("bot stopped with error: %v", err)
		}
	}()

	// Block until context is canceled.
	<-ctx.Done()

	// Stop the bot (if not already stopped).
	tb.Stop()
	log.Println("Bot stopped.")

}
