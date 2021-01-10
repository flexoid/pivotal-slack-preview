package main

import (
	"os"
	"time"

	"github.com/Logiraptor/go-pivotaltracker/v5/pivotal"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"

	"github.com/flexoid/slack-pivotalbot-go/internal/webservice"
)

var version = "vX.Y.Z"

func main() {
	setupLogger()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Debug().Msgf("defaulting to port %s", port)
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

	log.Info().Msgf("Starting slack-pivotalbot %s", version)

	server.Start()
}

func setupLogger() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().UTC()
	}
}
