package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/Logiraptor/go-pivotaltracker/v5/pivotal"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"

	"github.com/flexoid/slack-pivotalbot-go/internal/messages"
)

// You more than likely want your "Bot User OAuth Access Token" which starts with "xoxb-"
var api = slack.New(os.Getenv("SLACK_TOKEN"))

func main() {
	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")

	http.HandleFunc("/events-endpoint", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		sv, err := slack.NewSecretsVerifier(r.Header, signingSecret)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := sv.Write(body); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := sv.Ensure(); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if eventsAPIEvent.Type == slackevents.URLVerification {
			var r *slackevents.ChallengeResponse
			err := json.Unmarshal([]byte(body), &r)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text")
			w.Write([]byte(r.Challenge))
		}
		if eventsAPIEvent.Type == slackevents.CallbackEvent {
			innerEvent := eventsAPIEvent.InnerEvent
			switch ev := innerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				api.PostMessage(ev.Channel, slack.MsgOptionText("Yes, hello.", false))
			case *slackevents.MessageEvent:
				go HandleMessage(ev)
			}
		}
	})

	http.HandleFunc("/interactive-endpoint", func(w http.ResponseWriter, r *http.Request) {
		var payload slack.InteractionCallback
		err := json.Unmarshal([]byte(r.FormValue("payload")), &payload)
		if err != nil {
			fmt.Printf("Could not parse action response JSON: %v", err)
		}

		fmt.Printf("Payload: %+v\n", payload)
	})

	fmt.Println("[INFO] Server listening")
	http.ListenAndServe(":3000", nil)
}

func HandleMessage(event *slackevents.MessageEvent) {
	fmt.Printf("%+v\n", event)

	ids := messages.ExtractStoriesFromMessage(event.Text)
	fmt.Printf("%+v\n", ids)

	if len(ids) == 0 {
		return
	}

	client := pivotal.NewClient(os.Getenv("PIVOTAL_TOKEN"))
	story, _, err := client.Stories.GetByID(ids[0])
	if err != nil {
		fmt.Printf("%s", err)
		return
	}

	message := messages.MessageForStories([]*pivotal.Story{story})

	// var options []slack.MsgOption
	options := []slack.MsgOption{slack.MsgOptionBlocks(message.Blocks.BlockSet...)}

	if len(event.ThreadTimeStamp) > 0 {
		options = append(options, slack.MsgOptionTS(event.ThreadTimeStamp))
	}

	_, _, err = api.PostMessage(event.Channel, options...)
	if err != nil {
		fmt.Printf("%s", err)
		return
	}

	fmt.Printf("%+v\n", story)
}
