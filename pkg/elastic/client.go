package elastic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"

	"github.com/dominicluechinger/esqlorer/internal/config"
)

type Client struct {
	client *elasticsearch.Client
	cfg    config.Server
}

type QueryResult struct {
	Columns []Column
	Values  [][]interface{}
}

type Column struct {
	Name string
	Type string
}

type QueryOptions struct {
	Query           string
	From            string
	To              string
	DropNullColumns bool
}

func NewClient(cfg config.Server) (*Client, error) {
	esCfg := elasticsearch.Config{
		Addresses: []string{cfg.URL},
	}

	if cfg.APIKey != "" {
		esCfg.APIKey = cfg.APIKey
	} else if cfg.Username != "" {
		esCfg.Username = cfg.Username
		esCfg.Password = cfg.Password
	}

	client, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		return nil, err
	}

	return &Client{
		client: client,
		cfg:    cfg,
	}, nil
}

func (c *Client) ExecuteESQL(ctx context.Context, query string) (*QueryResult, error) {
	return c.ExecuteESQLWithOptions(ctx, QueryOptions{Query: query})
}

func (c *Client) ExecuteESQLWithOptions(ctx context.Context, opts QueryOptions) (*QueryResult, error) {
	body, err := buildESQLRequestBody(opts)
	if err != nil {
		return nil, err
	}

	req := buildESQLRequest(body, opts)

	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("query error: %s", res.String())
	}

	var response esqlQueryResponse
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, err
	}

	result := &QueryResult{
		Columns: make([]Column, len(response.Columns)),
		Values:  make([][]interface{}, len(response.Values)),
	}

	for i, col := range response.Columns {
		result.Columns[i] = Column{
			Name: col.Name,
			Type: col.Type,
		}
	}

	for i, row := range response.Values {
		result.Values[i] = row
	}

	return result, nil
}

func buildESQLRequest(body []byte, opts QueryOptions) esapi.EsqlQueryRequest {
	req := esapi.EsqlQueryRequest{
		Body:   strings.NewReader(string(body)),
		Format: "json",
	}

	if opts.DropNullColumns {
		dropNullColumns := true
		req.DropNullColumns = &dropNullColumns
	}

	return req
}

func buildESQLRequestBody(opts QueryOptions) ([]byte, error) {
	payload := map[string]any{
		"query": opts.Query,
	}

	if strings.TrimSpace(opts.From) != "" || strings.TrimSpace(opts.To) != "" {
		timeRange := map[string]any{}
		if strings.TrimSpace(opts.From) != "" {
			timeRange["gte"] = strings.TrimSpace(opts.From)
		}
		if strings.TrimSpace(opts.To) != "" {
			timeRange["lte"] = strings.TrimSpace(opts.To)
		}

		payload["filter"] = map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{
						"range": map[string]any{
							"@timestamp": timeRange,
						},
					},
				},
			},
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal query body: %w", err)
	}

	return body, nil
}

func (c *Client) Ping(ctx context.Context) error {
	req := esapi.PingRequest{}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("ping failed: %s", res.String())
	}

	return nil
}

type esqlQueryResponse struct {
	Columns []esqlColumn    `json:"columns"`
	Values  [][]interface{} `json:"values"`
}

type esqlColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
