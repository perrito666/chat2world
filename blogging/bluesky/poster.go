package bluesky

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/perrito666/chat2world/blogging"
	"github.com/perrito666/chat2world/blogging/bluesky/client"
	"github.com/perrito666/chat2world/secrets"
)

// Config holds the configuration for connecting to a Bluesky instance.
type Config struct {
	User        string `json:"user,omitempty"`
	AppPassword string `json:"app_password,omitempty"`
}

func (c *Config) LoadFromPersistableDict(dict map[string]string) error {
	c.User = dict["user"]
	c.AppPassword = dict["app_password"]
	return nil
}

func (c *Config) DumpToPersistableDict() map[string]string {
	return map[string]string{
		"user":         c.User,
		"app_password": c.AppPassword,
	}
}

var _ blogging.ClientConfig = (*Config)(nil)

// Client wraps a Mastodon client and provides a method to post.
type Client struct {
	store  *secrets.EncryptedStore
	client *bluesky.Client
	config *Config
	userID blogging.UserID
}

func (c *Client) Config(userID blogging.UserID) (blogging.ClientConfig, error) {
	if c.config == nil {
		return nil, blogging.ErrClientNotFound
	}
	return c.config, nil
}

// NewClient creates a new Mastodon client using the provided configuration.
func NewClient(store *secrets.EncryptedStore) (*Client, error) {
	return &Client{
		store:  store,
		client: bluesky.NewClient(),
		config: &Config{},
	}, nil
}

var _ blogging.AuthedPlatform = (*Client)(nil)

func (c *Client) IsAuthorized(id blogging.UserID) bool {
	if c.userID == 0 {
		c.userID = id
	}
	if c.config.User == "" || c.config.AppPassword == "" {
		_, err := c.loadConfigIfExists(id)
		if err != nil {
			log.Printf("error loading config: %v", err)
		}
	}
	if !c.client.IsAuthorized() {
		err := c.client.AuthenticateBluesky(context.Background(), c.config.User, c.config.AppPassword)
		if err != nil {
			log.Printf("error authenticating: %v", err)
			return false
		}
	}

	return c.client.IsAuthorized()
}

// loadConfigIfExists loads a config from a file if it exists.
func (c *Client) loadConfigIfExists(id blogging.UserID) (*Config, error) {
	cfg := &Config{}
	f, err := c.store.OpenReader(fmt.Sprintf("%d.bsky.json", id))
	if err != nil {
		return cfg, nil
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(cfg)
	if err != nil {
		return nil, fmt.Errorf("loading configuration for bsky from disk: %w", err)
	}
	c.config = cfg

	// FIXME: make an actual ctx get here
	return cfg, nil
}

func (c *Client) StartAuthorization(ctx context.Context, id blogging.UserID, cfgGeneric map[string]string) (chan string, error) {
	commsChan := make(chan string)
	if c.config.User == "" {
		_, err := c.loadConfigIfExists(id)
		if err != nil {
			return nil, fmt.Errorf("loading config: %w", err)
		}
	}
	go func(id blogging.UserID, cfg *Config, comms chan string) {
		defer close(comms)
		if cfg.User == "" {
			log.Printf("no user found in config, asking user")
			select {
			case comms <- "What is your Bluesky username?":
			case <-ctx.Done():
				return
			}
			select {
			case cfg.User = <-comms:
			case <-ctx.Done():
				return
			}
		}
		if cfg.AppPassword == "" {
			log.Printf("no app password found in config, asking user")
			select {
			case comms <- "What is your Bluesky Application password?":
			case <-ctx.Done():
				return
			}
			select {
			case cfg.AppPassword = <-comms:
			case <-ctx.Done():
				return
			}
		}
		err := c.client.AuthenticateBluesky(ctx, cfg.User, cfg.AppPassword)
		if err != nil {
			log.Printf("error authenticating: %v", err)
			return
		}
		if cfg.User != "" && cfg.AppPassword != "" {
			// create a file in the running folder named after the year, month, day, hour, minute, second.json
			// and dump the cfg to it.
			f, err := c.store.OpenWriter(fmt.Sprintf("%d.bsky.json", c.userID))
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()
			err = json.NewEncoder(f).Encode(cfg)
			if err != nil {
				log.Fatal(err)
			}
		}
	}(id, c.config, commsChan)
	return commsChan, nil
}

var _ blogging.Platform = (*Client)(nil)

func (c *Client) Post(ctx context.Context, userID blogging.UserID, post *blogging.MicroblogPost) (string, error) {
	postImages := make([]*bluesky.PostableImage, len(post.Images))
	var err error
	for idx, img := range post.Images {
		postImages[idx], err = bluesky.NewPostableImage(img.Data, img.AltText)
		if err != nil {
			return "", fmt.Errorf("creating postable image: %w", err)
		}
	}
	var bskyURL string
	bskyURL, err = c.client.PostToBluesky(post.Text, postImages, []string{"en"})
	if err != nil {
		return "", fmt.Errorf("posting to bluesky: %w", err)
	}
	return bskyURL, nil
}
