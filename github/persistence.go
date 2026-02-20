package github

import (
	"encoding/json"
	"os"
	"path"
)

const baseDir = "github"
const accessTokenFilename = "access_token"

func SaveAccessToken(configDir string, accessToken *AccessToken) error {
	dir := path.Join(configDir, baseDir)

	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	tokenPath := path.Join(dir, accessTokenFilename)

	content, err := json.Marshal(accessToken)

	if err != nil {
		return err
	}

	return os.WriteFile(tokenPath, content, 0600)
}

func LoadAccessToken(configDir string) (*AccessToken, error) {
	accessToken := &AccessToken{}

	path := path.Join(configDir, baseDir, accessTokenFilename)
	content, err := os.ReadFile(path)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return accessToken, err
	}

	if err = json.Unmarshal(content, accessToken); err != nil {
		return accessToken, err
	}

	return accessToken, nil
}
