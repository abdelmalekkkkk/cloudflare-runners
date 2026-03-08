package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/abdelmalekkkkk/cloudflare-runners/cloudflare"
	"github.com/abdelmalekkkkk/cloudflare-runners/credentials"
	"github.com/abdelmalekkkkk/cloudflare-runners/encryption"
	"github.com/abdelmalekkkkk/cloudflare-runners/github"
	"github.com/abdelmalekkkkk/cloudflare-runners/input"
	"github.com/abdelmalekkkkk/cloudflare-runners/state"
)

const workerVersion = "1.0.0"
const bucketName = "cf-runners"
const workerName = "cf-runners-worker"
const secretName = "cf-runners-key"
const runnerImage = "abdelmalekkkkk/cloudflare-runners"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChan
		cancel()
	}()

	cfToken, err := credentials.LoadCloudflareToken()
	if err != nil {
		log.Fatal(err)
	}

	if cfToken == "" {
		fmt.Print("Please create a Cloudflare Account API Token with the following permissions:\n- Containers:Edit\n- Workers R2 Storage:Edit\n- Workers Scripts:Edit\n- Secret Store:Edit\n\nEnter the token: ")

		cfToken, err = input.ReadLine(ctx)
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

	}

	client, err := cloudflare.CreateClient(ctx, cloudflare.CreateClientParams{
		Token: cfToken,
		Identifiers: cloudflare.Identifiers{
			WorkerName: workerName,
			BucketName: bucketName,
			SecretName: secretName,
		},
	})
	if err != nil {
		log.Fatal("An error occured while creating client: ", err)
	}

	err = credentials.SaveCloudflareToken(cfToken)
	if err != nil {
		log.Fatal("An error occured: ", err)
	}

	err = client.EnsureBucket()
	if err != nil {
		log.Fatal("An error occured: ", err)
	}

	stateManager := state.CreateStateManager(ctx, client)

	workerID, err := stateManager.GetWorkerID()
	if err != nil {
		log.Fatal("An error occured: ", err)
	}

	if workerID == "" {
		fmt.Println("Creating the worker...")
		workerID, err = client.CreateWorker()
		if err != nil {
			if errors.Is(err, cloudflare.ErrWorkerExists) {
				log.Fatal("The Cloudflare Runners worker already exists, but it was not found in the state, which means the state is corrupted. Aborting.")
			}
			log.Fatal("An error occured: ", err)
		}

		err = stateManager.SetWorkerID(workerID)
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		fmt.Printf("Created worker with ID: %s\n", workerID)
	}

	secretStoreID, err := stateManager.GetSecretStoreID()
	if err != nil {
		log.Fatal("An error occured: ", err)
	}

	githubApp, err := stateManager.GetGithubApp()

	if githubApp == nil {
		fmt.Print("Please enter your organization slug, or leave empty if you want the app to be created in your personal account: ")

		organization, err := input.ReadLine(ctx)
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		workerURL, err := client.GetWorkerURL()
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		initializer := github.CreateInitializer(ctx, github.AppParams{
			Organization: organization,
			WebhookURL:   workerURL,
		})

		app, err := initializer.Run()
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		err = stateManager.SetGithubApp(state.GithubApp{
			ID:   app.ID,
			Name: app.Name,
			Slug: app.Slug,
			URL:  app.URL,
		})
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		if secretStoreID == "" {
			secretStoreID, err = client.GetSecretStoreID()
			if err != nil {
				log.Fatal("An error occured: ", err)
			}

			err = stateManager.SetSecretStoreID(secretStoreID)
			if err != nil {
				log.Fatal("An error occured: ", err)
			}
		}

		key := make([]byte, 32)

		_, err = rand.Read(key)
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		err = client.StoreKey(secretStoreID, hex.EncodeToString(key))
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		sealed, err := encryption.Encrypt(app.JSON, key)
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		encryptedConfig := fmt.Sprintf(
			"%s:%s:%s",
			hex.EncodeToString(sealed.Nonce),
			hex.EncodeToString(sealed.Ciphertext),
			hex.EncodeToString(sealed.Tag),
		)

		err = client.PutObject("app_config.json", []byte(encryptedConfig))
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		fmt.Printf("Successfully created \"%s\" Github App. You can manage it from %s\n", app.Name, app.URL)
	} else {
		fmt.Printf("Github app \"%s\" already exists. You can manage it from %s\n", githubApp.Name, githubApp.URL)
	}

	currentWorkerVersion, err := stateManager.GetWorkerVersion()
	if err != nil {
		log.Fatal("An error occured: ", err)
	}

	if currentWorkerVersion == "" {
		fmt.Println("Uploading worker script...")
		err = client.UploadWorker(cloudflare.UploadWorkerParams{
			SecretStoreID: secretStoreID,
		})
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		err = stateManager.SetWorkerVersion(workerVersion)
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		err = client.ActivateSubdomain()
		if err != nil {
			log.Fatal("An error occured: ", err)
		}
	}

	dockerImage, err := stateManager.GetDockerImage()
	if err != nil {
		log.Fatal("An error occured: ", err)
	}

	if dockerImage == "" {
		fmt.Println("Uploading Docker image...")
		dockerImage, err = client.CopyDockerImage(cloudflare.CopyDockerImageParams{
			Image: runnerImage,
		})
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		err = stateManager.SetDockerImage(dockerImage)
		if err != nil {
			log.Fatal("An error occured: ", err)
		}
	}

	containerID, err := stateManager.GetContainerID()
	if containerID == "" {
		fmt.Println("Setting up Containers...")
		containerID, err = client.CreateContainer(cloudflare.CreateContainerParams{
			Image:        dockerImage,
			InstanceType: "standard-4",
		})
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		err = stateManager.SetContainerID(containerID)
		if err != nil {
			log.Fatal("An error occured: ", err)
		}
	}

}
