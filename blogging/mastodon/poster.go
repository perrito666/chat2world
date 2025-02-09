package mastodon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/mattn/go-mastodon"

	"github.com/perrito666/chat2world/blogging" // update the module path accordingly
)

// Config holds the configuration for connecting to a Mastodon instance.
type Config struct {
	loaded bool
	// Client
	Server       string      `json:"server,omitempty"`        // e.g., "https://mastodon.example.com"
	AppID        mastodon.ID `json:"app_id,omitempty"`        // the application's ID
	ClientID     string      `json:"client_id,omitempty"`     // your application's client ID
	ClientSecret string      `json:"client_secret,omitempty"` // your application's client secret
	AuthURL      *url.URL    `json:"-"`
	AccessToken  string      `json:"access_token,omitempty"` // the user's (or application's) access token
	// app
	ClientName    string `json:"client_name,omitempty"`
	ClientWebsite string `json:"client_website,omitempty"`
	ClientServer  string `json:"client_server,omitempty"`
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
	userID blogging.UserID
}

var _ blogging.Platform = &Client{}

var ErrClientNotFound = errors.New("client not found")

func (c *Client) Config(userID blogging.UserID) (blogging.ClientConfig, error) {
	if c.config == nil {
		return nil, ErrClientNotFound
	}
	return c.config, nil
}

// NewClient creates a new Mastodon client using the provided configuration.
func NewClient() (*Client, error) {
	return &Client{
		client: mastodon.NewClient(&mastodon.Config{}),
		config: baseConfig(),
	}, nil

}

const ClientName = "Chat2World"
const ClientWebsite = "https://github.com/perrito666/chat2world"

func baseConfig() *Config {
	return &Config{
		ClientName:    ClientName,
		ClientWebsite: ClientWebsite,
	}
}

func (c *Client) IsAuthorized(id blogging.UserID) bool {
	if c.userID == 0 {
		c.userID = id
	}
	if !c.config.loaded {
		_, err := c.loadConfigIfExists(id)
		if err != nil {
			log.Printf("error loading config: %v", err)
			return false
		}
	}
	log.Printf("loaded config for user: %d", c.userID)
	return c.config.loaded
}

// loadConfigIfExists loads a config from a file if it exists.
func (c *Client) loadConfigIfExists(id blogging.UserID) (*Config, error) {
	cfg := baseConfig()
	f, err := os.Open(fmt.Sprintf("%d.json", id))
	if err != nil {
		return cfg, nil
	}
	err = json.NewDecoder(f).Decode(cfg)
	if err != nil {
		return nil, err
	}
	c.config = cfg
	c.config.loaded = true

	// FIXME: make an actual ctx get here
	return cfg, c.authorizeForLoadedConfig(context.Background())
}

func (c *Client) authorizeForLoadedConfig(ctx context.Context) error {
	if c.config == nil || !c.config.loaded {
		return fmt.Errorf("no config loaded")
	}
	/*	app := &mastodon.Application{
		ID:           c.config.AppID,
		RedirectURI:  "urn:ietf:wg:oauth:2.0:oob",
		ClientID:     c.config.ClientID,
		ClientSecret: c.config.ClientSecret,
		AuthURI:      "",
	}*/

	c.client = mastodon.NewClient(&mastodon.Config{
		Server:       c.config.Server,
		ClientID:     c.config.ClientID,
		ClientSecret: c.config.ClientSecret,
		AccessToken:  c.config.AccessToken,
	})
	_, err := c.client.VerifyAppCredentials(ctx)
	if err != nil {
		return fmt.Errorf("verifying app credentials: %w", err)
	}

	return nil
}

