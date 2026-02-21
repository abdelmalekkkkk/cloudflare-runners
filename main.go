package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/abdelmalekkkkk/cf-runners/cloudflare"
	github "github.com/abdelmalekkkkk/cf-runners/github"
)

const configDir = "cf-runners"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChan
		cancel()
	}()

	baseDir, err := os.UserConfigDir()

	if err != nil {
		log.Fatal("An error occured", err)
	}

	fmt.Println("Verifying Github auth...")
	ghAuth := github.CreateAuthenticator(ctx, path.Join(baseDir, configDir))

	err = ghAuth.Authenticate()

	if err != nil {
		log.Fatal("An error occured", err)
	}

	accessToken, err := ghAuth.AccessToken()

	if err != nil {
		log.Fatal("An error occured", err)
	}

	ghClient := github.CreateAPIClient(ctx, accessToken.AccessToken)

	err = ghClient.RegisterWebhooks()

	if err != nil {
		log.Fatal("An error occured", err)
	}

	fmt.Println("Verifying Cloudflare auth...")
	cfAuth := cloudflare.CreateAuthenticator(ctx)

	err = cfAuth.Authenticate()

	if err != nil {
		log.Fatal("An error occured", err)
	}
}
