package bluesky

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // register GIF format
	_ "image/jpeg" // register JPEG format
	_ "image/png"  // register PNG format
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// This is mostly documentation and chatGPT, take it with several grains of salt.

// baseURL is the Bluesky server we are targeting.
const baseURL = "https://bsky.social"

// Client holds authentication details and an HTTP client.
type Client struct {
	HttpClient  *http.Client
	AccessJwt   string
	RefreshJwt  string
	Did         string
	Handle      string
	isAthorized bool
	username    string
	appPassword string
}

// NewClient creates a new Bluesky client with the default HTTP client.
func NewClient() *Client {
	return &Client{
		HttpClient: http.DefaultClient,
	}
}

// RefreshSession refreshes the Bluesky session using the current refresh token.
// It sends a POST request to the refresh endpoint and updates the client's tokens.
func (client *Client) RefreshSession() (err error) {
	defer func() {
		if err != nil {
			client.isAthorized = false
		}
	}()
	// Construct the request payload. The API expects the refresh token in a field,
	// here we assume it is named "refresh".
	reqPayload := map[string]string{
		"refresh": client.RefreshJwt,
	}
	jsonBody, err := json.Marshal(reqPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal refresh request body: %w", err)
	}

	url := baseURL + "/xrpc/com.atproto.server.refreshSession"
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Optionally include the current access token in the header.
	req.Header.Set("Authorization", "Bearer "+client.AccessJwt)

	resp, err := client.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read refresh response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh request returned non-OK status: %s", string(body))
	}

	// Assume the response JSON contains new accessJwt and refreshJwt fields.
	var refreshResp struct {
		AccessJwt  string `json:"accessJwt"`
		RefreshJwt string `json:"refreshJwt"`
	}
	if err := json.Unmarshal(body, &refreshResp); err != nil {
		return fmt.Errorf("failed to unmarshal refresh response: %w", err)
	}

	// Update the client with the new tokens.
	client.AccessJwt = refreshResp.AccessJwt
	client.RefreshJwt = refreshResp.RefreshJwt
	return nil
}

// StartSessionRefresher starts a goroutine that periodically refreshes the session
// using the provided interval. The refresher will run until a signal is sent on stopChan.
func (client *Client) StartSessionRefresher(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := client.RefreshSession(); err != nil {
				log.Printf("Failed to refresh session: %v", err)
				// If the refresh fails, attempt to re-authenticate.
				err = client.AuthenticateBluesky(ctx, client.username, client.appPassword)
				if err != nil {
					log.Printf("Failed to re-authenticate: %v", err)
				}
				return
			}
		case <-ctx.Done():
			log.Println("Stopping bsky session refresher")
			return
		}
	}
}

// IsAuthorized returns true if the client is authorized to make requests.
func (client *Client) IsAuthorized() bool {
	return client.isAthorized
}

// AuthenticateBluesky logs in to Bluesky using the provided identifier (handle)
// and app password. On success, it returns a Client with the access tokens.
// According to the official Bluesky Get Started docs (https://docs.bsky.app/docs/get-started),
// you must call the com.atproto.server.createSession endpoint.
func (client *Client) AuthenticateBluesky(ctx context.Context, identifier, password string) error {
	client.username = identifier
	client.appPassword = password
	reqBody := CreateSessionRequest{
		Identifier: identifier,
		Password:   password,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshaling session request body: %w", err)
	}

	url := baseURL + "/xrpc/com.atproto.server.createSession"
	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("POSTing request to create session: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading session response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return errors.New(fmt.Sprintf("createSession returned non-OK status (%d): %s", resp.StatusCode, string(body)))
	}

	var sessionResp CreateSessionResponse
	err = json.Unmarshal(body, &sessionResp)
	if err != nil {
		return fmt.Errorf("unmarshaling session response: %w", err)
	}

	client.isAthorized = true
	client.AccessJwt = sessionResp.AccessJwt
	client.RefreshJwt = sessionResp.RefreshJwt
	client.Did = sessionResp.Did
	client.Handle = sessionResp.Handle

	go client.StartSessionRefresher(ctx, 10*time.Minute)
	return nil
}

/*
	{
	    "$type": "blob",
	    "ref": {
	        "$link": "bafkreibabalobzn6cd366ukcsjycp4yymjymgfxcv6xczmlgpemzkz3cfa"
	    },
	    "mimeType": "image/png",
	    "size": 760898
	}
*/

// UploadImageBlob uploads imageData (e.g. JPEG bytes) to Bluesky's blob storage.
// The MIME type should be provided (e.g. "image/jpeg").
// It returns the blob reference that can be used in a post embed.
func (client *Client) UploadImageBlob(imageData []byte, mimeType string) (*ImageUploadResponse, error) {
	url := baseURL + "/xrpc/com.atproto.repo.uploadBlob"
	req, err := http.NewRequest("POST", url, bytes.NewReader(imageData))
	if err != nil {
		return nil, fmt.Errorf("failed to create upload blob request: %w", err)
	}
	// Set the MIME type of the image.
	req.Header.Set("Content-Type", mimeType)
	// Use the authenticated access token.
	req.Header.Set("Authorization", "Bearer "+client.AccessJwt)

	resp, err := client.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute upload blob request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read upload blob response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload blob returned non-OK status: %s", string(body))
	}

	// Define a response struct to capture the blob reference.
	var uploadResp ImageUploadResponse
	if err := json.Unmarshal(body, &uploadResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal upload blob response: %w", err)
	}
	return &uploadResp, nil
}

