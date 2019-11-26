package main

import (
	b64 "encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jeffreylo/cronofy"
	"github.com/mattermost/mattermost-server/model"
	"github.com/pkg/errors"
)

func getRandomHash() ([]byte, error) {
	return []byte("random hash"), nil
}

const DEFAULT_DATE_FORMAT = "Monday January 02"
const DEFAULT_DATETIME_FORMAT = "Monday January 02 3:04 PM"
const DEFAULT_TIME_FORMAT = "3:04 PM"
const CRONOFY_DATETIME_FORMAT = "2006-01-02T15:04:05Z"

const GOOGLE_IMAGE_URL = "https://collegeinfogeek.com/wp-content/uploads/2016/08/Google_Calendar_Logo.png"
const OUTLOOK_IMAGE_URL = "https://images.techhive.com/images/article/2014/09/outlook-logo-100457446-large.jpg"

const HTML_TEMPLATE = `
<html>
	<body style="
		background-color: #1d2e5c;
		color: #adb5c9;
		display:flex;
		justify-content:center;
		align-items:center;
		font-family:'Open Sans', sans-serif;
		text-align: center;
	">
		<div>
			<div>
				%s
				<img style="border-radius: 50%%; margin-left: 20px;" src="%s" width="100" height="100">
			</div>
			<div>
				<h3 style="margin-left: 30px">%s</h3>
			</div>
		</div>
	</body>
</html>
`

func makeHTMLResponse(h IHandler, mattermostUserID, message, providerName string) string {
	userImage := "https://cdn.freebiesupply.com/logos/large/2x/mattermost-logo-png-transparent.png"
	b, err := h.GetPlugin().API.GetProfileImage(mattermostUserID)
	if err == nil {
		sEnc := b64.StdEncoding.EncodeToString(b)
		userImage = fmt.Sprintf("data:image/png;base64,%s", sEnc)
	}

	providerImage := ""
	if providerName != "" {
		providerImageURL := ""
		switch providerName {
		case "live_connect":
			providerImageURL = OUTLOOK_IMAGE_URL
		case "google":
			providerImageURL = GOOGLE_IMAGE_URL
		}

		providerImage = fmt.Sprintf(`<img style="border-radius: 50%%" src="%s" width="100" height="100">`, providerImageURL)
	}

	return fmt.Sprintf(HTML_TEMPLATE, providerImage, userImage, message)
}

func (p *Plugin) postCommandResponse(args *model.CommandArgs, text string) {
	post := &model.Post{
		UserId:    args.UserId,
		ChannelId: args.ChannelId,
		Message:   text,
	}

	_ = p.API.SendEphemeralPost(args.UserId, post)
}

func (p *Plugin) responsef(commandArgs *model.CommandArgs, format string, args ...interface{}) *model.CommandResponse {
	p.postCommandResponse(commandArgs, fmt.Sprintf(format, args...))
	return &model.CommandResponse{}
}

func (p *Plugin) CreateBotDMtoMMUserId(mattermostUserID, format string, args ...interface{}) (post *model.Post, returnErr error) {
	defer func() {
		if returnErr != nil {
			returnErr = errors.WithMessage(returnErr,
				fmt.Sprintf("failed to create DMError to user %v: ", mattermostUserID))
		}
	}()

	channel, appErr := p.API.GetDirectChannel(mattermostUserID, p.botUserID)
	if appErr != nil {
		return nil, appErr
	}

	post = &model.Post{
		UserId:    p.botUserID,
		ChannelId: channel.Id,
		Message:   fmt.Sprintf(format, args...),
	}

	_, appErr = p.API.CreatePost(post)
	if appErr != nil {
		return nil, appErr
	}

	return post, nil
}

/*

const DEFAULT_USER_ID = "t88abq1sipbbxmhijgw431gc9c"
const DEFAULT_CHANNEL_ID = "c1idbbyc33fe78z9zh36wkt7qa"

func (p *Plugin) ephemeral(text string) {
	uid := DEFAULT_USER_ID
	cid := DEFAULT_CHANNEL_ID
	post := &model.Post{
		UserId:    uid,
		ChannelId: cid,
		Message:   text,
	}
	p.API.SendEphemeralPost(uid, post)
}

*/

func prettyPrintEventList(events []*cronofy.Event) string {
	var currentDay string

	rows := []string{}
	for _, event := range events {
		start, _ := time.Parse(CRONOFY_DATETIME_FORMAT, event.Start)
		dateStr := start.Format(DEFAULT_DATE_FORMAT)
		if currentDay != dateStr {
			currentDay = dateStr
			rows = append(rows, fmt.Sprintf("\n##### %s\n\n", dateStr))
		}

		name := event.Summary

		bullets := []string{}
		if event.ParticipationStatus == "needs_action" {
			bullets = append(bullets, getParticipationLinksString(event))
		}

		startTime, _ := time.Parse(CRONOFY_DATETIME_FORMAT, event.Start)
		startTimeStr := startTime.Format(DEFAULT_TIME_FORMAT)
		endTime, _ := time.Parse(CRONOFY_DATETIME_FORMAT, event.End)
		endTimeStr := endTime.Format(DEFAULT_TIME_FORMAT)

		participationStatus := event.ParticipationStatus
		if participationStatus == "unknown" || participationStatus == "needs_action" {
			participationStatus = ""
		} else {
			bullets = append(bullets, "You have replied: "+strings.Title(participationStatus))
		}

		text := fmt.Sprintf("* ##### %s - %s \"%s\" %s\n", startTimeStr, endTimeStr, name, participationStatus)
		rows = append(rows, text)

		for _, bullet := range bullets {
			text := fmt.Sprintf("    * %s\n", bullet)
			rows = append(rows, text)
		}
	}

	return strings.Join(rows, "")
}

func prettyPrintEventsResponse(calendars []*cronofy.Calendar, events *cronofy.EventsResponse) string {
	type EventsByCalendar struct {
		Calendar *cronofy.Calendar
		Events   []*cronofy.Event
	}

	eventsMappedToCalendars := map[string]*EventsByCalendar{}

	for _, event := range events.Events {
		entry, exists := eventsMappedToCalendars[event.CalendarID]
		if !exists {
			for _, c := range calendars {
				if c.CalendarID == event.CalendarID {
					entry = &EventsByCalendar{
						Calendar: c,
						Events:   []*cronofy.Event{},
					}
					eventsMappedToCalendars[event.CalendarID] = entry
					break
				}
			}
		}

		entry.Events = append(entry.Events, event)
	}

	rows := []string{}
	rows = append(rows, "### Weekly Summary of Calendar Events\n\n")

	temp := []*EventsByCalendar{}
	for _, entry := range eventsMappedToCalendars {
		temp = append(temp, entry)
	}
	sort.Slice(temp, func(i, j int) bool {
		return temp[i].Calendar.ProviderName < temp[j].Calendar.ProviderName
	})

	i := 0
	for _, entry := range temp {
		c := entry.Calendar
		name := c.CalendarName

		var providerName string
		switch c.ProviderName {
		case "google":
			providerName = "Google"
		case "live_connect":
			providerName = "Microsoft Outlook"
		default:
			providerName = c.ProviderName
		}

		text := fmt.Sprintf("### %s \"%s\"\n", providerName, name)
		rows = append(rows, text)

		text = prettyPrintEventList(entry.Events)
		rows = append(rows, text)

		if i != len(eventsMappedToCalendars)-1 {
			rows = append(rows, "\n\n-----\n\n")
		}
		i++
	}

	return strings.Join(rows, "")
}
