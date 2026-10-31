package protocol

import (
	"encoding/base64"
	"io"
	"net/url"
	"strings"
)

// Preserve ordered text/image tool results and user input without fetching URLs
// or decoding image bytes into another representation. Files, audio, unknown
// blocks and nonempty annotations/logprobs remain unsupported.
func anthropicContent(value any) (any, error) {
	parts, ok := value.([]any)
	if !ok {
		return text(value)
	}
	blocks := make([]any, 0, len(parts))
	hasImage := false
	var combined strings.Builder
	for _, value := range parts {
		part := obj(value)
		if part["type"] == "input_image" {
			if allowed(part, "type image_url detail") != nil {
				return nil, ErrUnsupported
			}
			if detail := part["detail"]; detail != nil && detail != "auto" && detail != "low" && detail != "high" && detail != "original" {
				return nil, ErrUnsupported
			}
			imageURL, ok := part["image_url"].(string)
			if !ok || imageURL == "" {
				return nil, ErrUnsupported
			}
			source := Object{}
			if strings.HasPrefix(imageURL, "data:") {
				header, data, found := strings.Cut(imageURL, ",")
				if !found || data == "" || !strings.HasSuffix(header, ";base64") {
					return nil, ErrUnsupported
				}
				media := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
				switch media {
				case "image/png", "image/jpeg", "image/gif", "image/webp":
				default:
					return nil, ErrUnsupported
				}
				if _, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(data))); err != nil {
					return nil, ErrUnsupported
				}
				source = Object{"type": "base64", "media_type": media, "data": data}
			} else {
				parsed, err := url.Parse(imageURL)
				if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
					return nil, ErrUnsupported
				}
				source = Object{"type": "url", "url": imageURL}
			}
			blocks = append(blocks, Object{"type": "image", "source": source})
			hasImage = true
			continue
		}
		content, err := text([]any{value})
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, Object{"type": "text", "text": content})
		combined.WriteString(content)
	}
	if !hasImage {
		return combined.String(), nil
	}
	return blocks, nil
}

func anthropicImageDetailLost(value any) bool {
	for _, value := range arr(value) {
		part := obj(value)
		if part["type"] == "input_image" && part["detail"] != nil && part["detail"] != "auto" {
			return true
		}
	}
	return false
}
