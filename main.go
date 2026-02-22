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

	"github.com/abdelmalekkkkk/cf-runners/cloudflare"
	"github.com/abdelmalekkkkk/cf-runners/encryption"
	"github.com/abdelmalekkkkk/cf-runners/github"
	"github.com/abdelmalekkkkk/cf-runners/input"
	"github.com/abdelmalekkkkk/cf-runners/state"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChan
		cancel()
	}()

	cfToken, err := cloudflare.LoadToken()
	if err != nil {
		log.Fatal(err)
	}

	if cfToken == "" {
		fmt.Print("Please enter a Cloudflare Account API Token: ")

		cfToken, err = input.ReadLine(ctx)
		if err != nil {
			log.Fatal("An error occured: ", err)
		}

		err = cloudflare.SaveToken(cfToken)
		if err != nil {
			log.Fatal("An error occured: ", err)
		}
	}

	client, err := cloudflare.CreateClient(ctx, cfToken)
	if err != nil {
		log.Fatal("An error occured while creating client: ", err)
	}

	fmt.Println("Creating bucket to store state...")

	err = client.CreateBucket()
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

		secretStoreID, err := stateManager.GetSecretStoreID()
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

}
