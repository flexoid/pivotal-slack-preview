package messages

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Logiraptor/go-pivotaltracker/v5/pivotal"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

const ActionShowMore = "show_more"
const ActionPostPreview = "post_preview"

// Strict is used to serialize data needed for preview posting
// in response on `post_preview` action.
type PreviewActionData struct {
	ChannelID       string `json:"channel"`
	ThreadTimeStamp string `json:"thread_ts"`
	StoryIDs        []int  `json:"ids"`
}

func ExtractStoriesFromMessage(text string) []int {
	regex := regexp.MustCompile(`pivotaltracker\.com(?:\/n)?\/(?:story\/show|projects\/\d+\/stories)\/(\d+)`)
	matches := regex.FindAllStringSubmatch(text, -1)

	var ids []int

	for _, match := range matches {
		matchID, err := strconv.Atoi(match[1])
		if err != nil {
			break
		}

		existing := false

		// Iterating over loop to find a duplicate should be efficient enough
		// as very short slices are expected here.
		for _, id := range ids {
			if id == matchID {
				existing = true
				break
			}
		}

		if !existing {
			ids = append(ids, matchID)
		}
	}

	return ids
}

func MessageForStories(stories []*pivotal.Story, threadTimeStamp string) []slack.MsgOption {
	var sections []slack.Block

	for _, story := range stories {
		headerText := slack.NewTextBlockObject(slack.MarkdownType, storyHeader(story), false, false)
		headerButton := slack.NewButtonBlockElement(ActionShowMore, fmt.Sprintf("%d", story.ID),
			slack.NewTextBlockObject(slack.PlainTextType, "Show more", false, false))
		headerSection := slack.NewSectionBlock(headerText, nil, slack.NewAccessory(headerButton))
		sections = append(sections, headerSection)

		stateField := slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*State:*\n%s", story.State), false, false)
		labelsField := slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Labels:*\n%s", storyLabels(story)), false, false)

		fields := []*slack.TextBlockObject{stateField, labelsField}
		fieldsSection := slack.NewSectionBlock(nil, fields, nil)
		sections = append(sections, fieldsSection)

		if len(stories) > 1 {
			sections = append(sections, slack.NewDividerBlock())
		}
	}

	message := slack.NewBlockMessage(sections...)

	return messageOptions(&message, threadTimeStamp)
}

func DescriptionMessage(story *pivotal.Story, threadTimeStamp string) []slack.MsgOption {
	var sections []slack.Block

	headerText := slack.NewTextBlockObject(slack.MarkdownType, storyHeader(story), false, false)
	sections = append(sections, slack.NewSectionBlock(headerText, nil, nil))

	if len(story.Description) > 0 {
		descriptionText := slack.NewTextBlockObject(slack.MarkdownType, story.Description, false, false)
		sections = append(sections, slack.NewSectionBlock(descriptionText, nil, nil))
	}

	message := slack.NewBlockMessage(sections...)

	return messageOptions(&message, threadTimeStamp)
}

func AskForPreviewMessage(event *slackevents.MessageEvent, ids []int, threadTimeStamp string) ([]slack.MsgOption, error) {
	var sections []slack.Block

	actionData := PreviewActionData{
		ChannelID:       event.Channel,
		StoryIDs:        ids,
		ThreadTimeStamp: event.ThreadTimeStamp,
	}
	actionValue, err := json.Marshal(actionData)

	if err != nil {
		return []slack.MsgOption{}, fmt.Errorf("cannot serialize action data: %v", err)
	}

	text := slack.NewTextBlockObject(slack.MarkdownType,
		"Last message refers to multiple stories, post a preview?", false, false)
	button := slack.NewButtonBlockElement(ActionPostPreview, string(actionValue),
		slack.NewTextBlockObject(slack.PlainTextType, "Post preview", false, false))
	sections = append(sections, slack.NewSectionBlock(text, nil, slack.NewAccessory(button)))
	message := slack.NewBlockMessage(sections...)

	return messageOptions(&message, threadTimeStamp), nil
}

func storyHeader(story *pivotal.Story) string {
	return fmt.Sprintf("*<%s|%s #%d>*\n%s", story.URL, storyEmoji(story), story.ID, story.Name)
}

func storyEmoji(story *pivotal.Story) string {
	var emoji string

	switch story.Type {
	case "feature":
		emoji = ":star:"
	case "bug":
		emoji = ":beetle:"
	case "chore":
		emoji = ":gear:"
	case "release":
		emoji = ":checkered_flag:"
	}

	return emoji
}

func storyLabels(story *pivotal.Story) string {
	var names []string
	for _, label := range story.Labels {
		names = append(names, label.Name)
	}

	return strings.Join(names, ", ")
}

func messageOptions(message *slack.Message, threadTimeStamp string) []slack.MsgOption {
	options := []slack.MsgOption{slack.MsgOptionBlocks(message.Blocks.BlockSet...)}

	// Respond to a thread if message is from it.
	if len(threadTimeStamp) > 0 {
		options = append(options, slack.MsgOptionTS(threadTimeStamp))
	}

	return options
}
