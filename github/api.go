package github

import (
	"context"

	"github.com/google/go-github/v81/github"
)

type APIClient struct {
	client *github.Client
	ctx    context.Context
}

func CreateAPIClient(ctx context.Context, accessToken string) *APIClient {
	client := github.NewClient(nil).WithAuthToken(accessToken)
	return &APIClient{
		client,
		ctx,
	}
}

func (c *APIClient) RegisterWebhooks() error {
	_, _, err := c.client.Repositories.CreateHook(c.ctx, "abdelmalekkkkk", "cf-runners", &github.Hook{
		Name: new("web"),
		Config: &github.HookConfig{
			ContentType: new("json"),
			URL:         new(""),
			Secret:      new("so secret"),
		},
		Events: []string{"workflow_run"},
	})

	return err
}
