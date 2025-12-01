package ingest

import (
	"time"

	"cortex-task/internal/article"
)

func MapECBToArticle(e ECBArticle) (article.Article, error) {
	return article.Article{
		ExternalID:   e.ID,
		Type:         e.Type,
		Title:        e.Title,
		Description:  e.Description,
		Date:         parseDate(e.Date),
		Location:     e.Location,
		Language:     e.Language,
		CanonicalURL: e.CanonicalURL,
		LastModified: parseUnixTime(e.LastModified),
		Body:         e.Body,
		Summary:      e.Summary,
		LeadMedia: article.LeadMedia{
			ID:           e.LeadMedia.ID,
			Type:         e.LeadMedia.Type,
			Title:        e.LeadMedia.Title,
			Date:         parseDate(e.LeadMedia.Date),
			Language:     e.LeadMedia.Language,
			ImageURL:     e.LeadMedia.ImageURL,
			LastModified: parseUnixTime(e.LeadMedia.LastModified),
		},
	}, nil
}

// parseDate example "2025-12-01T09:15:00Z"
func parseDate(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseUnixTime example `1764580500000`
func parseUnixTime(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	// detect milliseconds vs seconds
	if n > 9999999999 {
		return time.UnixMilli(n)
	}
	return time.Unix(n, 0)
}
