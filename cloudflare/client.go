package cloudflare

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/accounts"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/workers"
)

type Client struct {
	ctx       context.Context
	client    *cloudflare.Client
	accountID string
}

func GetAccountID(ctx context.Context, client *cloudflare.Client, token string) (string, error) {
	res, err := client.Accounts.List(context.Background(), accounts.AccountListParams{})

	if err != nil {
		return "", err
	}

	if len(res.Result) == 0 {
		return "", errors.New("accounts list is empty")
	}

	return res.Result[0].ID, nil
}

func CreateClient(ctx context.Context, token string) (*Client, error) {
	client := cloudflare.NewClient(option.WithAPIToken(token))

	accountID, err := GetAccountID(ctx, client, token)

	if err != nil {
		return nil, err
	}

	return &Client{
		ctx,
		client,
		accountID,
	}, nil
}

func (c *Client) PutObject(path string, data []byte) error {
	endpoint := fmt.Sprintf("/accounts/%s/r2/buckets/cf-runners/objects/config.json", c.accountID)

	return c.client.Put(c.ctx, endpoint, bytes.NewReader(data), nil)
}

func (c *Client) CreateWorker() error {
	_, err := c.client.Workers.Beta.Workers.New(c.ctx, workers.BetaWorkerNewParams{
		AccountID: cloudflare.String(c.accountID),
		Worker: workers.WorkerParam{
			Name: cloudflare.String("cloudflare-runners-worker"),
			Subdomain: cloudflare.F(workers.WorkerSubdomainParam{
				Enabled: cloudflare.Bool(true),
			}),
		},
	})
	if err != nil {
		return err
	}

	return nil
}
