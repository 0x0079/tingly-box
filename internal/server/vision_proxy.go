package server

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const (
	visionProxyInstruction   = "Describe this image precisely for a downstream LLM. If the user's text below provides a question or focus, tailor the description toward it. Plain text, under 300 words."
	visionProxyCacheKey      = "vision_proxy_cache"
	defaultVisionProxyTimeMs = 15000
)

// visionProxyCache holds per-request image-hash → description results.
type visionProxyCache map[string]string

func (s *Server) getVisionProxyConfig(scenario typ.RuleScenario) *typ.VisionProxyConfig {
	sc := s.config.GetScenarioConfig(scenario)
	if sc == nil || !sc.VisionProxy.Enabled {
		return nil
	}
	return &sc.VisionProxy
}

// describeImage calls the configured vision proxy model and returns a text description.
// Results are cached within the request by image content hash to avoid duplicate calls.
func (s *Server) describeImage(c *gin.Context, cfg *typ.VisionProxyConfig, imageURL string, userText string) (string, error) {
	h := sha256.Sum256([]byte(imageURL))
	cacheKey := fmt.Sprintf("%x", h)

	var cache visionProxyCache
	if v, exists := c.Get(visionProxyCacheKey); exists {
		cache = v.(visionProxyCache)
	} else {
		cache = make(visionProxyCache)
		c.Set(visionProxyCacheKey, cache)
	}
	if desc, hit := cache[cacheKey]; hit {
		return desc, nil
	}

	provider, err := s.config.GetProviderByUUID(cfg.ProviderID)
	if err != nil {
		return "", fmt.Errorf("vision proxy provider not found: %w", err)
	}

	timeoutMs := cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultVisionProxyTimeMs
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	// Build minimal, context-isolated request: instruction + user text (optional) + image.
	// No system prompt and no conversation history to keep cost low.
	parts := []openai.ChatCompletionContentPartUnionParam{
		openai.TextContentPart(visionProxyInstruction),
	}
	if t := strings.TrimSpace(userText); t != "" {
		parts = append(parts, openai.TextContentPart(t))
	}
	parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: imageURL}))

	req := openai.ChatCompletionNewParams{
		Model:    cfg.Model,
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage(parts)},
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	wrapper := s.clientPool.GetOpenAIClient(ctx, provider, cfg.Model)
	fc := forwarding.NewForwardContext(ctx, provider).WithTimeout(timeout)
	resp, cancelFn, err := forwarding.ForwardOpenAIChat(fc, wrapper, &req)
	if cancelFn != nil {
		defer cancelFn()
	}
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("vision proxy returned no choices")
	}

	desc := resp.Choices[0].Message.Content
	cache[cacheKey] = desc
	return desc, nil
}

// applyVisionProxyOpenAI rewrites image_url content parts in an OpenAI request with text descriptions.
func (s *Server) applyVisionProxyOpenAI(c *gin.Context, req *protocol.OpenAIChatCompletionRequest, scenario typ.RuleScenario) {
	cfg := s.getVisionProxyConfig(scenario)
	if cfg == nil {
		return
	}

	for i := range req.Messages {
		msg := &req.Messages[i]
		if msg.OfUser == nil {
			continue
		}
		parts := msg.OfUser.Content.OfArrayOfContentParts
		if len(parts) == 0 {
			continue
		}

		userText := collectOpenAIUserText(parts)

		newParts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(parts))
		for _, p := range parts {
			if p.OfImageURL == nil {
				newParts = append(newParts, p)
				continue
			}
			imageURL := p.OfImageURL.ImageURL.URL
			desc, err := s.describeImage(c, cfg, imageURL, userText)
			if err != nil {
				logrus.WithError(err).Warn("vision proxy: failed to describe image, dropping part")
				continue
			}
			newParts = append(newParts, openai.TextContentPart("[Image description: "+desc+"]"))
		}
		msg.OfUser.Content.OfArrayOfContentParts = newParts
	}
}

// applyVisionProxyAnthropic rewrites image blocks in Anthropic (v1 or beta) requests with text descriptions.
func (s *Server) applyVisionProxyAnthropic(
	c *gin.Context,
	messages *protocol.AnthropicMessagesRequest,
	betaMessages *protocol.AnthropicBetaMessagesRequest,
	scenario typ.RuleScenario,
) {
	cfg := s.getVisionProxyConfig(scenario)
	if cfg == nil {
		return
	}

	if messages != nil {
		for i := range messages.Messages {
			msg := &messages.Messages[i]
			if string(msg.Role) != "user" {
				continue
			}
			userText := collectAnthropicUserText(msg.Content)
			newContent := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Content))
			for _, b := range msg.Content {
				if b.OfImage == nil {
					newContent = append(newContent, b)
					continue
				}
				imageURL := anthropicImageToURL(b.OfImage)
				if imageURL == "" {
					continue
				}
				desc, err := s.describeImage(c, cfg, imageURL, userText)
				if err != nil {
					logrus.WithError(err).Warn("vision proxy: failed to describe Anthropic image, dropping block")
					continue
				}
				newContent = append(newContent, anthropic.NewTextBlock("[Image description: "+desc+"]"))
			}
			msg.Content = newContent
		}
	}

	if betaMessages != nil {
		for i := range betaMessages.Messages {
			msg := &betaMessages.Messages[i]
			if string(msg.Role) != "user" {
				continue
			}
			userText := collectBetaAnthropicUserText(msg.Content)
			newContent := make([]anthropic.BetaContentBlockParamUnion, 0, len(msg.Content))
			for _, b := range msg.Content {
				if b.OfImage == nil {
					newContent = append(newContent, b)
					continue
				}
				imageURL := betaAnthropicImageToURL(b.OfImage)
				if imageURL == "" {
					continue
				}
				desc, err := s.describeImage(c, cfg, imageURL, userText)
				if err != nil {
					logrus.WithError(err).Warn("vision proxy: failed to describe Beta Anthropic image, dropping block")
					continue
				}
				newContent = append(newContent, anthropic.NewBetaTextBlock("[Image description: "+desc+"]"))
			}
			msg.Content = newContent
		}
	}
}

func collectOpenAIUserText(parts []openai.ChatCompletionContentPartUnionParam) string {
	var b strings.Builder
	for _, p := range parts {
		if p.OfText != nil {
			b.WriteString(p.OfText.Text)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

func collectAnthropicUserText(content []anthropic.ContentBlockParamUnion) string {
	var b strings.Builder
	for _, block := range content {
		if block.OfText != nil {
			b.WriteString(block.OfText.Text)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

func collectBetaAnthropicUserText(content []anthropic.BetaContentBlockParamUnion) string {
	var b strings.Builder
	for _, block := range content {
		if block.OfText != nil {
			b.WriteString(block.OfText.Text)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

func anthropicImageToURL(img *anthropic.ImageBlockParam) string {
	if img == nil {
		return ""
	}
	if img.Source.OfURL != nil {
		return img.Source.OfURL.URL
	}
	if img.Source.OfBase64 != nil {
		return "data:" + string(img.Source.OfBase64.MediaType) + ";base64," + img.Source.OfBase64.Data
	}
	return ""
}

func betaAnthropicImageToURL(img *anthropic.BetaImageBlockParam) string {
	if img == nil {
		return ""
	}
	if img.Source.OfURL != nil {
		return img.Source.OfURL.URL
	}
	if img.Source.OfBase64 != nil {
		return "data:" + string(img.Source.OfBase64.MediaType) + ";base64," + img.Source.OfBase64.Data
	}
	return ""
}
