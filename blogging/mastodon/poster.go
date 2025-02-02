package mastodon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"

	"github.com/mattn/go-mastodon"

	"github.com/perrito666/chat2world/blogging" // update the module path accordingly
)

// Config holds the configuration for connecting to a Mastodon instance.
type Config struct {
	// Client
	Server       string // e.g., "https://mastodon.example.com"
	ClientID     string // your application's client ID
	ClientSecret string // your application's client secret
	AuthURL      *url.URL
	AccessToken  string // the user's (or application's) access token
	// app
	ClientName    string
	ClientWebsite string
	ClientServer  string
}

func (c *Config) LoadFromPersistableDict(dict map[string]string) error {
	c.Server = dict["server"]
	c.ClientID = dict["client_id"]
	c.ClientSecret = dict["client_secret"]
	c.AuthURL, _ = url.Parse(dict["auth_url"])
	c.AccessToken = dict["access_token"]
	c.ClientName = dict["client_name"]
	c.ClientWebsite = dict["client_website"]
	c.ClientServer = dict["client_server"]
	return nil
}

func (c *Config) DumpToPersistableDict() map[string]string {
	return map[string]string{
		"server":         c.Server,
		"client_id":      c.ClientID,
		"client_secret":  c.ClientSecret,
		"auth_url":       c.AuthURL.String(),
		"access_token":   c.AccessToken,
		"client_name":    c.ClientName,
		"client_website": c.ClientWebsite,
		"client_server":  c.ClientServer,
	}
}

// Client wraps a Mastodon client and provides a method to post.
type Client struct {
	client *mastodon.Client
	config *Config
}

func (c *Client) Config() blogging.ClientConfig {
	//TODO implement me
	panic("implement me")
}

var _ blogging.Platform = &ClientManager{}

type ClientManager struct {
	clients map[blogging.ChatID]*Client
}

var ErrClientNotFound = errors.New("client not found")

func (cm *ClientManager) Post(ctx context.Context, chatID blogging.ChatID, post *blogging.MicroblogPost) error {
	if client, ok := cm.clients[chatID]; ok {
		err := client.Post(ctx, post)
		if err != nil {
			return fmt.Errorf("posting failed: %v", err)
		}
		return nil
	}
	return ErrClientNotFound
}

func (cm *ClientManager) Config(chatID blogging.ChatID) (blogging.ClientConfig, error) {
	if client, ok := cm.clients[chatID]; ok {
		if client.config == nil {
			client.config = baseConfig()
		}
		return client.config, nil
	}
	return nil, ErrClientNotFound
}

// NewClient creates a new Mastodon client using the provided configuration.
func NewClient() (*ClientManager, error) {
	cm := &ClientManager{clients: make(map[blogging.ChatID]*Client)}
	return cm, nil
}

const ClientName = "Chat2World"
const ClientWebsite = "https://github.com/perrito666/chat2world"

func baseConfig() *Config {
	return &Config{
		ClientName:    ClientName,
		ClientWebsite: ClientWebsite,
	}
}

func (cm *ClientManager) StartAuthorization(id blogging.ChatID, cfg *Config) (chan string, error) {
	commsChan := make(chan string)
	go func(id blogging.ChatID, cfg *Config, comms chan string) {
		if cfg == nil {
			cfg = baseConfig()
		}
		if cfg.Server == "" {
			comms <- "What is the mastodon instance server URL?"
			cfg.Server = <-comms
		}
		appConfig := &mastodon.AppConfig{
			Server:       cfg.Server,
			ClientName:   cfg.ClientName,
			Scopes:       "read write follow",
			Website:      cfg.ClientWebsite,
			RedirectURIs: "urn:ietf:wg:oauth:2.0:oob",
		}
		if cfg.ClientID == "" || cfg.ClientSecret == "" {
			app, err := mastodon.RegisterApp(context.Background(), appConfig)
			if err != nil {
				log.Fatal(err)
			}
			cfg.ClientID = app.ClientID
			cfg.ClientSecret = app.ClientSecret
			u, err := url.Parse(app.AuthURI)
			if err != nil {
				log.Fatal(err)
			}
			cfg.AuthURL = u
		}
		if cfg.AccessToken == "" {
			comms <- fmt.Sprintf("Open your browser to \n%s\n and copy/paste the given token\n", cfg.AuthURL)
			cfg.AccessToken = <-comms
		}

		mc := mastodon.NewClient(&mastodon.Config{
			Server:       cfg.Server,
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			AccessToken:  cfg.AccessToken,
		})
		// Token will be at c.Config.AccessToken
		// and will need to be persisted.
		// Otherwise you'll need to register and authenticate token again.
		err := mc.AuthenticateToken(context.Background(), cfg.AccessToken, "urn:ietf:wg:oauth:2.0:oob")
		if err != nil {
			log.Fatal(fmt.Errorf("authenticating client: %w", err))
		}

		cm.clients[id] = &Client{
			client: mc,
			config: cfg,
		}
	}(id, cfg, commsChan)
	return commsChan, nil
}

// Post sends a MicroblogPost to Mastodon. It uploads any images (if present)
// and then creates a new status (toot) with the given text and attachments.
func (c *Client) Post(ctx context.Context, post *blogging.MicroblogPost) error {
	var mediaIDs []mastodon.ID

	// Upload images (if any).
	for idx, img := range post.Images {
		// UploadMediaFromReader accepts an io.Reader; here we wrap the raw data.
		attachment, err := c.client.UploadMediaFromMedia(ctx, &mastodon.Media{
			File:        img.Reader(),
			Description: img.AltText,
		})
		if err != nil {
			log.Printf("failed to upload image %d: %v", idx, err)
			return fmt.Errorf("failed to upload image %d: %w", idx, err)
		}
		mediaIDs = append(mediaIDs, attachment.ID)
	}

	// Prepare the toot (status).
	toot := &mastodon.Toot{
		Status:   post.Text,
		MediaIDs: mediaIDs,
		// Optionally, you could set additional fields such as Visibility here.
	}

	// Post the toot.
	_, err := c.client.PostStatus(ctx, toot)
	if err != nil {
		log.Printf("failed to post status: %v", err)
		return fmt.Errorf("failed to post status: %w", err)
	}

	log.Printf("successfully posted status: %s", post.Text)
	return nil
}
