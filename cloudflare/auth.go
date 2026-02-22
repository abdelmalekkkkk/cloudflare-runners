package cloudflare

import (
	"errors"

	"github.com/zalando/go-keyring"
)

const service = "cf-runners"
const user = "cf-runners"

func LoadToken() (string, error) {
	token, err := keyring.Get(service, user)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", nil
		}
		return "", err
	}

	return token, nil
}

func SaveToken(token string) error {
	return keyring.Set(service, user, token)
}
