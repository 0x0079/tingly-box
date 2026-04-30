package onboarding

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/data"
)

// Handler serves the onboarding extraction endpoint.
type Handler struct {
	templateManager *data.TemplateManager
}

// NewHandler creates a new onboarding handler.
func NewHandler(tm *data.TemplateManager) *Handler {
	return &Handler{templateManager: tm}
}

// Extract parses an arbitrary text blob (env file, curl, snippet from docs,
// etc.) and returns ranked provider candidates. v1 uses a pure rule-based
// extractor — no LLM calls, no network.
func (h *Handler) Extract(c *gin.Context) {
	var req ExtractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ExtractResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	extractor := NewRuleExtractor(h.templateManager)
	candidates, warnings, err := extractor.Extract(c.Request.Context(), req.Input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ExtractResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: err.Error(),
				Type:    "extraction_error",
			},
		})
		return
	}

	if candidates == nil {
		candidates = []Candidate{}
	}

	c.JSON(http.StatusOK, ExtractResponse{
		Success: true,
		Data: &ExtractData{
			Candidates: candidates,
			Warnings:   warnings,
		},
	})
}
