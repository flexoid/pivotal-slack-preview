package main

import (
	"os"
	"strconv"

	"github.com/Logiraptor/go-pivotaltracker/v5/pivotal"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"

	"github.com/flexoid/pivotal-slack-preview/internal/webservice"
)

const defaultPort = "8080"
const defaultStoriesCountToAsk = 2

var version = "vX.Y.Z"

func main() {
	logger := setupLogger()

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
		log.Debug().Msgf("defaulting to port %s", port)
	}

	pivotalClient := pivotal.NewClient(os.Getenv("PIVOTAL_TOKEN"))

	slackClient := slack.New(os.Getenv("SLACK_TOKEN"))
	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")

	storiesCountToAsk := defaultStoriesCountToAsk
	if count, err := strconv.Atoi(os.Getenv("STORIES_COUNT_TO_ASK")); err == nil {
		storiesCountToAsk = count
	}

	server := webservice.Server{
		Port:               port,
		SlackClient:        slackClient,
		SlackSigningSecret: signingSecret,
		PivotalClient:      pivotalClient,
		Logger:             &logger,
		StoriesCountToAsk:  storiesCountToAsk,
	}

	logger.Info().Msgf("Starting slack-pivotalbot %s", version)

	server.Start()
}

func setupLogger() zerolog.Logger {
	return zerolog.New(os.Stdout).With().Timestamp().Logger()
}
