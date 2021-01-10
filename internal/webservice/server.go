package webservice

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/Logiraptor/go-pivotaltracker/v5/pivotal"
	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"

	"github.com/flexoid/slack-pivotalbot-go/internal/messages"
)

type Server struct {
	Port               string
	SlackClient        *slack.Client
	SlackSigningSecret string
	PivotalClient      *pivotal.Client
}

func (s *Server) Start() {
	http.HandleFunc("/events-endpoint", s.eventsHandler)
	http.HandleFunc("/interactive-endpoint", s.interactiveHandler)

	log.Info().Msgf("Listening on port %s", s.Port)
	http.ListenAndServe(":"+s.Port, nil)
}

func (s *Server) eventsHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Warn().Err(err).Msg("Could not read event body")
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	sv, err := slack.NewSecretsVerifier(r.Header, s.SlackSigningSecret)
	if err != nil {
		log.Warn().Err(err).Msg("Could not initialize slack secrets verifier")
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	if _, err = sv.Write(body); err != nil {
		log.Warn().Err(err).Msg("Secrets verifier write failed")
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	if err = sv.Ensure(); err != nil {
		log.Warn().Err(err).Msg("Slack signature validation failed")
		w.WriteHeader(http.StatusUnauthorized)

		return
	}

	eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
	if err != nil {
		log.Warn().Err(err).Msg("Could not parse slack event")
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	s.handleSlackEvent(&eventsAPIEvent, body, w)
}

func (s *Server) interactiveHandler(w http.ResponseWriter, r *http.Request) {
	var payload slack.InteractionCallback
	err := json.Unmarshal([]byte(r.FormValue("payload")), &payload)

	if err != nil {
		log.Warn().Err(err).Msgf("Could not parse inveractive action payload")
		return
	}

	logger := log.With().Str("trigger_id", payload.TriggerID).Logger()
	ctx := logger.WithContext(context.TODO())

	for _, blockAction := range payload.ActionCallback.BlockActions {
		if blockAction.ActionID == messages.ActionShowMore {
			go s.handleExpandAction(ctx, &payload, blockAction)
		}
	}
}

func (s *Server) handleSlackEvent(event *slackevents.EventsAPIEvent, body []byte, w http.ResponseWriter) {
	if event.Type == slackevents.URLVerification {
		var r *slackevents.ChallengeResponse
		err := json.Unmarshal(body, &r)

		if err != nil {
			log.Warn().Err(err).Msg("Could not unmarshal slack challenge response body")
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Type", "text")
		w.Write([]byte(r.Challenge))
	}

	if event.Type == slackevents.CallbackEvent {
		var callbackEvent slackevents.EventsAPICallbackEvent
		err := json.Unmarshal(body, &callbackEvent)

		if err != nil {
			log.Warn().Err(err).Msg("Could not unmarshal slack callback event")
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		logger := log.With().Str("event_id", callbackEvent.EventID).Logger()
		ctx := logger.WithContext(context.TODO())

		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.MessageEvent:
			go s.handleMessage(ctx, ev)
		}
	}
}

func (s *Server) handleMessage(ctx context.Context, event *slackevents.MessageEvent) {
	ids := messages.ExtractStoriesFromMessage(event.Text)

	if len(ids) == 0 {
		return
	}

	log.Ctx(ctx).Info().Ints("stories", ids).Msg("Received message with pivotal stories mentioned")

	var stories []*pivotal.Story

	for _, id := range ids {
		story, _, err := s.PivotalClient.Stories.GetByID(id)
		if err != nil {
			log.Ctx(ctx).Warn().Err(err).Msgf("Cannot fetch pivotal story %d", id)
			continue
		}

		stories = append(stories, story)
	}

	if len(stories) == 0 {
		log.Ctx(ctx).Debug().Msg("No mentioned stories were fetched successfully, nothing to post")
		return
	}

	message := messages.MessageForStories(stories)
	options := []slack.MsgOption{slack.MsgOptionBlocks(message.Blocks.BlockSet...)}

	// Respond to thread if message is from thread.
	if len(event.ThreadTimeStamp) > 0 {
		options = append(options, slack.MsgOptionTS(event.ThreadTimeStamp))
	}

	_, _, err := s.SlackClient.PostMessage(event.Channel, options...)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msgf("Cannot post slack message in response to mentioned stories")
		return
	}

	log.Ctx(ctx).Info().Msg("Slack message with mentioned stories is posted")
}

func (s *Server) handleExpandAction(ctx context.Context, payload *slack.InteractionCallback, blockAction *slack.BlockAction) {
	pivotalStoryID, err := strconv.Atoi(blockAction.Value)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msgf("Unexpected value for expand block action: %v", blockAction.Value)
	}

	story, _, err := s.PivotalClient.Stories.GetByID(pivotalStoryID)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msgf("Cannot fetch pivotal story %d", pivotalStoryID)
		return
	}

	message := messages.DescriptionMessage(story)
	options := []slack.MsgOption{slack.MsgOptionBlocks(message.Blocks.BlockSet...)}

	// Respond to thread if message is from thread.
	if len(payload.Message.ThreadTimestamp) > 0 {
		options = append(options, slack.MsgOptionTS(payload.Message.ThreadTimestamp))
	}

	_, err = s.SlackClient.PostEphemeral(payload.Channel.ID, payload.User.ID, options...)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msgf("Cannot post ephemeral slack message with more info")
		return
	}

	log.Ctx(ctx).Info().Msg("Ephemeral slack message with details is posted")
}
