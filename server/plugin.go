package main

import (
	"sync"

	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/plugin"
	"github.com/pkg/errors"
)

const (
	botUserName    = "calendar_bot"
	botDisplayName = "Calendar Bot"
	botDescription = "I help you manage your calendars"
)

func (p *Plugin) OnActivate() error {
	var err error

	botUserID, err := p.Helpers.EnsureBot(&model.Bot{
		Username:    botUserName,
		DisplayName: botDisplayName,
		Description: botDescription,
	})
	if err != nil {
		return errors.Wrap(err, "failed to ensure bot account")
	}

	p.botUserID = botUserID

	err = p.API.RegisterCommand(getCommand())
	if err != nil {
		return errors.WithMessage(err, "OnActivate: failed to register command")
	}

	p.InitRecurringJob(p.getConfiguration().EnableAvailabilityJob)

	return nil
}

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	botUserID string

	recurringJob *RecurringJob
}

func (p *Plugin) getSiteURL() string {
	ptr := p.API.GetConfig().ServiceSettings.SiteURL
	if ptr == nil {
		return ""
	}
	return *ptr
}

type IHandler interface {
	GetPlugin() *Plugin
	MakeCronofyClient(accessToken string) ICronofyClient
}

type Handler struct {
	plugin *Plugin
}

func (h *Handler) GetPlugin() *Plugin {
	return h.plugin
}

func (h *Handler) MakeCronofyClient(accessToken string) ICronofyClient {
	return NewCronofyClient(accessToken)
}
