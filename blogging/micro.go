package blogging

import (
	"bytes"
	"io"
)

// BlogImageRaw is a byte slice that represents an image as obtained from a im messenger, it is mostly intended
// so we do not pass arbitrary byte slices around without intent.
type BlogImageRaw []byte

// BlogImage is a struct that holds the data and metadata of an image (that we care about)
type BlogImage struct {
	Data    BlogImageRaw `json:"data"`
	AltText string       `json:"alt_text"`
}

// Reader returns the raw image bytes wrapped in a reader.
func (i *BlogImage) Reader() io.Reader {
	return bytes.NewReader(i.Data)
}

// NewBlogImage creates a new BlogImage from a byte slice and an alt text as usually provided by the messenger.
func NewBlogImage(data []byte, altText string) *BlogImage {
	return &BlogImage{
		Data:    data,
		AltText: altText,
	}
}

// MicroblogPost holds the data for a Microblog post.
type MicroblogPost struct {
	Text   string       // Accumulated text content.
	Images []*BlogImage // Telegram file IDs for images.
}

// AddImage adds an image to the post.
func (b *MicroblogPost) AddImage(image *BlogImage) {
	b.Images = append(b.Images, image)
}
