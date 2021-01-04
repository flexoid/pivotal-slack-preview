package messages

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Logiraptor/go-pivotaltracker/v5/pivotal"
	"github.com/slack-go/slack"
)

func ExtractStoriesFromMessage(text string) []int {
	regex := regexp.MustCompile(`pivotaltracker.com(?:\/n)?\/(?:story\/show|projects\/\d+\/stories)\/(\d+)`)
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

func MessageForStories(stories []*pivotal.Story) slack.Message {
	var sections []slack.Block

	for _, story := range stories {
		headerText := slack.NewTextBlockObject(slack.MarkdownType, storyHeader(story), false, false)
		headerButton := slack.NewButtonBlockElement("expand", "", slack.NewTextBlockObject(slack.PlainTextType, "Expand", false, false))
		headerSection := slack.NewSectionBlock(headerText, nil, slack.NewAccessory(headerButton))
		sections = append(sections, headerSection)

		// titleText := slack.NewTextBlockObject(slack.MarkdownType, story.Name, false, false)
		// titleButton := slack.NewButtonBlockElement("expand", "", slack.NewTextBlockObject(slack.PlainTextType, "Expand", false, false))
		// titleSection := slack.NewSectionBlock(titleText, nil, slack.NewAccessory(titleButton))
		// sections = append(sections, titleSection)

		stateField := slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*State:*\n%s", story.State), false, false)
		labelsField := slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Labels:*\n%s", storyLabels(story)), false, false)

		fields := []*slack.TextBlockObject{stateField, labelsField}
		fieldsSection := slack.NewSectionBlock(nil, fields, nil)
		sections = append(sections, fieldsSection)

		// descriptionText := slack.NewTextBlockObject(slack.MarkdownType, story.Description, false, false)
		// descriptionSection := slack.NewSectionBlock(descriptionText, nil, nil)
		// sections = append(sections, descriptionSection)

		if len(stories) > 1 {
			sections = append(sections, slack.NewDividerBlock())
		}
	}

	return slack.NewBlockMessage(sections...)
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
