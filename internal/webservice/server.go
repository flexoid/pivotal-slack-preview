package webservice

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/Logiraptor/go-pivotaltracker/v5/pivotal"
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

	fmt.Println("[INFO] Server listening")
	http.ListenAndServe(":"+s.Port, nil)
}

func (s *Server) eventsHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sv, err := slack.NewSecretsVerifier(r.Header, s.SlackSigningSecret)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if _, err = sv.Write(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err = sv.Ensure(); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	s.handleSlackEvent(&eventsAPIEvent, body, w)
}

func (s *Server) interactiveHandler(w http.ResponseWriter, r *http.Request) {
	var payload slack.InteractionCallback
	err := json.Unmarshal([]byte(r.FormValue("payload")), &payload)

	if err != nil {
		fmt.Printf("Could not parse action response JSON: %v", err)
	}

	for _, blockAction := range payload.ActionCallback.BlockActions {
		if blockAction.ActionID == messages.ActionShowMore {
			s.handleExpandAction(payload, blockAction)
		}
	}
}

func (s *Server) handleSlackEvent(event *slackevents.EventsAPIEvent, body []byte, w http.ResponseWriter) {
	if event.Type == slackevents.URLVerification {
		var r *slackevents.ChallengeResponse
		err := json.Unmarshal(body, &r)

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text")
		w.Write([]byte(r.Challenge))
	}

	if event.Type == slackevents.CallbackEvent {
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.MessageEvent:
			go s.handleMessage(ev)
		}
	}
}

func (s *Server) handleMessage(event *slackevents.MessageEvent) {
	fmt.Printf("%+v\n", event)

	ids := messages.ExtractStoriesFromMessage(event.Text)
	fmt.Printf("%+v\n", ids)

	if len(ids) == 0 {
		return
	}

	var stories []*pivotal.Story

	for _, id := range ids {
		story, _, err := s.PivotalClient.Stories.GetByID(id)
		if err != nil {
			fmt.Printf("%s", err)
			continue
		}

		stories = append(stories, story)
	}

	if len(stories) == 0 {
		fmt.Printf("No stories are fetched successfully\n")
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
		fmt.Printf("%s", err)
		return
	}

	fmt.Printf("%+v\n", stories)
}

func (s *Server) handleExpandAction(payload slack.InteractionCallback, blockAction *slack.BlockAction) {
	pivotalStoryID, err := strconv.Atoi(blockAction.Value)
	if err != nil {
		fmt.Printf("Unexpected value for expand block action: %v", blockAction.Value)
	}

	story, _, err := s.PivotalClient.Stories.GetByID(pivotalStoryID)
	if err != nil {
		fmt.Printf("Failed to fetch story: %s", err)
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
		fmt.Printf("%s", err)
		return
	}
}
