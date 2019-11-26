package main

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/plugin"
)

type CommandHandlerFunc func(h IHandler, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse

type CommandHandler struct {
	handlers       map[string]CommandHandlerFunc
	defaultHandler CommandHandlerFunc
}

var commandHandler = CommandHandler{
	handlers: map[string]CommandHandlerFunc{
		"view":         executeView,
		"subscribe":    executeSubscribe,
		"connect":      executeConnect,
		"availability": executeAvailability,
	},
	defaultHandler: executeDefaultCommand,
}

func (ch CommandHandler) Handle(h IHandler, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse {
	for n := len(args); n > 0; n-- {
		hFunc := ch.handlers[strings.Join(args[:n], "/")]
		if hFunc != nil {
			return hFunc(h, c, header, args[n:]...)
		}
	}
	return ch.defaultHandler(h, c, header, args...)
}

func (p *Plugin) ExecuteCommand(c *plugin.Context, commandArgs *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	args := strings.Fields(commandArgs.Command)
	if len(args) == 0 || args[0] != "/cronofy" {
		return p.responsef(commandArgs, "Invalid command"), nil
	}

	h := &Handler{plugin: p}

	return commandHandler.Handle(h, c, commandArgs, args[1:]...), nil
}

func getCommand() *model.Command {
	return &model.Command{
		Trigger:          "cronofy",
		DisplayName:      "Cronofy",
		Description:      "Integration with Cronofy.",
		AutoComplete:     true,
		AutoCompleteDesc: "Available commands: connect, view, subscribe, availability",
		AutoCompleteHint: "[command]",
	}
}

func executeView(h IHandler, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse {
	p := h.GetPlugin()

	info, err := p.getCronofyUser(header.UserId)
	if err != nil {
		return p.responsef(header, err.Error())
	}

	accessToken := info.AccessToken
	client := NewCronofyClient(accessToken)

	calendars, err := client.GetCalendars()
	if err != nil {
		p.responsef(header, fmt.Sprintf("Error: %s", err.Error()))
	}

	if len(calendars) == 0 {
		return p.responsef(header, "No calendars matched the query")
	}

	calenderIDs := []string{}
	for _, c := range calendars {
		calenderIDs = append(calenderIDs, c.CalendarID)
	}

	events, err := getCalendarInfo(info.AccessToken, calenderIDs)
	if err != nil {
		return p.responsef(header, err.Error())
	}

	err = p.storeEvents(events.Events)
	if err != nil {
		return p.responsef(header, err.Error())
	}

	return p.responsef(header, prettyPrintEventsResponse(calendars, events))
}

var executeDefaultCommand = executeView

func executeSubscribe(h IHandler, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse {
	p := h.GetPlugin()

	s, err := createNotificationChannel(h, header.UserId)
	if err != nil {
		p.responsef(header, fmt.Sprintf("Error: %s", err.Error()))
	}

	data := []byte(s)
	p.API.KVSet("cronofy_notification_channel", data)

	return p.responsef(header, "Successfully created a subscription to update your Mattermost status based on your calendar availability.")
}

func executeConnect(h IHandler, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse {
	p := h.GetPlugin()

	scope := "read_events change_participation_status"
	clientID := p.getConfiguration().ClientID
	redirectURL := p.getSiteURL() + "/plugins/cronofy/oauth/complete"

	hash, err := getRandomHash()
	if err != nil {
		return p.responsef(header, fmt.Sprintf("Failed to create hash: %s", err.Error()))
	}
	p.storeOAuthUserState(header.UserId, hash)

	state := header.UserId + "||" + string(hash)

	callback := fmt.Sprintf(`https://app.cronofy.com/oauth/authorize?response_type=code&client_id=%s&redirect_uri=%s&scope=%s&state=%s`, clientID, redirectURL, scope, state)
	return p.responsef(header, fmt.Sprintf("#### [Click me to connect!](%s)", callback))
}

func executeAvailability(h IHandler, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse {
	p := h.GetPlugin()

	res, err := getAvailabiltiesAndUpdateStatus(h, header.UserId)
	if err != nil {
		return p.responsef(header, fmt.Sprintf(err.Error()))
	}

	return p.responsef(header, fmt.Sprintf(res))
}
