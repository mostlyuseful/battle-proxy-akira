package logging

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"battle-proxy-akira/internal/ir"
)

const (
	ImageSourceDataURL = "data_url"
	ImageSourceURL     = "url"
)

// ImageMetadataFromRequest extracts safe image metadata from a normalized request.
func ImageMetadataFromRequest(req ir.Request) []ImageInputMetadata {
	var out []ImageInputMetadata
	for _, message := range req.Messages {
		for _, part := range message.Content {
			if part.Type != ir.ContentTypeImageURL && part.ImageURL == "" {
				continue
			}
			if metadata, ok := imageMetadata(part.ImageURL); ok {
				out = append(out, metadata)
			}
		}
	}
	return out
}

func imageMetadata(raw string) (ImageInputMetadata, bool) {
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		return dataURLMetadata(raw)
	}
	if strings.TrimSpace(raw) == "" {
		return ImageInputMetadata{}, false
	}
	// External URLs are redacted by default until a configurable URL logging
	// policy exists; record only that a URL image was present.
	return ImageInputMetadata{Source: ImageSourceURL, URLRedacted: true}, true
}

func dataURLMetadata(raw string) (ImageInputMetadata, bool) {
	metadata, encoded, ok := strings.Cut(raw, ",")
	if !ok {
		return ImageInputMetadata{Source: ImageSourceDataURL, URLRedacted: true}, true
	}
	mimeType := ""
	meta := strings.TrimPrefix(metadata, "data:")
	for i, part := range strings.Split(meta, ";") {
		if i == 0 && part != "" {
			mimeType = part
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return ImageInputMetadata{Source: ImageSourceDataURL, MIMEType: mimeType, URLRedacted: true}, true
	}
	hash := sha256.Sum256(decoded)
	return ImageInputMetadata{
		Source:     ImageSourceDataURL,
		MIMEType:   mimeType,
		SHA256:     hex.EncodeToString(hash[:]),
		ByteLength: len(decoded),
	}, true
}
