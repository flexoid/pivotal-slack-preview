package main

import (
	"log"
	"os"

	"github.com/Logiraptor/go-pivotaltracker/v5/pivotal"
	"github.com/slack-go/slack"

	"github.com/flexoid/slack-pivotalbot-go/internal/webservice"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("defaulting to port %s", port)
	}

	pivotalClient := pivotal.NewClient(os.Getenv("PIVOTAL_TOKEN"))

	slackClient := slack.New(os.Getenv("SLACK_TOKEN"))
	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")

	server := webservice.Server{
		Port:               port,
		SlackClient:        slackClient,
		SlackSigningSecret: signingSecret,
		PivotalClient:      pivotalClient,
	}

	server.Start()
}
