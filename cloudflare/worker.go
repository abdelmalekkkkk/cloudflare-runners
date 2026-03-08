package cloudflare

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/workers"
	"github.com/tidwall/sjson"
)

//go:embed dist
var buildFS embed.FS

func (c *Client) ActivateSubdomain() error {
	_, err := c.client.Workers.Scripts.Subdomain.New(c.ctx, c.identifiers.WorkerName, workers.ScriptSubdomainNewParams{
		AccountID: cloudflare.String(c.accountID),
		Enabled:   cloudflare.Bool(true),
	})

	return err
}

func (c *Client) getWorkerVersionID() (string, error) {
	res, err := c.client.Workers.Scripts.Versions.List(c.ctx, c.identifiers.WorkerName, workers.ScriptVersionListParams{
		AccountID: cloudflare.String(c.accountID),
	})
	if err != nil {
		return "", err
	}

	if len(res.Result.Items) == 0 {
		return "", errors.New("worker versions list is empty")
	}

	return res.Result.Items[0].ID, nil
}

func (c *Client) getRunnerDurableObjectNamespaceID() (string, error) {
	versionID, err := c.getWorkerVersionID()
	if err != nil {
		return "", err
	}

	res, err := c.client.Workers.Scripts.Versions.Get(c.ctx, c.identifiers.WorkerName, versionID, workers.ScriptVersionGetParams{
		AccountID: cloudflare.String(c.accountID),
	})
	if err != nil {
		return "", err
	}

	namespaceID := ""

	for _, binding := range res.Resources.Bindings {
		if binding.Type != workers.ScriptVersionGetResponseResourcesBindingsTypeDurableObjectNamespace || binding.ClassName != "Runner" {
			continue
		}

		namespaceID = binding.NamespaceID
	}

	if namespaceID == "" {
		return "", errors.New("could not extract namespace_id from version")
	}

	return namespaceID, nil
}

type logs struct {
	Enabled bool `json:"enabled"`
}

type observability struct {
	Logs logs `json:"logs"`
}

type containerConfiguration struct {
	Image         string        `json:"image"`
	InstanceType  string        `json:"instance_type"`
	Observability observability `json:"observability"`
}

type durableObject struct {
	NamespaceID string `json:"namespace_id"`
}

type createApplicationParams struct {
	Name             string                 `json:"name"`
	SchedulingPolicy string                 `json:"scheduling_policy"`
	Configuration    containerConfiguration `json:"configuration"`
	Instances        int                    `json:"instances"`
	MaxInstances     int                    `json:"max_instances"`
	DurableObjects   durableObject          `json:"durable_objects"`
}

type CreateContainerParams struct {
	Image        string
	InstanceType string
}

type Container struct {
	ID string `json:"id"`
}

type ContainerEnvelope struct {
	Success bool      `json:"success"`
	Result  Container `json:"result"`
}

func (c *Client) CreateContainer(params CreateContainerParams) (string, error) {
	doNamespaceID, err := c.getRunnerDurableObjectNamespaceID()
	if err != nil {
		return "", err
	}

	body := createApplicationParams{
		Name:             c.identifiers.WorkerName + "-container",
		SchedulingPolicy: "default",
		Configuration: containerConfiguration{
			Image:        params.Image,
			InstanceType: params.InstanceType,
			Observability: observability{
				Logs: logs{
					Enabled: true,
				},
			},
		},
		Instances:    0,
		MaxInstances: 100,
		DurableObjects: durableObject{
			NamespaceID: doNamespaceID,
		},
	}

	data, err := json.Marshal(&body)
	if err != nil {
		return "", err
	}

	var env ContainerEnvelope

	err = c.client.Post(
		c.ctx,
		fmt.Sprintf("accounts/%s/containers/applications", c.accountID),
		data,
		&env,
	)
	if err != nil {
		if isErrorStatus(err, http.StatusForbidden) {
			return "", errors.New("API token is missing the \"Cloudchamber:Edit\" permission")
		}
		return "", err
	}

	if !env.Success {
		return "", errors.New("unable to create container (application)")
	}

	return env.Result.ID, nil
}

type UploadWorkerParams struct {
	SecretStoreID string
}