func (c *Client) StartAuthorization(ctx context.Context, id blogging.UserID, cfgGeneric map[string]string) (chan string, error) {
	commsChan := make(chan string)
	var cfg *Config
	if !c.config.loaded {
		_, err := c.loadConfigIfExists(id)
		if err != nil {
			log.Printf("error loading config: %v", err)
		}
	}
	go func(id blogging.UserID, cfg *Config, comms chan string) {
		defer close(comms)
		if cfg == nil {
			cfg = baseConfig()
		}
		if cfg.Server == "" {
			log.Printf("No server in config, asking user")
			select {
			case comms <- "What is the mastodon instance server URL?":
			case <-ctx.Done():
				return
			}
			select {
			case cfg.Server = <-comms:
			case <-ctx.Done():
				return
			}

			log.Printf("Server is %s", cfg.Server)
		}
		appConfig := &mastodon.AppConfig{
			Server:       cfg.Server,
			ClientName:   cfg.ClientName,
			Scopes:       "read write follow",
			Website:      cfg.ClientWebsite,
			RedirectURIs: "urn:ietf:wg:oauth:2.0:oob",
		}
		var reauth = cfg.ClientID == "" || cfg.ClientSecret == ""

		app, err := mastodon.RegisterApp(ctx, appConfig)
		if err != nil {
			log.Fatal(err)
			return
		}
		cfg.AppID = app.ID
		cfg.ClientID = app.ClientID
		cfg.ClientSecret = app.ClientSecret
		u, err := url.Parse(app.AuthURI)
		if err != nil {
			log.Fatal(err)
		}
		cfg.AuthURL = u

		if cfg.AccessToken == "" {
			select {
			case comms <- fmt.Sprintf("Open your browser to \n%s\n and copy/paste the given token\n", cfg.AuthURL):
			case <-ctx.Done():
			}
			select {
			case cfg.AccessToken = <-comms:
			case <-ctx.Done():
			}
			reauth = true
		}

		mc := mastodon.NewClient(&mastodon.Config{
			Server:       cfg.Server,
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			AccessToken:  cfg.AccessToken,
		})
		// Token will be at c.Config.AccessToken
		// and will need to be persisted.
		// Otherwise, you'll need to register and authenticate token again.
		if reauth {
			err = mc.AuthenticateToken(context.Background(), cfg.AccessToken, "urn:ietf:wg:oauth:2.0:oob")
			if err != nil {
				log.Fatal(fmt.Errorf("authenticating client: %w", err))
				return
			}
			cfg.AccessToken = mc.Config.AccessToken
		}

		verif, err := c.client.VerifyAppCredentials(ctx)
		if err != nil {
			log.Printf(fmt.Sprintf("verifying app credentials: %v", err))
		}
		log.Printf("verified app credentials: %+v", verif)

		c.client = mc
		cfg.loaded = true
		c.config = cfg
		log.Printf("Client authenticated for server %s with client ID %s", cfg.Server, cfg.ClientID)
		if !reauth {
			return
		}
		mapCfg := cfg.DumpToPersistableDict()
		// create a file in the running folder named after the year, month, day, hour, minute, second.json
		// and dump the cfg to it.
		f, err := os.OpenFile(fmt.Sprintf("%d-%d-%d-%d-%d-%d.json", time.Now().Year(), time.Now().Month(), time.Now().Day(), time.Now().Hour(), time.Now().Minute(), time.Now().Second()), os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		err = json.NewEncoder(f).Encode(mapCfg)
		if err != nil {
			log.Fatal(err)
		}
	}(id, cfg, commsChan)
	return commsChan, nil
}

// Post sends a MicroblogPost to Mastodon. It uploads any images (if present)
// and then creates a new status (toot) with the given text and attachments.
func (c *Client) Post(ctx context.Context, userID blogging.UserID, post *blogging.MicroblogPost) (string, error) {
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
			return "", fmt.Errorf("failed to upload image %d: %w", idx, err)
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
	postedToot, err := c.client.PostStatus(ctx, toot)
	if err != nil {
		log.Printf("failed to post status: %v", err)
		return "", fmt.Errorf("failed to post status: %w", err)
	}

	log.Printf("successfully posted status: %s", post.Text)
	return postedToot.URL, nil
}
