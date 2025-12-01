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
		Date:         parseTime(e.Date),
		Location:     e.Location,
		Language:     e.Language,
		CanonicalURL: e.CanonicalURL,
		LastModified: e.LastModified,
		Body:         e.Body,
		Summary:      e.Summary,
		LeadMedia: article.LeadMedia{
			ID:           e.LeadMedia.ID,
			Type:         e.LeadMedia.Type,
			Title:        e.LeadMedia.Title,
			Date:         parseTime(e.LeadMedia.Date),
			Language:     e.LeadMedia.Language,
			ImageURL:     e.LeadMedia.ImageURL,
			LastModified: e.LeadMedia.LastModified,
		},
	}, nil
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}
