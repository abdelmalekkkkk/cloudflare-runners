package github

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/pkg/browser"
)

type Initializer struct {
	ctx          context.Context
	organization string
}

func CreateInitializer(ctx context.Context, organization string) *Initializer {
	return &Initializer{
		ctx,
		organization,
	}
}

type app struct {
}

func (i *Initializer) Run(organization string) error {
	state := rand.Text()

	server := CreateSetupServer(i.ctx, organization, state)

	if err := server.run(); err != nil {
		return err
	}

	url, err := server.url()
	if err != nil {
		return err
	}

	if err = browser.OpenURL(url); err != nil {
		return err
	}

	callback, err := server.waitForCallback()
	if err != nil {
		return err
	}

	err = server.stop()
	if err != nil {
		return err
	}

	if callback.state != state {
		return errors.New("invalid state")
	}

	req, err := http.NewRequestWithContext(i.ctx, http.MethodPost, fmt.Sprintf("https://api.github.com/app-manifests/%s/conversions", callback.code), nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	app := new(map[string]any)

	err = json.NewDecoder(resp.Body).Decode(app)
	if err != nil {
		return err
	}

	fmt.Println(app)

	return nil
}
