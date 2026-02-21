package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/abdelmalekkkkk/cf-runners/github"
	"github.com/abdelmalekkkkk/cf-runners/input"
)

type InitPageParams struct {
	Organization string
	State        string
	ManifestJSON string
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChan
		cancel()
	}()

	fmt.Print("Please enter your organization slug, or leave empty if you want the app to be created in your personal account: ")

	organization, err := input.ReadLine(ctx)

	fmt.Println(organization)

	initializer := github.CreateInitializer(ctx, organization)

	err = initializer.Run(organization)

	if err != nil {
		log.Fatal("An error occured: ", err)
	}
}
