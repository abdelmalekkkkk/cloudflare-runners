package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/sync/errgroup"
)

const dockerRegistryBase = "https://registry-1.docker.io/v2"

type copier struct {
	ctx              context.Context
	cloudflareClient *Client

	dockerImageName     string
	cloudflareImageName string

	dockerToken           string
	cloudflareCredentials RegistryCredentials
}

type CopierParams struct {
	CloudflareClient    *Client
	DockerImageName     string
	CloudflareImageName string
}

func createCopier(ctx context.Context, params CopierParams) *copier {
	return &copier{
		ctx:                 ctx,
		cloudflareClient:    params.CloudflareClient,
		dockerImageName:     params.DockerImageName,
		cloudflareImageName: params.CloudflareImageName,
	}
}

func (c *copier) copy() (string, error) {
	err := c.authenticateDockerRegistry()
	if err != nil {
		return "", err
	}

	err = c.authenticateCloudflareRegistry()
	if err != nil {
		return "", err
	}

	manifest, err := c.getManifest()
	if err != nil {
		return "", err
	}

	g := new(errgroup.Group)

	g.Go(func() error {
		return c.copyBlob(manifest.Config)
	})
	for _, layer := range manifest.Layers {
		g.Go(func() error {
			return c.copyBlob(layer)
		})
	}

	err = g.Wait()
	if err != nil {
		return "", err
	}

	imageID, err := c.imageID(manifest.Digest)
	if err != nil {
		return "", err
	}

	err = c.writeManifest(manifest, imageID)
	if err != nil {
		return "", err
	}

	return imageID, nil
}

func (c *copier) imageID(digest string) (string, error) {
	id, found := strings.CutPrefix(digest, "sha256:")

	if !found {
		return "", errors.New("could not extract image id from digest")
	}

	return id, nil
}

func (c *copier) writeManifest(manifest dockerManifest, imageID string) error {
	endpoint := fmt.Sprintf("https://%s/v2/%s/%s/manifests/%s", c.cloudflareCredentials.RegistryHost, c.cloudflareCredentials.AccountID, c.cloudflareImageName, imageID)

	buf := bytes.NewBuffer(nil)

	err := json.NewEncoder(buf).Encode(&manifest)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(c.ctx, http.MethodPut, endpoint, buf)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.cloudflareCredentials.Username, c.cloudflareCredentials.Password)
	req.Header.Set("content-type", "application/vnd.docker.distribution.manifest.v2+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated {
		return errors.New("unable to write the manifest to cloudflare registry")
	}

	return nil
}

func (c *copier) copyBlob(blob dockerBlobMetadata) error {
	exists, err := c.blobExists(blob.Digest)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	dockerEndpoint := fmt.Sprintf("%s/%s/blobs/%s", dockerRegistryBase, c.dockerImageName, blob.Digest)
	cloudflareEndpoint, err := c.getBlobUploadURL(blob.Digest)
	if err != nil {
		return err
	}

	dockerReq, err := http.NewRequestWithContext(c.ctx, http.MethodGet, dockerEndpoint, nil)
	if err != nil {
		return err
	}
	dockerReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.dockerToken))

	dockerResp, err := http.DefaultClient.Do(dockerReq)
	if err != nil {
		return err
	}

	cloudflareReq, err := http.NewRequestWithContext(c.ctx, http.MethodPut, cloudflareEndpoint, dockerResp.Body)
	if err != nil {
		return err
	}
	cloudflareReq.SetBasicAuth(c.cloudflareCredentials.Username, c.cloudflareCredentials.Password)

	cloudflareResp, err := http.DefaultClient.Do(cloudflareReq)
	if err != nil {
		return err
	}

	if cloudflareResp.StatusCode != http.StatusCreated {
		return errors.New("unable to upload blob to cloudflare registry")
	}

	return nil
}

func (c *copier) blobExists(digest string) (bool, error) {
	endpoint := fmt.Sprintf("https://%s/v2/%s/%s/blobs/%s", c.cloudflareCredentials.RegistryHost, c.cloudflareCredentials.AccountID, c.cloudflareImageName, digest)

	req, err := http.NewRequestWithContext(c.ctx, http.MethodHead, endpoint, nil)
	if err != nil {
		return false, err
	}

	req.SetBasicAuth(c.cloudflareCredentials.Username, c.cloudflareCredentials.Password)

	resp, err := http.DefaultClient.Do(req)
	return resp.StatusCode == http.StatusOK, err
}

func (c *copier) getBlobUploadURL(digest string) (string, error) {
	endpoint := fmt.Sprintf("https://%s/v2/%s/%s/blobs/uploads", c.cloudflareCredentials.RegistryHost, c.cloudflareCredentials.AccountID, c.cloudflareImageName)

	req, err := http.NewRequestWithContext(c.ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(c.cloudflareCredentials.Username, c.cloudflareCredentials.Password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusAccepted {
		return "", errors.New("could not initiate blob upload")
	}

	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		return "", errors.New("could not initiate blob upload")
	}
	location.Scheme = "https"
	location.Host = c.cloudflareCredentials.RegistryHost
	query := location.Query()
	query.Add("digest", digest)
	location.RawQuery = query.Encode()

	return location.String(), nil
}

type dockerBlobMetadata struct {
	MediaType string `json:"mediaType"`
	Size      int    `json:"size"`
	Digest    string `json:"digest"`
}

type dockerManifest struct {
	Digest        string               `json:"-"`
	SchemaVersion int                  `json:"schemaVersion"`
	MediaType     string               `json:"mediaType"`
	Config        dockerBlobMetadata   `json:"config"`
	Layers        []dockerBlobMetadata `json:"layers"`
}

func (c *copier) getManifest() (dockerManifest, error) {
	var manifest dockerManifest

	endpoint := fmt.Sprintf("%s/%s/manifests/latest", dockerRegistryBase, c.dockerImageName)
	req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return manifest, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.dockerToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return manifest, err
	}

	if resp.StatusCode != http.StatusOK {
		return manifest, errors.New("unable to retrieve the latest image manifest")
	}

	err = json.NewDecoder(resp.Body).Decode(&manifest)
	if err != nil {
		return manifest, err
	}

	manifest.Digest = resp.Header.Get("docker-content-digest")
	if manifest.Digest == "" {
		return manifest, errors.New("docker manifest has no digest header")
	}

	return manifest, nil
}

func (c *copier) authenticateCloudflareRegistry() (err error) {
	c.cloudflareCredentials, err = c.cloudflareClient.GetRegistryCredentials()
	return
}

type dockerTokens struct {
	Token string `json:"token"`
}

func (c *copier) authenticateDockerRegistry() error {
	endpoint := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", c.dockerImageName)
	req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errors.New("unable to authenticate to the docker registry")
	}

	var res dockerTokens

	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return err
	}

	c.dockerToken = res.Token

	return nil
}
