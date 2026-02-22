package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/abdelmalekkkkk/cf-runners/cloudflare"
	"github.com/abdelmalekkkkk/cf-runners/github"
	"github.com/abdelmalekkkkk/cf-runners/input"
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

	_, err = cloudflare.CreateClient(ctx, cfToken)
	if err != nil {
		log.Fatal("An error occured while creating client: ", err)
	}

	fmt.Print("Please enter your organization slug, or leave empty if you want the app to be created in your personal account: ")

	organization, err := input.ReadLine(ctx)
	if err != nil {
		log.Fatal("An error occured: ", err)
	}

	// todo: pass worker url
	initializer := github.CreateInitializer(ctx, organization)

	app, err := initializer.Run()
	if err != nil {
		log.Fatal("An error occured: ", err)
	}

	fmt.Printf("Successfully created \"%s\" Github App. You can manage it from %s\n", app.Name, app.URL)
}
