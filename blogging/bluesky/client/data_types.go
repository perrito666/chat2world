package bluesky

// CreateSessionRequest is the JSON structure for creating a session.
type CreateSessionRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

// CreateSessionResponse is the expected JSON response from the server.
type CreateSessionResponse struct {
	Did        string `json:"did"`
	Handle     string `json:"handle"`
	AccessJwt  string `json:"accessJwt"`
	RefreshJwt string `json:"refreshJwt"`
}

type ATProtoType string

const (
	BlobType         ATProtoType = "blob"
	PostRecordType   ATProtoType = "app.bsky.feed.post"
	EmbedImagesType  ATProtoType = "app.bsky.embed.images"
	FacetMentionType ATProtoType = "app.bsky.richtext.facet#mention"
	FacetLinkType    ATProtoType = "app.bsky.richtext.facet#link"
)

// {"blob":{"$type":"blob","ref":{"$link":"bafkreiepxzhesdi2637rtdgmkm4jdsnixpi5bbpp5gz2fq64ebwzrltoau"},"mimeType":"image/jpeg","size":115022}}
type Ref struct {
	Link string `json:"$link"`
}

type OuterBlob struct {
	Blob ImageUploadResponse `json:"blob"`
}

// ImageUploadResponse represents the response from an image upload call.
type ImageUploadResponse struct {
	Type     ATProtoType `json:"$type"`
	Ref      Ref         `json:"ref"`
	MimeType string      `json:"mimeType"`
	Size     int         `json:"size"`
}

// EmbedAspectRatio defines the aspect ratio of an embedded image.
type EmbedAspectRatio struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// EmbedImage defines the structure for embedding an image in a Bluesky post.
type EmbedImage struct {
	Alt         string              `json:"alt"`
	Image       ImageUploadResponse `json:"image"`
	AspectRatio EmbedAspectRatio    `json:"aspectRatio"`
}

// PostEmbed defines the structure for embedding images in a Bluesky post.
type PostEmbed struct {
	Type   ATProtoType  `json:"$type"`
	Images []EmbedImage `json:"images"`
}

// PostRecord defines the inner record for a Bluesky post.
type PostRecord struct {
	Type      ATProtoType `json:"$type"`
	Text      string      `json:"text"`
	CreatedAt string      `json:"createdAt"`
	Embed     PostEmbed   `json:"embed"`
	Langs     []string    `json:"langs"` //tbd in a decent way
	Facets    []Facet     `json:"facets"`
}

// CreateRecordRequest defines the full request for creating a record.
// The "repo" field should be set to your handle (as per the examples in the docs,
// see :contentReference[oaicite:3]{index=3}) and "collection" is fixed to "app.bsky.feed.post".
type CreateRecordRequest struct {
	Repo       string     `json:"repo"`
	Collection string     `json:"collection"`
	Record     PostRecord `json:"record"`
}

// CreateRecordResponse represents the response from a post creation call.
type CreateRecordResponse struct {
	Uri string `json:"uri"`
	Cid string `json:"cid"`
}
