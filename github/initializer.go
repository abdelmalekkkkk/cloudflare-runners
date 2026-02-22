package github

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/pkg/browser"
)

type AppParams struct {
	WebhookURL   string
	Organization string
}

type Initializer struct {
	ctx    context.Context
	params AppParams
}

func CreateInitializer(ctx context.Context, params AppParams) *Initializer {
	return &Initializer{
		ctx,
		params,
	}
}

type App struct {
	ID       int    `json:"id"`
	ClientID string `json:"client_id"`
	Slug     string `json:"slug"`
	NodeID   string `json:"node_id"`
	Name     string `json:"name"`
	URL      string `json:"html_url"`
	JSON     []byte
}

func (i *Initializer) Run() (App, error) {
	app := App{}

	state := rand.Text()

	server := CreateSetupServer(i.ctx, templateParams{
		organization: i.params.Organization,
		webhookURL:   i.params.WebhookURL,
		state:        state,
	})

	if err := server.run(); err != nil {
		return app, err
	}

	url, err := server.url()
	if err != nil {
		return app, err
	}

	if err = browser.OpenURL(url); err != nil {
		return app, err
	}

	callback, err := server.waitForCallback()
	if err != nil {
		return app, err
	}

	err = server.stop()
	if err != nil {
		return app, err
	}

	if callback.state != state {
		return app, errors.New("invalid state")
	}

	req, err := http.NewRequestWithContext(i.ctx, http.MethodPost, fmt.Sprintf("https://api.github.com/app-manifests/%s/conversions", callback.code), nil)
	if err != nil {
		return app, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return app, err
	}

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return app, err
	}

	app.JSON = body

	err = json.Unmarshal(body, &app)

	if err != nil {
		return app, err
	}

	return app, nil
}
