package bluesky

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
)

// This is a straight translation from the example python in https://docs.bsky.app/docs/advanced-guides/posts#mentions-and-links

// MentionSpan represents a mention found in the text.
type MentionSpan struct {
	Start  int
	End    int
	Handle string
}

// URLSpan represents a URL found in the text.
type URLSpan struct {
	Start int
	End   int
	URL   string
}

// Index represents the span (by byte offsets) for a facet.
type Index struct {
	ByteStart int `json:"byteStart"`
	ByteEnd   int `json:"byteEnd"`
}

// Feature represents a facet feature – it can be either a mention or a link.
type Feature struct {
	Type ATProtoType `json:"$type"`         // e.g. "app.bsky.richtext.facet#mention" or "#link"
	Did  string      `json:"did,omitempty"` // for mentions
	URI  string      `json:"uri,omitempty"` // for links
}

// Facet represents a facet with an index and a set of features.
type Facet struct {
	Index    Index     `json:"index"`
	Features []Feature `json:"features"`
}

// ResolveHandleResponse is used to decode the JSON response from the
// handle resolution endpoint.
type ResolveHandleResponse struct {
	Did string `json:"did"`
}

// The mention regex is based on the AT Protocol handle specification.
// It matches a mention preceded by either the beginning of the string or a non-word character.
var mentionRegex = regexp.MustCompile(
	`(?:^|[^A-Za-z0-9_])(@(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)`)

// The URL regex is a partial/naïve regex based on a common StackOverflow answer.
// It captures http(s) URLs.
var urlRegex = regexp.MustCompile(
	`(?:^|[^A-Za-z0-9_])(https?://(?:www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[A-Za-z0-9()]{1,6}\b(?:[-a-zA-Z0-9()@:%_\+.~#?&//=]*[-a-zA-Z0-9@%_\+~#//=])?)`)

// parseMentions scans the given text and returns a slice of MentionSpan.
// It converts the text to a byte slice and uses the compiled regex.
// Note: The returned Start and End indices refer to byte positions.
func parseMentions(text string) []MentionSpan {
	var spans []MentionSpan
	textBytes := []byte(text)
	// FindAllSubmatchIndex returns a slice of index pairs:
	// [fullMatchStart, fullMatchEnd, group1Start, group1End, ...]
	matches := mentionRegex.FindAllSubmatchIndex(textBytes, -1)
	for _, m := range matches {
		// We expect at least two pairs: m[0:2] for the whole match and m[2:4] for group 1.
		if len(m) < 4 {
			continue
		}
		grpStart, grpEnd := m[2], m[3]
		// Skip if the match is too short.
		if grpEnd-grpStart < 1 {
			continue
		}
		// Remove the initial "@" by slicing one byte forward.
		handle := string(textBytes[grpStart+1 : grpEnd])
		spans = append(spans, MentionSpan{
			Start:  grpStart,
			End:    grpEnd,
			Handle: handle,
		})
	}
	return spans
}

// parseURLs scans the given text and returns a slice of URLSpan.
func parseURLs(text string) []URLSpan {
	var spans []URLSpan
	textBytes := []byte(text)
	matches := urlRegex.FindAllSubmatchIndex(textBytes, -1)
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		grpStart, grpEnd := m[2], m[3]
		urlStr := string(textBytes[grpStart:grpEnd])
		spans = append(spans, URLSpan{
			Start: grpStart,
			End:   grpEnd,
			URL:   urlStr,
		})
	}
	return spans
}

// ParseFacets parses the text for mentions and URLs and builds facet data.
// It takes a second parameter, pdsURL, which is the base URL of the PDS service
// used to resolve handles into DIDs.
// For each mention, it makes an HTTP GET request to resolve the handle.
// If the response status is 400, the mention is skipped.
func ParseFacets(text string, pdsURL string) ([]Facet, error) {
	var facets []Facet

	// Process mentions.
	mentions := parseMentions(text)
	for _, m := range mentions {
		// Build the URL for handle resolution.
		resolveURL := fmt.Sprintf("%s/xrpc/com.atproto.identity.resolveHandle?handle=%s", pdsURL, m.Handle)
		resp, err := http.Get(resolveURL)
		if err != nil {
			// Skip this mention on error.
			log.Printf("Error resolving handle %s: %v", m.Handle, err)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Error reading response for handle %s: %v", m.Handle, err)
			continue
		}
		if resp.StatusCode == http.StatusBadRequest {
			// Skip unresolved handles.
			continue
		}
		var resolveResp ResolveHandleResponse
		if err := json.Unmarshal(body, &resolveResp); err != nil {
			log.Printf("Error unmarshaling response for handle %s: %v", m.Handle, err)
			continue
		}
		// Create a facet for this mention.
		facet := Facet{
			Index: Index{
				ByteStart: m.Start,
				ByteEnd:   m.End,
			},
			Features: []Feature{
				{
					Type: FacetMentionType,
					Did:  resolveResp.Did,
				},
			},
		}
		facets = append(facets, facet)
	}

	// Process URLs.
	urls := parseURLs(text)
	for _, u := range urls {
		facet := Facet{
			Index: Index{
				ByteStart: u.Start,
				ByteEnd:   u.End,
			},
			Features: []Feature{
				{
					Type: FacetLinkType,
					URI:  u.URL,
				},
			},
		}
		facets = append(facets, facet)
	}

	return facets, nil
}
