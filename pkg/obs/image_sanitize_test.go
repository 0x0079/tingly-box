package obs

import (
	"strings"
	"testing"
)

func TestSanitizeBase64Images_ReplacesPayload(t *testing.T) {
	// Long enough that the placeholder is unambiguously shorter.
	payload := strings.Repeat("A", 4000)
	body := `{"image_url":{"url":"data:image/png;base64,` + payload + `"}}`

	out, saved := SanitizeBase64Images(body)
	if saved <= 0 {
		t.Fatalf("expected saved > 0, got %d", saved)
	}
	if strings.Contains(out, payload) {
		t.Fatal("expected base64 payload to be stripped")
	}
	if !strings.Contains(out, "data:image/png;base64,<omitted") {
		t.Fatalf("expected placeholder to mention mime + omitted, got: %s", out)
	}
}

func TestSanitizeBase64Images_NoMatch(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"hello"}]}`
	out, saved := SanitizeBase64Images(body)
	if saved != 0 || out != body {
		t.Fatalf("expected no-op, got saved=%d body=%s", saved, out)
	}
}

func TestSanitizeBase64Images_MultipleImages(t *testing.T) {
	p := strings.Repeat("Q", 2000)
	body := `[{"x":"data:image/jpeg;base64,` + p + `"},{"y":"data:image/webp;base64,` + p + `"}]`
	out, saved := SanitizeBase64Images(body)
	if saved <= 0 {
		t.Fatal("expected savings")
	}
	if strings.Count(out, "<omitted") != 2 {
		t.Fatalf("expected 2 placeholders, got: %s", out)
	}
}
