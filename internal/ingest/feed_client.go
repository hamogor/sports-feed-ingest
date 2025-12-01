package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

type ecbClient struct {
	baseURL string
	http    *http.Client
}

func NewECBClient(baseURL string, httpClient *http.Client) FeedClient {
	return &ecbClient{
		baseURL: baseURL,
		http:    httpClient,
	}
}

func (c *ecbClient) FetchPage(ctx context.Context, page, pageSize int) (ECBResponse, error) {
	u, _ := url.Parse(c.baseURL)
	q := u.Query()
	q.Set("page", strconv.Itoa(page))
	q.Set("pageSize", strconv.Itoa(pageSize))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return ECBResponse{}, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return ECBResponse{}, err
	}
	defer resp.Body.Close()

	var out ECBResponse
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out, err
}
