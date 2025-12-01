package ingest

type ECBResponse struct {
	PageInfo PageInfo     `json:"pageInfo"`
	Content  []ECBArticle `json:"content"`
}

type PageInfo struct {
	Page       int `json:"page"`
	NumPages   int `json:"numPages"`
	PageSize   int `json:"pageSize"`
	NumEntries int `json:"numEntries"`
}

type ECBArticle struct {
	ID           int64        `json:"id"`
	Type         string       `json:"type"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	Date         string       `json:"date"`
	Location     string       `json:"location"`
	Language     string       `json:"language"`
	CanonicalURL string       `json:"canonicalUrl"`
	LastModified int64        `json:"lastModified"`
	Body         string       `json:"body"`
	Summary      string       `json:"summary"`
	LeadMedia    ECBLeadMedia `json:"leadMedia"`
}

type ECBLeadMedia struct {
	ID           int64  `json:"id"`
	Type         string `json:"type"`
	Title        string `json:"title"`
	Date         string `json:"date"`
	Language     string `json:"language"`
	ImageURL     string `json:"imageUrl"`
	LastModified int64  `json:"lastModified"`
}
