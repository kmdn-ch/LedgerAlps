package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/services/updatecheck"
	"github.com/kmdn-ch/ledgeralps/version"
)

// ComplianceHandler serves the compliance advisories the UI surfaces to users,
// and the update check that tells them to install a conforming release.
type ComplianceHandler struct {
	updates *updatecheck.Checker
}

func NewComplianceHandler(updates *updatecheck.Checker) *ComplianceHandler {
	return &ComplianceHandler{updates: updates}
}

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

// CheckForUpdate GET /api/v1/compliance/update-check
//
// Reports whether a newer release exists. This is the last link in the
// compliance chain: the CI watcher warns maintainers that a standard moved,
// they ship a conforming build, and this tells the user to install it —
// otherwise someone who never updates keeps issuing invoices banks reject.
//
// The check is cached, silent on failure, and switched off entirely by setting
// update_check to false. It transmits no identifiers and no user data.
func (h *ComplianceHandler) CheckForUpdate(c *gin.Context) {
	if h.updates == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "update_available": false})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	c.JSON(http.StatusOK, h.updates.Check(ctx, version.Version()))
}