func (c *Client) UploadWorker(params UploadWorkerParams) error {
	runnerDoBinding := workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindDurableObjectNamespace{
		Type:      cloudflare.F(workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindDurableObjectNamespaceTypeDurableObjectNamespace),
		Name:      cloudflare.String("RUNNER"),
		ClassName: cloudflare.String("Runner"),
	}

	orchestratorDoBinding := workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindDurableObjectNamespace{
		Type:      cloudflare.F(workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindDurableObjectNamespaceTypeDurableObjectNamespace),
		Name:      cloudflare.String("ORCHESTRATOR"),
		ClassName: cloudflare.String("Orchestrator"),
	}

	bucketBinding := workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindR2Bucket{
		Type:       cloudflare.F(workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindR2BucketTypeR2Bucket),
		Name:       cloudflare.String("BUCKET"),
		BucketName: cloudflare.String(c.identifiers.BucketName),
	}

	secretBinding := workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindSecretsStoreSecret{
		Type:       cloudflare.F(workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindSecretsStoreSecretTypeSecretsStoreSecret),
		Name:       cloudflare.String("APP_KEY"),
		SecretName: cloudflare.String(c.identifiers.SecretName),
		StoreID:    cloudflare.String(params.SecretStoreID),
	}

	migrations := workers.ScriptUpdateParamsMetadataMigrationsWorkersMultipleStepMigrations{
		NewTag: cloudflare.String("v1"),
		Steps: cloudflare.F([]workers.MigrationStepParam{
			{
				NewSqliteClasses: cloudflare.F([]string{"Runner", "Orchestrator"}),
			},
		}),
	}

	metadata := workers.ScriptUpdateParamsMetadata{
		MainModule:         cloudflare.String("index.js"),
		Bindings:           cloudflare.F([]workers.ScriptUpdateParamsMetadataBindingUnion{runnerDoBinding, orchestratorDoBinding, bucketBinding, secretBinding}),
		CompatibilityDate:  cloudflare.String("2026-02-19"),
		CompatibilityFlags: cloudflare.F([]string{"nodejs_compat"}),
		Migrations:         cloudflare.F(workers.ScriptUpdateParamsMetadataMigrationsUnion(migrations)),
		Observability: cloudflare.F(workers.ScriptUpdateParamsMetadataObservability{
			Enabled:          cloudflare.Bool(true),
			HeadSamplingRate: cloudflare.Float(1),
			Logs: cloudflare.F(workers.ScriptUpdateParamsMetadataObservabilityLogs{
				Enabled:          cloudflare.Bool(true),
				HeadSamplingRate: cloudflare.Float(1),
				Persist:          cloudflare.Bool(true),
				InvocationLogs:   cloudflare.Bool(true),
			}),
		}),
	}

	metadataJSON, err := metadata.MarshalJSON()
	if err != nil {
		return nil
	}

	// TODO: workaround, use built in structs once they're available
	metadataJSONWithContainers, err := sjson.SetRawBytes(metadataJSON, "containers", []byte("[{\"class_name\": \"Runner\"}]"))
	if err != nil {
		return nil
	}

	buf := bytes.Buffer{}
	writer := multipart.NewWriter(&buf)

	err = writer.WriteField("metadata", string(metadataJSONWithContainers))
	if err != nil {
		return nil
	}

	buildFiles, err := buildFS.ReadDir("dist")
	if err != nil {
		return err
	}

	for _, entry := range buildFiles {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", multipart.FileContentDisposition("files", entry.Name()))
		header.Set("Content-Type", "application/javascript+module")

		fileWriter, err := writer.CreatePart(header)
		if err != nil {
			return err
		}

		file, err := buildFS.Open("dist/" + entry.Name())
		if err != nil {
			return err
		}

		_, err = io.Copy(fileWriter, file)
		if err != nil {
			return err
		}
	}

	err = writer.Close()
	if err != nil {
		return err
	}

	var env workers.ScriptUpdateResponseEnvelope
	err = c.client.Put(
		c.ctx,
		fmt.Sprintf("accounts/%s/workers/scripts/%s", c.accountID, c.identifiers.WorkerName),
		&buf,
		&env,
		option.WithHeader("content-type", writer.FormDataContentType()),
	)
	if err != nil {
		return err
	}

	return nil
}