/*
{
    "$type": "app.bsky.embed.images",
    "images": [{
        "alt": IMAGE_ALT_TEXT,
        "image": blob,
        "aspectRatio": {
            "width": width,
            "height": height
        }
    }],
}
*/

type PostableImage struct {
	ImageRaw []byte
	AltText  string
	Width    int
	Height   int
	MimeType string
}

func (postableImage *PostableImage) fillImageMeta() error {
	// Determine the MIME type using a sample of the byte slice.
	mimeType := http.DetectContentType(postableImage.ImageRaw)

	// Use image.DecodeConfig to efficiently get the image dimensions.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(postableImage.ImageRaw))
	if err != nil {
		return fmt.Errorf("decoding image config: %w", err)
	}
	postableImage.Width = cfg.Width
	postableImage.Height = cfg.Height
	postableImage.MimeType = mimeType

	return nil
}

// NewPostableImage creates a new PostableImage from the raw image data and alt text.
func NewPostableImage(imageRaw []byte, altText string) (*PostableImage, error) {
	pi := &PostableImage{
		ImageRaw: imageRaw,
		AltText:  altText,
	}
	err := pi.fillImageMeta()
	if err != nil {
		return nil, fmt.Errorf("filling image meta: %w", err)
	}
	return pi, nil
}

// atURIToHTTPSBsky converts an at:// URI to an HTTPS Bluesky link.
func atURIToHTTPSBsky(atURI string) string {
	// at://<DID>/<COLLECTION>/<RKEY>
	// https://bsky.app/profile/<DID>/post/<RKEY>
	atURINoSchema := strings.TrimPrefix(atURI, "at://") // url.Parse does not like DIDs
	parts := strings.Split(atURINoSchema, "/")
	if len(parts) < 3 {
		log.Printf("invalid URI: %s", atURI)
		return ""
	}
	did := parts[0]
	collection := parts[1]
	rkey := parts[2]
	if collection != "app.bsky.feed.post" {
		log.Printf("unsupported collection: %s", collection)
		return ""
	}
	return fmt.Sprintf("https://bsky.app/profile/%s/post/%s", did, rkey)

}

// PostToBluesky publishes a text post using the authenticated Client.
// It sends a POST to the com.atproto.repo.createRecord endpoint with the post content.
// For details on the expected JSON structure, see the Bluesky API reference https://docs.bsky.app/docs/tutorials/creating-a-post
// It tries to return the URL to the bluesky post.
func (client *Client) PostToBluesky(text string, images []*PostableImage, lang []string) (string, error) {
	var embeds []PostEmbed
	if lang == nil {
		lang = []string{"en"} // not a sane default, my default for this example.
	}
	for _, img := range images {
		uploadResp, err := client.UploadImageBlob(img.ImageRaw, img.MimeType)
		if err != nil {
			return "", fmt.Errorf("failed to upload image: %w", err)
		}
		embed := PostEmbed{
			Type: EmbedImagesType,
			Images: []EmbedImage{
				{
					Alt:   img.AltText,
					Image: *uploadResp,
					AspectRatio: EmbedAspectRatio{
						Width:  img.Width,
						Height: img.Height,
					},
				},
			},
		}
		embeds = append(embeds, embed)
	}
	facets, err := ParseFacets(text, baseURL)
	if err != nil {
		log.Printf("failed to parse facets: %v", err)
	}
	record := PostRecord{
		Type:      PostRecordType,
		Text:      text,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Embed:     embeds,
		Langs:     lang,
	}
	if len(facets) > 0 {
		record.Facets = facets
	}
	recordReq := CreateRecordRequest{
		// Use the handle (username) as the repo identifier.
		Repo:       client.Handle,
		Collection: "app.bsky.feed.post",
		Record:     record,
	}
	jsonBody, err := json.Marshal(recordReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal post request: %w", err)
	}

	url := baseURL + "/xrpc/com.atproto.repo.createRecord"
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create new post request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+client.AccessJwt)

	resp, err := client.HttpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute post request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read post response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		jsonBody, _ := json.MarshalIndent(recordReq, "", "  ")
		log.Printf("sending post body: %s", string(jsonBody))
		return "", fmt.Errorf("post request returned non-OK status: %s", string(body))
	}

	var postResp CreateRecordResponse
	err = json.Unmarshal(body, &postResp)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal post response: %w", err)
	}
	return atURIToHTTPSBsky(postResp.Uri), nil
}
