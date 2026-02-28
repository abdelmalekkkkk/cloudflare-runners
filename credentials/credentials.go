package credentials

import (
	"errors"

	"github.com/zalando/go-keyring"
)

const cloudflareService = "cf-runners"
const cloudflareUser = "cf-runners"

func LoadCloudflareToken() (string, error) {
	token, err := keyring.Get(cloudflareService, cloudflareUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", nil
		}
		return "", err
	}

	return token, nil
}

func SaveCloudflareToken(token string) error {
	return keyring.Set(cloudflareService, cloudflareUser, token)
}
