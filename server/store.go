package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"

	"github.com/jeffreylo/cronofy"
	"github.com/mattermost/mattermost-server/model"
	"github.com/pkg/errors"
)

const KVUserPrefix = "user_"
const KVOAuthUserStatePrefix = "oauth_state_"
const KVCalendarEvents = "calendar_events"

func (p *Plugin) getCronofyUser(userID string) (*AccessTokenResponse, error) {
	key := KVUserPrefix + userID

	data, appErr := p.API.KVGet(key)
	if appErr != nil {
		return nil, errors.Wrap(appErr, "Failed to get user from kv store")
	}

	u := &AccessTokenResponse{}
	err := json.Unmarshal(data, u)
	return u, err
}

func (p *Plugin) storeCronofyUser(userID string, cronofyUser *AccessTokenResponse) error {
	key := KVUserPrefix + userID
	data, appErr := json.Marshal(cronofyUser)
	if appErr != nil {
		return errors.Wrap(appErr, "Failed to store user in kv store")
	}

	return p.API.KVSet(key, data)
}

func (p *Plugin) storeOAuthUserState(userID string, state []byte) error {
	key := KVOAuthUserStatePrefix + userID

	return p.API.KVSet(key, state)
}

func (p *Plugin) getOAuthUserState(userID string) ([]byte, error) {
	key := KVOAuthUserStatePrefix + userID

	data, appErr := p.API.KVGet(key)
	if appErr != nil {
		return nil, errors.Wrap(appErr, "Failed to get user from kv store")
	}

	return data, nil
}

type CalendarEventStore map[string]cronofy.Event

func (p *Plugin) getEvent(event_uid string) (*cronofy.Event, error) {
	allEvents, err := p.getEvents()
	if err != nil {
		return nil, err
	}

	evt, exists := allEvents[event_uid]
	if exists {
		return &evt, nil
	}

	return nil, errors.New("Failed to get event from kv store")
}

func (p *Plugin) getEvents() (map[string]cronofy.Event, error) {
	key := KVCalendarEvents

	var data []byte
	var appErr *model.AppError

	data, appErr = p.API.KVGet(key)
	if appErr != nil {
		return nil, errors.Wrap(appErr, "KVGET failed")
	}

	allEvents := map[string]cronofy.Event{}
	if data == nil {
		return allEvents, nil
	}

	err := json.Unmarshal(data, &allEvents)
	return allEvents, err
}

func (p *Plugin) storeEvents(events []*cronofy.Event) error {
	key := KVCalendarEvents

	allEvents, err := p.getEvents()
	if err != nil {
		return errors.WithMessage(err, "1")
	}

	for _, evt := range events {
		allEvents[evt.EventUID] = *evt
	}

	value, err := json.Marshal(allEvents)
	if err != nil {
		return errors.WithMessage(err, "2")
	}

	appErr := p.API.KVSet(key, value)
	if appErr != nil {
		return errors.Wrap(appErr, "KVSET failed")
	}

	return nil
}

func hashkey(prefix, key string) string {
	h := md5.New()
	_, _ = h.Write([]byte(key))
	return fmt.Sprintf("%s%x", prefix, h.Sum(nil))
}
