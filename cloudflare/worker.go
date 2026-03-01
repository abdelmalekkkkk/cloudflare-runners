package cloudflare

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/workers"
	"github.com/tidwall/sjson"
)

//go:embed dist
var buildFS embed.FS

type WorkerDeployer struct {
	ctx         context.Context
	client      *cloudflare.Client
	accountID   string
	identifiers Identifiers
}

type WorkerDeployerParams struct {
	Client      *cloudflare.Client
	AccountID   string
	Environment Identifiers
}

func CreateWorkerDeployer(ctx context.Context, params WorkerDeployerParams) *WorkerDeployer {
	return &WorkerDeployer{
		ctx:         ctx,
		client:      params.Client,
		identifiers: params.Environment,
		accountID:   params.AccountID,
	}
}

type DeployWorkerParams struct {
	SecretStoreID string
}

func (d *WorkerDeployer) Deploy(params DeployWorkerParams) error {
	return nil
}

func (d *WorkerDeployer) activateSubdomain() error {
	_, err := d.client.Workers.Scripts.Subdomain.New(d.ctx, d.identifiers.WorkerName, workers.ScriptSubdomainNewParams{
		AccountID: cloudflare.String(d.accountID),
		Enabled:   cloudflare.Bool(true),
	})

	return err
}

func (d *WorkerDeployer) getVersionID() (string, error) {
	res, err := d.client.Workers.Scripts.Versions.List(d.ctx, d.identifiers.WorkerName, workers.ScriptVersionListParams{
		AccountID: cloudflare.String(d.accountID),
	})
	if err != nil {
		return "", err
	}

	if len(res.Result.Items) == 0 {
		return "", errors.New("worker versions list is empty")
	}

	return res.Result.Items[0].ID, nil
}

func (d *WorkerDeployer) getDoNamespaceID() (string, error) {
	versionID, err := d.getVersionID()
	if err != nil {
		return "", err
	}

	res, err := d.client.Workers.Scripts.Versions.Get(d.ctx, d.identifiers.WorkerName, versionID, workers.ScriptVersionGetParams{
		AccountID: cloudflare.String(d.accountID),
	})
	if err != nil {
		return "", err
	}

	namespaceID := ""

	for _, binding := range res.Resources.Bindings {
		if binding.Type != workers.ScriptVersionGetResponseResourcesBindingsTypeDurableObjectNamespace {
			continue
		}

		namespaceID = binding.NamespaceID
	}

	if namespaceID == "" {
		return "", errors.New("could not extract namespace_id from version")
	}

	return namespaceID, nil
}

func (d *WorkerDeployer) upload() error {
	doBinding := workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindDurableObjectNamespace{
		Type:      cloudflare.F(workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindDurableObjectNamespaceTypeDurableObjectNamespace),
		Name:      cloudflare.String("RUNNER"),
		ClassName: cloudflare.String("Runner"),
	}

	queueBinding := workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindQueue{
		Type:      cloudflare.F(workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindQueueTypeQueue),
		Name:      cloudflare.String("QUEUE"),
		QueueName: cloudflare.String(d.identifiers.QueueName),
	}

	bucketBinding := workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindR2Bucket{
		Type:       cloudflare.F(workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindR2BucketTypeR2Bucket),
		Name:       cloudflare.String("BUCKET"),
		BucketName: cloudflare.String(d.identifiers.BucketName),
	}

	secretBinding := workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindSecretsStoreSecret{
		Type:       cloudflare.F(workers.ScriptUpdateParamsMetadataBindingsWorkersBindingKindSecretsStoreSecretTypeSecretsStoreSecret),
		Name:       cloudflare.String("APP_KEY"),
		SecretName: cloudflare.String(d.identifiers.SecretName),
		StoreID:    cloudflare.String(d.identifiers.SecretStoreID),
	}

	metadata := workers.ScriptUpdateParamsMetadata{
		MainModule:         cloudflare.String("index.js"),
		Bindings:           cloudflare.F([]workers.ScriptUpdateParamsMetadataBindingUnion{doBinding, queueBinding, bucketBinding, secretBinding}),
		CompatibilityDate:  cloudflare.String("2026-02-19"),
		CompatibilityFlags: cloudflare.F([]string{"nodejs_compat"}),
		Migrations: cloudflare.F(workers.ScriptUpdateParamsMetadataMigrationsUnion(workers.SingleStepMigrationParam{
			NewTag:           cloudflare.String("v1"),
			NewSqliteClasses: cloudflare.F([]string{"Runner"}),
		})),
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
	err = d.client.Put(
		d.ctx,
		fmt.Sprintf("accounts/%s/workers/scripts/%s", d.accountID, d.identifiers.WorkerName),
		&buf,
		&env,
		option.WithHeader("content-type", writer.FormDataContentType()),
	)
	if err != nil {
		return err
	}

	return err
}
