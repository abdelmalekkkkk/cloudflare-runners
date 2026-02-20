package cloudflare

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type Authenticator struct {
	ctx context.Context
}

func CreateAuthenticator(ctx context.Context) *Authenticator {
	return &Authenticator{
		ctx: ctx,
	}
}

func (a *Authenticator) Authenticate() error {
	exists, err := a.CheckWranglerAuth()

	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	fmt.Println("Intializing Cloudflare auth through Wrangler...")

	err = a.LoginWrangler()

	if err != nil {
		return err
	}

	return nil
}

func (a *Authenticator) CheckWranglerAuth() (bool, error) {
	// todo: check if node/npx is available
	cmd := exec.CommandContext(a.ctx, "npx", "--yes", "wrangler", "whoami")

	output, err := cmd.Output()

	if err != nil {
		return false, err
	}

	if bytes.ContainsRune(output, '👋') {
		return true, nil
	}

	return false, nil
}

func (a *Authenticator) LoginWrangler() error {
	return exec.CommandContext(a.ctx, "npx", "--yes", "wrangler", "login").Run()
}
