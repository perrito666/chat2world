package blogging

import (
	"bytes"
	"io"
)

type BlogImageRaw []byte

type BlogImage struct {
	Data    BlogImageRaw `json:"data"`
	AltText string       `json:"alt_text"`
}

func (i *BlogImage) Reader() io.Reader {
	return bytes.NewReader(i.Data)
}

func NewBlogImage(data []byte, altText string) *BlogImage {
	return &BlogImage{
		Data:    data,
		AltText: altText,
	}
}

// MicroblogPost holds the data for a Mastodon post.
type MicroblogPost struct {
	Text   string       // Accumulated text content.
	Images []*BlogImage // Telegram file IDs for images.
}

func (b *MicroblogPost) AddImage(image *BlogImage) {
	b.Images = append(b.Images, image)
}
