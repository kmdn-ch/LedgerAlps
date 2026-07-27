package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/version"
)

// ComplianceHandler serves the compliance advisories the UI surfaces to users.
type ComplianceHandler struct{}

func NewComplianceHandler() *ComplianceHandler { return &ComplianceHandler{} }

// defaultHorizon is how far ahead an obligation is announced. Six months is
// enough to act on a legal deadline without nagging about it for two years —
// a banner shown too early is one users learn to dismiss unread.
const defaultHorizon = 180 * 24 * time.Hour

type advisoryResponse struct {
	ID            string `json:"id"`
	Domain        string `json:"domain"`
	Severity      string `json:"severity"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	SourceName    string `json:"source_name"`
	SourceURL     string `json:"source_url"`
	PublishedAt   string `json:"published_at,omitempty"`
	EffectiveFrom string `json:"effective_from,omitempty"`
}

// ListAdvisories GET /api/v1/compliance/advisories?lang=fr
//
// Returns advisories relevant to this build: those not already resolved by the
// running version and taking effect within the horizon. Served from the feed
// embedded in the binary, so it works with no network access.
func (h *ComplianceHandler) ListAdvisories(c *gin.Context) {
	lang := c.DefaultQuery("lang", "fr")

	feed, err := compliance.BundledFeed()
	if err != nil {
		// Never surface a half-parsed legal notice; report the fault instead.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "compliance feed unavailable"})
		return
	}

	relevant := feed.Relevant(version.Version(), time.Now(), defaultHorizon)

	items := make([]advisoryResponse, 0, len(relevant))
	for _, a := range relevant {
		items = append(items, advisoryResponse{
			ID:            a.ID,
			Domain:        a.Domain,
			Severity:      a.Severity,
			Title:         compliance.Localised(a.Title, lang),
			Body:          compliance.Localised(a.Body, lang),
			SourceName:    a.SourceName,
			SourceURL:     a.SourceURL,
			PublishedAt:   a.PublishedAt,
			EffectiveFrom: a.EffectiveFrom,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items":       items,
		"total":       len(items),
		"feed_date":   feed.GeneratedAt,
		"app_version": version.Version(),
		"lang":        lang,
	})
}
