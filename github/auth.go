package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/abdelmalekkkkk/cf-runners/input"
)

const clientID = "Ov23lidLqTUTi2tFQPeh"
const scope = "repo,workflow"
const grantType = "urn:ietf:params:oauth:grant-type:device_code"
const deviceLoginEndpoint = "https://github.com/login/device/code"
const accessTokenEndpoint = "https://github.com/login/oauth/access_token"

type DeviceLoginRequest struct {
	ClientID string `json:"client_id"`
	Scope    string `json:"scope"`
}

type DeviceLogin struct {
	DeviceCode      string
	ExpiresIn       int
	Interval        int
	UserCode        string
	VerificationURI string
}

type Authenticator struct {
	ctx         context.Context
	configDir   string
	deviceLogin *DeviceLogin
	accessToken *AccessToken
}

func CreateAuthenticator(ctx context.Context, configDir string) *Authenticator {
	return &Authenticator{
		ctx:       ctx,
		configDir: configDir,
	}
}

func (a *Authenticator) Authenticate() error {
	exists, err := a.Load()

	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	fmt.Println("Intializing Github auth...")
	err = a.StartDeviceLogin()
	if err != nil {
		return err
	}

	fmt.Printf("Please visit %s and enter the following code: %s\nPress enter once that's done to cotinue.\n", a.deviceLogin.VerificationURI, a.deviceLogin.UserCode)

	err = input.WaitForEnter(a.ctx)

	if err != nil {
		return err
	}

	fmt.Printf("\x1b[1A\x1b[K")

	err = a.GetAccessToken()

	if err != nil {
		return err
	}

	err = a.Save()

	if err != nil {
		return err
	}

	fmt.Println("Successfully authenticated Github.")

	return nil
}

func (a *Authenticator) StartDeviceLogin() error {
	deviceLogin := &DeviceLogin{}

	reqBody, err := json.Marshal(DeviceLoginRequest{
		ClientID: clientID,
		Scope:    scope,
	})

	if err != nil {
		return fmt.Errorf("error creating login request body: %w", err)
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost, deviceLoginEndpoint, bytes.NewReader(reqBody))

	if err != nil {
		return fmt.Errorf("error creating login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return fmt.Errorf("error sending login request: %w", err)
	}

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return fmt.Errorf("error reading login response: %w", err)
	}

	data, err := url.ParseQuery(string(body))

	if err != nil {
		return fmt.Errorf("error parsing login response: %w", err)
	}

	// todo: get expiresIn so we can stop polling after the code expires
	if deviceLogin.DeviceCode = data.Get("device_code"); deviceLogin.DeviceCode == "" {
		return errors.New("device_code is empty")
	}

	if deviceLogin.UserCode = data.Get("user_code"); deviceLogin.UserCode == "" {
		return errors.New("user_code is empty")
	}

	if deviceLogin.VerificationURI = data.Get("verification_uri"); deviceLogin.VerificationURI == "" {
		return errors.New("verification_uri is empty")
	}

	a.deviceLogin = deviceLogin

	return nil
}

type AccessTokenRequest struct {
	ClientID   string `json:"client_id"`
	DeviceCode string `json:"device_code"`
	GrantType  string `json:"grant_type"`
}

type AccessToken struct {
	AccessToken string
	Scope       string
	TokenType   string
}

type SlowDownError struct {
	interval int
}

func (e SlowDownError) Error() string {
	return "rate limit exceeded"
}

func (e SlowDownError) Interval() int {
	return e.interval
}

var ErrAuthorizationPending = errors.New("authorization is still pending")

func (a *Authenticator) GetAccessToken() error {
	accessToken := &AccessToken{}

	if a.deviceLogin == nil {
		return errors.New("device login is nil")
	}

	reqBody, err := json.Marshal(AccessTokenRequest{
		ClientID:   clientID,
		DeviceCode: a.deviceLogin.DeviceCode,
		GrantType:  grantType,
	})

	if err != nil {
		return fmt.Errorf("error creating access token body: %w", err)
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost, accessTokenEndpoint, bytes.NewReader(reqBody))

	if err != nil {
		return fmt.Errorf("error creating access token: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return fmt.Errorf("error sending access token: %w", err)
	}

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return fmt.Errorf("error reading access token response: %w", err)
	}

	data, err := url.ParseQuery(string(body))

	if err != nil {
		return fmt.Errorf("error parsing access token response: %w", err)
	}

	if error := data.Get("error"); error != "" {
		switch error {
		case "authorization_pending":
			return ErrAuthorizationPending
		case "slow_down":
			if interval, err := strconv.Atoi(data.Get("interval")); err != nil {
				return SlowDownError{
					interval: 60,
				}
			} else {
				return SlowDownError{
					interval,
				}
			}

		default:
			return fmt.Errorf("got an unknown error while getting access token: %s", error)
		}
	}

	if accessToken.AccessToken = data.Get("access_token"); accessToken.AccessToken == "" {
		return errors.New("access_token is empty")
	}

	if accessToken.Scope = data.Get("scope"); accessToken.Scope == "" {
		return errors.New("scope is empty")
	}

	if accessToken.TokenType = data.Get("token_type"); accessToken.TokenType == "" {
		return errors.New("token_type is empty")
	}

	a.accessToken = accessToken

	return nil
}

func (a *Authenticator) Save() error {
	if a.accessToken == nil {
		return errors.New("access token is not available")
	}

	return SaveAccessToken(a.configDir, a.accessToken)
}

func (a *Authenticator) Load() (bool, error) {
	accessToken, err := LoadAccessToken(a.configDir)

	if err != nil {
		return false, err
	}

	if accessToken == nil {
		return false, nil
	}

	// todo: make sure it's still valid
	a.accessToken = accessToken

	return true, nil
}

func (a *Authenticator) AccessToken() (*AccessToken, error) {
	if a.accessToken == nil {
		return nil, errors.New("access token is not available")
	}

	return a.accessToken, nil
}
