package obs

import (
	"fmt"
	"regexp"
	"strings"
)

// dataURLImageRe matches base64 image data URLs commonly embedded in
// chat-completion / messages request payloads (OpenAI image_url, Anthropic
// image source). It captures the mime subtype and replaces the base64
// payload with a short placeholder noting the original byte size.
//
// Pattern matches:   data:image/<subtype>;base64,<base64-bytes>
// up to the next double quote, which is the JSON string terminator.
var dataURLImageRe = regexp.MustCompile(`data:image/([a-zA-Z0-9.+-]+);base64,([^"\\]+)`)

// SanitizeBase64Images replaces base64 image data URLs in a JSON string
// with placeholders that retain the mime type and original size. The
// surrounding JSON structure (and any non-image content) is left
// untouched so existing tooling can still parse the body.
//
// This is intended for debug-only request capture: it strips out the
// large, opaque image payloads that dominate request size without
// contributing diagnostic value 99% of the time.
//
// Returns the sanitized body and the number of bytes saved.
func SanitizeBase64Images(body string) (string, int) {
	// Skip the regex scan for the common case (no inline images). A
	// substring search is ~free compared to the full regex over a multi-MB
	// JSON request.
	if !strings.Contains(body, "data:image/") {
		return body, 0
	}
	saved := 0
	out := dataURLImageRe.ReplaceAllStringFunc(body, func(match string) string {
		sub := dataURLImageRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		mime := sub[1]
		payloadLen := len(sub[2])
		replacement := fmt.Sprintf("data:image/%s;base64,<omitted %dB>", mime, payloadLen)
		saved += len(match) - len(replacement)
		return replacement
	})
	return out, saved
}
