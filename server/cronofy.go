package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jeffreylo/cronofy"
	"github.com/pkg/errors"
)

type NotificationMeta struct {
	Type         string `json:"type"`
	ChangesSince string `json:"changes_since"`
}

type NotificationChannel struct {
	ChannelId               string            `json:"channel_id"`
	CallbackUrl             string            `json:"callback_url"`
	Filters                 map[string]string `json:"filters"`
	SchedulingConversations map[string]string `json:"scheduling_concersations"`
}

type WebhookMessage struct {
	Notification NotificationMeta    `json:"notification"`
	Channel      NotificationChannel `json:"channel"`
}

func createNotificationChannel(h IHandler, userID string) (string, error) {
	p := h.GetPlugin()

	reqURL := "https://api.cronofy.com/v1/channels"
	secret := p.getConfiguration().InternalSecret

	cronofyUser, err := h.GetPlugin().getCronofyUser(userID)
	if err != nil {
		return "", err
	}
	accessToken := cronofyUser.AccessToken

	payload := map[string]string{
		"callback_url": fmt.Sprintf("%s/plugins/cronofy/webhook?user_id=%s&secret=%s", p.getSiteURL(), userID, secret),
	}

	client := h.MakeCronofyClient(accessToken)
	_, body, err := client.CronofyRequest(userID, reqURL, payload)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func getCalendarInfo(accessToken string, calendarIDs []string) (*cronofy.EventsResponse, error) {
	now := time.Now().UTC()
	from := now.Format("2006-01-02")
	end := now.Add(7 * 24 * time.Hour)
	to := end.Format("2006-01-02")
	client := NewCronofyClient(accessToken)

	res, err := client.GetEvents(&cronofy.EventsRequest{
		TZID:        "UTC",
		From:        &from,
		To:          &to,
		CalendarIDs: calendarIDs,
	})

	if err != nil {
		return nil, err
	}

	return res, nil

	// timezone := "Local"
}

func httpWebhook(h IHandler, w http.ResponseWriter, r *http.Request) (int, error) {
	p := h.GetPlugin()

	mattermostUserID := r.URL.Query().Get("user_id")
	secret := r.URL.Query().Get("secret")
	storedSecret := p.getConfiguration().InternalSecret
	if mattermostUserID == "" || secret != storedSecret {
		return http.StatusUnauthorized, errors.New("not authorized")
	}

	decoder := json.NewDecoder(r.Body)
	var body WebhookMessage
	err := decoder.Decode(&body)
	if err != nil {
		p.CreateBotDMtoMMUserId(mattermostUserID, err.Error())
		return http.StatusInternalServerError, err
	}

	switch body.Notification.Type {
	case "change":
		return handleWebhookEventChange(h, mattermostUserID, body)
	case "verification":
		return handleWebhookVerification(h, mattermostUserID, body)
	}

	return http.StatusNotFound, fmt.Errorf("Unsupported webhook message type: %s", body.Notification.Type)
}

func handleWebhookEventChange(h IHandler, mattermostUserID string, body WebhookMessage) (int, error) {
	p := h.GetPlugin()

	cronofyUser, err := p.getCronofyUser(mattermostUserID)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	accessToken := cronofyUser.AccessToken

	lastMod, err := time.Parse(CRONOFY_DATETIME_FORMAT, body.Notification.ChangesSince)
	if err != nil {
		p.CreateBotDMtoMMUserId(mattermostUserID, err.Error())
		return http.StatusInternalServerError, err
	}

	client := NewCronofyClient(accessToken)

	res, err := client.GetEvents(&cronofy.EventsRequest{
		TZID:         "UTC",
		LastModified: &lastMod,
	})
	if err != nil {
		p.CreateBotDMtoMMUserId(mattermostUserID, err.Error())
		return http.StatusInternalServerError, err
	}

	if len(res.Events) == 0 {
		p.CreateBotDMtoMMUserId(mattermostUserID, "A calendar event was deleted")
		return http.StatusOK, nil
	}

	p.storeEvents(res.Events)

	evt := res.Events[0]

	if lastMod.Sub(evt.Created).Seconds() > 10 && false {
		text := fmt.Sprintf(`Event "%s" has been updated.`, evt.Summary)
		p.CreateBotDMtoMMUserId(mattermostUserID, text)
		return http.StatusOK, nil
	}

	if evt.ParticipationStatus != "needs_action" {
		text := fmt.Sprintf(`Event "%s" has changed. You have already replied: %s`, evt.Summary, evt.ParticipationStatus)
		p.CreateBotDMtoMMUserId(mattermostUserID, text)
		return http.StatusOK, nil
	}

	text := getParticipationLinksString(evt)

	p.CreateBotDMtoMMUserId(mattermostUserID, text)

	return http.StatusOK, nil
}

func getParticipationLinksString(evt *cronofy.Event) string {
	params := url.Values{}
	params.Add("calendar_id", evt.CalendarID)
	params.Add("event_uid", evt.EventUID)
	s := params.Encode()

	acceptUrl := "/plugins/cronofy/participation?" + s + "&participation=accepted"
	declineUrl := "/plugins/cronofy/participation?" + s + "&participation=declined"
	tentativeUrl := "/plugins/cronofy/participation?" + s + "&participation=tentative"
	text := fmt.Sprintf(`%s has invited you for "%s" [Accept](%s) [Decline](%s) [Tentative](%s)`, evt.Organizer.Email, evt.Summary, acceptUrl, declineUrl, tentativeUrl)
	return text
}

func handleWebhookVerification(h IHandler, mattermostUserID string, body WebhookMessage) (int, error) {
	h.GetPlugin().CreateBotDMtoMMUserId(mattermostUserID, "You will now receive notifications from your calendar!")
	return http.StatusOK, nil
}

func httpSetParticipation(h IHandler, w http.ResponseWriter, r *http.Request) (int, error) {
	mattermostUserID := r.Header.Get("Mattermost-User-Id")
	if mattermostUserID == "" {
		return http.StatusUnauthorized, errors.New("not authorized")
	}

	p := h.GetPlugin()

	q := r.URL.Query()
	participation := q.Get("participation")
	cid := q.Get("calendar_id")
	eid := q.Get("event_uid")

	body := map[string]string{
		"status": participation,
	}

	cronofyUser, err := h.GetPlugin().getCronofyUser(mattermostUserID)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	accessToken := cronofyUser.AccessToken

	client := NewCronofyClient(accessToken)

	reqURL := fmt.Sprintf("https://api.cronofy.com/v1/calendars/%s/events/%s/participation_status", cid, eid)
	status, _, err := client.CronofyRequest(mattermostUserID, reqURL, body)
	if err != nil {
		p.CreateBotDMtoMMUserId(mattermostUserID, err.Error())
	}

	var text string

	if status != http.StatusAccepted {
		text = "Failed to change event's status."
		p.CreateBotDMtoMMUserId(mattermostUserID, text)
		w.Write([]byte(makeHTMLResponse(h, mattermostUserID, text, "")))
		return status, errors.New(text)
	}

	text = fmt.Sprintf(`Successfully set status to %s`, participation)
	providerName := ""
	evt, err := p.getEvent(eid)
	if err == nil && evt != nil {
		startTime, _ := time.Parse(CRONOFY_DATETIME_FORMAT, evt.Start)
		startTimeStr := startTime.Format(DEFAULT_TIME_FORMAT)
		startDateStr := startTime.Format(DEFAULT_DATE_FORMAT)

		text = fmt.Sprintf(`Successfully set status to %s, for "%s" with %s on %s at %s`, participation, evt.Summary, evt.Organizer.Email, startDateStr, startTimeStr)
	}

	w.Write([]byte(makeHTMLResponse(h, mattermostUserID, text, providerName)))
	p.CreateBotDMtoMMUserId(mattermostUserID, text)

	return http.StatusOK, nil
}

type AvailabilityParticipantMember struct {
	Sub         string   `json:"sub"`
	CalendarIDs []string `json:"calendar_ids"`
}

type AvailabilityParticipant struct {
	Members  []AvailabilityParticipantMember `json:"members"`
	Required string                          `json:"required"`
}

type AvailabilityDuration struct {
	Minutes int `json:"minutes"`
}

type AvailabilityPeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type AvailabilityBuffer struct {
	Before AvailabilityDuration `json:"before"`
	After  AvailabilityDuration `json:"after"`
}

type AvailabilityRequest struct {
	Participants     []AvailabilityParticipant `json:"participants"`
	RequiredDuration AvailabilityDuration      `json:"required_duration"`
	AvailablePeriods []AvailabilityPeriod      `json:"available_periods"`
	Buffer           AvailabilityBuffer        `json:"buffer"`
}

type AvailabilityResponse struct {
	AvailablePeriods []AvailabilityPeriod            `json:"available_periods"`
	Participants     []AvailabilityParticipantMember `json:"participants"`
}

func buildAvailabilityRequest(sub string, calendarIDs []string) (AvailabilityRequest, error) {
	member := AvailabilityParticipantMember{
		Sub:         sub,
		CalendarIDs: calendarIDs,
	}
	availablePeriods := []AvailabilityPeriod{AvailabilityPeriod{
		Start: time.Now().Add(time.Minute * 2).UTC().Format(CRONOFY_DATETIME_FORMAT),
		End:   time.Now().Add(time.Minute * 15).UTC().Format(CRONOFY_DATETIME_FORMAT),
	}}

	return AvailabilityRequest{
		Participants: []AvailabilityParticipant{AvailabilityParticipant{
			Members:  []AvailabilityParticipantMember{member},
			Required: "all",
		}},
		RequiredDuration: AvailabilityDuration{Minutes: 2},
		AvailablePeriods: availablePeriods,
		Buffer: AvailabilityBuffer{
			Before: AvailabilityDuration{Minutes: 6},
			After:  AvailabilityDuration{Minutes: 5},
		},
	}, nil
}

func getUserAvailabilityStatus(h IHandler, userID string) (*AvailabilityResponse, error) {
	p := h.GetPlugin()

	info, err := p.getCronofyUser(userID)
	if err != nil {
		return nil, err
	}

	accessToken := info.AccessToken
	client := NewCronofyClient(accessToken)

	calendars, err := client.GetCalendars()
	if err != nil {
		return nil, err
	}

	if len(calendars) == 0 {
		return nil, errors.New("No calendars found")
	}

	sub := info.Sub
	calendarIDs := []string{}
	for _, c := range calendars {
		calendarIDs = append(calendarIDs, c.CalendarID)
	}

	req, err := buildAvailabilityRequest(sub, calendarIDs)
	if err != nil {
		return nil, err
	}

	reqURL := "https://api.cronofy.com/v1/availability"
	status, data, err := client.CronofyRequest(userID, reqURL, req)

	if err != nil {
		return nil, err
	} else if status >= 300 {
		return nil, fmt.Errorf("Cronofy returned status %d %s", status, string(data))
	}

	av := &AvailabilityResponse{}
	err = json.Unmarshal(data, av)
	if err != nil {
		return nil, err
	}

	return av, nil
}

func updateUserStatusWithAvailabilities(h IHandler, userID string, availabilities *AvailabilityResponse) (string, error) {
	p := h.GetPlugin()

	prevStatus, appErr := p.API.GetUserStatus(userID)
	if appErr != nil {
		return "", appErr
	}

	if len(availabilities.AvailablePeriods) == 0 {
		if prevStatus.Status == "dnd" {
			return "User is not available. User is already DND.", nil
		}

		nextStatus, appErr := p.API.UpdateUserStatus(userID, "dnd")
		if appErr != nil {
			return "", appErr
		}

		return fmt.Sprintf(`User is not available. Old status "%s", New status "%s"`, prevStatus.Status, nextStatus.Status), nil
	}

	if prevStatus.Status != "dnd" {
		return fmt.Sprintf(`User is available. Status is still %s`, prevStatus.Status), nil
	}

	nextStatus, appErr := p.API.UpdateUserStatus(userID, "online")
	if appErr != nil {
		return "", appErr
	}

	return fmt.Sprintf(`User is available. Old status "%s", New status "%s"`, prevStatus.Status, nextStatus.Status), nil
}

func getAvailabiltiesAndUpdateStatus(h IHandler, userID string) (string, error) {
	availabilities, err := getUserAvailabilityStatus(h, userID)
	if err != nil {
		return "", errors.Wrap(err, "Failed to fetch user availabilities")
	}

	res, err := updateUserStatusWithAvailabilities(h, userID, availabilities)
	if err != nil {
		return "", errors.Wrap(err, "Failed to update user status based on availabilities")
	}

	return res, nil
}
