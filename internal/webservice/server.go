package webservice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/Logiraptor/go-pivotaltracker/v5/pivotal"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/yfuruyama/crzerolog"

	"github.com/flexoid/pivotal-slack-preview/internal/messages"
)

type Server struct {
	Port               string
	SlackClient        *slack.Client
	SlackSigningSecret string
	PivotalClient      *pivotal.Client
	Logger             *zerolog.Logger
	StoriesCountToAsk  int
}

func (s *Server) Start() {
	// The `crzerolog` library calls `With().Timestamp()` to the provided logger
	// which leads to double timestamp in the logs. So providing clear log instance here.
	rootLogger := zerolog.New(os.Stdout)
	loggingMiddleware := crzerolog.InjectLogger(&rootLogger)

	http.Handle("/events-endpoint", loggingMiddleware(http.HandlerFunc(s.eventsHandler)))
	http.Handle("/interactive-endpoint", loggingMiddleware(http.HandlerFunc(s.interactiveHandler)))

	s.Logger.Info().Msgf("Listening on port %s", s.Port)

	err := http.ListenAndServe(":"+s.Port, nil)
	if err != nil {
		s.Logger.Fatal().Err(err).Msgf(err.Error())
	}
}

func (s *Server) eventsHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
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

	s.handleSlackEvent(r.Context(), &eventsAPIEvent, body, w)
}

func (s *Server) interactiveHandler(_ http.ResponseWriter, r *http.Request) {
	var payload slack.InteractionCallback
	err := json.Unmarshal([]byte(r.FormValue("payload")), &payload)

	if err != nil {
		log.Warn().Err(err).Msgf("Could not parse inveractive action payload")
		return
	}

	ctx := r.Context()
	log.Ctx(ctx).UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Str("trigger_id", payload.TriggerID)
	})

	for _, blockAction := range payload.ActionCallback.BlockActions {
		switch blockAction.ActionID {
		case messages.ActionShowMore:
			s.handleExpandAction(ctx, &payload, blockAction)
		case messages.ActionPostPreview:
			s.handlePostPreviewAction(ctx, &payload, blockAction)
		}
	}
}

func (s *Server) handleSlackEvent(ctx context.Context, event *slackevents.EventsAPIEvent, body []byte, w http.ResponseWriter) {
	if event.Type == slackevents.URLVerification {
		var r *slackevents.ChallengeResponse
		err := json.Unmarshal(body, &r)

		if err != nil {
			log.Warn().Err(err).Msg("Could not unmarshal slack challenge response body")
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Type", "text")

		_, err = w.Write([]byte(r.Challenge))
		if err != nil {
			log.Error().Err(err).Msg("Failed to write response")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}

	if event.Type == slackevents.CallbackEvent {
		var callbackEvent slackevents.EventsAPICallbackEvent
		err := json.Unmarshal(body, &callbackEvent)

		if err != nil {
			log.Warn().Err(err).Msg("Could not unmarshal slack callback event")
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		log.Ctx(ctx).UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str("event_id", callbackEvent.EventID)
		})

		innerEvent := event.InnerEvent
		if ev, ok := innerEvent.Data.(*slackevents.MessageEvent); ok {
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

	if s.needToAskForPreview(ids) {
		s.askForPreview(ctx, event, ids)
	} else {
		s.postPreview(ctx, ids, event.Channel, event.ThreadTimeStamp, "")
	}
}

func (s *Server) handleExpandAction(ctx context.Context, payload *slack.InteractionCallback, blockAction *slack.BlockAction) {
	pivotalStoryID, err := strconv.Atoi(blockAction.Value)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msgf("Unexpected value for expand block action: %v", blockAction.Value)
	}

	story, _, err := s.PivotalClient.Stories.GetByID(pivotalStoryID) //nolint:bodyclose // False positive
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msgf("Cannot fetch pivotal story %d", pivotalStoryID)
		return
	}

	messageOptions := messages.DescriptionMessage(story, payload.Message.ThreadTimestamp)

	_, err = s.SlackClient.PostEphemeral(payload.Channel.ID, payload.User.ID, messageOptions...)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msgf("Cannot post ephemeral slack message with more info")
		return
	}

	log.Ctx(ctx).Info().Msg("Ephemeral slack message with details is posted")
}

func (s *Server) handlePostPreviewAction(ctx context.Context, payload *slack.InteractionCallback, blockAction *slack.BlockAction) {
	var previewActionData messages.PreviewActionData

	err := json.Unmarshal([]byte(blockAction.Value), &previewActionData)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msgf("Cannot parse preview action data")
	}

	s.postPreview(ctx, previewActionData.StoryIDs, previewActionData.ChannelID,
		previewActionData.ThreadTimeStamp, payload.ResponseURL)
}

// `responseURL` is specified when preview is posted in response to interactive action
// and used to remove a message with interaction after posting.
// In other cases, can be empty string.
func (s *Server) postPreview(ctx context.Context, storyIDs []int, channel, threadTimeStamp, responseURL string) {
	var stories []*pivotal.Story

	for _, id := range storyIDs {
		story, _, err := s.PivotalClient.Stories.GetByID(id) //nolint:bodyclose // False positive
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

	messageOptions := messages.MessageForStories(stories, threadTimeStamp)

	if len(responseURL) > 0 {
		messageOptions = append(messageOptions, slack.MsgOptionDeleteOriginal(responseURL))
	}

	_, _, err := s.SlackClient.PostMessage(channel, messageOptions...)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msgf("Cannot post slack message in response to mentioned stories")
		return
	}

	log.Ctx(ctx).Info().Msg("Slack message with mentioned stories is posted")
}

func (s *Server) needToAskForPreview(ids []int) bool {
	return len(ids) >= s.StoriesCountToAsk
}

func (s *Server) askForPreview(ctx context.Context, event *slackevents.MessageEvent, ids []int) {
	messageOptions, err := messages.AskForPreviewMessage(event, ids, event.ThreadTimeStamp)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msg(err.Error())
		return
	}

	_, err = s.SlackClient.PostEphemeral(event.Channel, event.User, messageOptions...)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msgf("Cannot post ephemeral slack message to ask for preview")
		return
	}

	log.Ctx(ctx).Info().Msg("User is asked for the need of preview")
}
