package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/accounts"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/r2"
	"github.com/cloudflare/cloudflare-go/v6/secrets_store"
	"github.com/cloudflare/cloudflare-go/v6/workers"
)

type Identifiers struct {
	WorkerName string
	BucketName string
	SecretName string
}

type Client struct {
	ctx         context.Context
	client      *cloudflare.Client
	accountID   string
	identifiers Identifiers
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

type CreateClientParams struct {
	Token       string
	Identifiers Identifiers
}

func CreateClient(ctx context.Context, params CreateClientParams) (*Client, error) {
	client := cloudflare.NewClient(option.WithAPIToken(params.Token), option.WithMaxRetries(0))

	accountID, err := GetAccountID(ctx, client, params.Token)

	if err != nil {
		return nil, err
	}

	environment := params.Identifiers

	return &Client{
		ctx,
		client,
		accountID,
		environment,
	}, nil
}

func isErrorStatus(err error, status int) bool {
	apiErr, ok := errors.AsType[*cloudflare.Error](err)
	return ok && apiErr.StatusCode == status
}

// error is nil if the bucket already exists
func (c *Client) EnsureBucket() error {
	_, err := c.client.R2.Buckets.New(c.ctx, r2.BucketNewParams{
		AccountID: cloudflare.String(c.accountID),
		Name:      cloudflare.String(c.identifiers.BucketName),
	})

	if err != nil {
		if isErrorStatus(err, http.StatusForbidden) {
			return errors.New("API token is missing the \"Workers R2 Storage:Edit\" permission")
		}

		if isErrorStatus(err, http.StatusConflict) {
			return nil
		}

		return err
	}

	return nil
}

func (c *Client) PutObject(path string, data []byte) error {
	endpoint := fmt.Sprintf("/accounts/%s/r2/buckets/%s/objects/%s", c.accountID, c.identifiers.BucketName, path)

	return c.client.Put(c.ctx, endpoint, data, nil)
}

func (c *Client) GetObject(path string) ([]byte, error) {
	endpoint := fmt.Sprintf("/accounts/%s/r2/buckets/%s/objects/%s", c.accountID, c.identifiers.BucketName, path)

	resp := &http.Response{}

	err := c.client.Get(c.ctx, endpoint, nil, nil, option.WithResponseInto(&resp))
	if err != nil {
		if apiErr, ok := errors.AsType[*cloudflare.Error](err); ok && apiErr.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

var ErrWorkerExists = errors.New("cf runners worker already exists")

func (c *Client) CreateWorker() (string, error) {
	res, err := c.client.Workers.Beta.Workers.New(c.ctx, workers.BetaWorkerNewParams{
		AccountID: cloudflare.String(c.accountID),
		Worker: workers.WorkerParam{
			Name: cloudflare.String(c.identifiers.WorkerName),
			Subdomain: cloudflare.F(workers.WorkerSubdomainParam{
				Enabled: cloudflare.Bool(true),
			}),
		},
	})
	if err != nil {
		if isErrorStatus(err, http.StatusForbidden) {
			return "", errors.New("API token is missing the \"Workers Scripts:Edit\" permission")
		}
		if isErrorStatus(err, http.StatusConflict) {
			return "", ErrWorkerExists
		}
		return "", err
	}

	return res.ID, nil
}

func (c *Client) GetWorkerURL() (string, error) {
	res, err := c.client.Workers.Subdomains.Get(c.ctx, workers.SubdomainGetParams{
		AccountID: cloudflare.String(c.accountID),
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("https://%s.%s.workers.dev", c.identifiers.WorkerName, res.Subdomain), err
}

func (c *Client) GetSecretStoreID() (string, error) {
	res, err := c.client.SecretsStore.Stores.List(c.ctx, secrets_store.StoreListParams{
		AccountID: cloudflare.String(c.accountID),
	})

	if err != nil {
		if isErrorStatus(err, http.StatusForbidden) {
			return "", errors.New("API token is missing the \"Secret Store:Edit\" permission")
		}
		return "", err
	}

	if len(res.Result) == 0 {
		return "", errors.New("secret stores list is empty")
	}

	return res.Result[0].ID, nil
}

func (c *Client) StoreKey(storeID string, key string) error {
	_, err := c.client.SecretsStore.Stores.Secrets.New(c.ctx, storeID, secrets_store.StoreSecretNewParams{
		AccountID: cloudflare.String(c.accountID),
		Body: []secrets_store.StoreSecretNewParamsBody{
			{
				Name:    cloudflare.String(c.identifiers.SecretName),
				Scopes:  cloudflare.F([]string{"workers"}),
				Value:   cloudflare.String(key),
				Comment: cloudflare.String("Generated and managed by Cloudflare Runners"),
			},
		},
	})

	if isErrorStatus(err, http.StatusForbidden) {
		return errors.New("API token is missing the \"Secret Store:Edit\" permission")
	}

	return err
}

type RegistryCredentials struct {
	AccountID    string `json:"account_id"`
	RegistryHost string `json:"registry_host"`
	Username     string `json:"username"`
	Password     string `json:"password"`
}

type RegistryCredentialsEnvelope struct {
	Success bool                `json:"success"`
	Result  RegistryCredentials `json:"result"`
}

func (c *Client) GetRegistryCredentials() (RegistryCredentials, error) {
	endpoint := fmt.Sprintf("/accounts/%s/containers/registries/registry.cloudflare.com/credentials", c.accountID)

	var env RegistryCredentialsEnvelope

	err := c.client.Post(
		c.ctx,
		endpoint,
		[]byte("{ \"expiration_minutes\": 15000, \"permissions\": [ \"push\", \"pull\" ] }"),
		&env,
	)

	if isErrorStatus(err, http.StatusForbidden) {
		return env.Result, errors.New("API token is missing the \"Containers:Edit\" permission")
	}
	if !env.Success {
		return env.Result, errors.New("unable to get registry credentials")
	}

	return env.Result, err
}

type CopyDockerImageParams struct {
	Image string
}

func (c *Client) CopyDockerImage(params CopyDockerImageParams) (string, error) {
	copier := createCopier(c.ctx, CopierParams{
		CloudflareClient:    c,
		DockerImageName:     params.Image,
		CloudflareImageName: c.identifiers.WorkerName,
	})

	imageID, err := copier.copy()
	if err != nil {
		return "", err
	}

	image := fmt.Sprintf("registry.cloudflare.com/%s/%s:%s", c.accountID, c.identifiers.WorkerName, imageID)
	return image, nil
}
