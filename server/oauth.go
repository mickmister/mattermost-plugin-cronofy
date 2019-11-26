package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type AccessTokenRequest struct {
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	GrantType    string `json:"grant_type"`
	Code         string `json:"code"`
	RedirectUri  string `json:"redirect_uri"`
}

type AccessTokenResponse struct {
	// "access_token": "P531x88i05Ld2yXHIQ7WjiEyqlmOHsgI",
	AccessToken string `json:"access_token"`

	// "refresh_token": "3gBYG1XamYDUEXUyybbummQWEe5YqPmf",
	RefreshToken string `json:"refresh_token"`

	// "scope": "create_event delete_event",
	Scope string `json:"scope"`

	// "account_id": "acc_567236000909002",
	AccountId string `json:"account_id"`

	// "sub": "acc_567236000909002",
	Sub string `json:"sub"`

	// "token_type": "bearer",
	TokenType string `json:"token_type"`

	// "expires_in": 3600,
	ExpiresIn int `json:"expires_in"`

	LinkingProfile struct {
		ProviderName string `json:"provider_name"`
		ProfileId    string `json:"profile_id"`
		ProfileName  string `json:"profile_name"`
	} `json:"linking_profile"`

	// "linking_profile": {
	//   "provider_name": "google",
	//   "profile_id": "pro_n23kjnwrw2",
	//   "profile_name": "example@cronofy.com"
	// }
}

func httpOAuthComplete(h IHandler, w http.ResponseWriter, r *http.Request) (status int, err error) {
	p := h.GetPlugin()

	code := r.URL.Query().Get("code")
	res, err := getAccessToken(p, code)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	parts := strings.Split(r.URL.Query().Get("state"), "||")
	mattermostUserID := parts[0]
	providedState := parts[1]

	storedState, _ := p.getOAuthUserState(mattermostUserID)
	if string(storedState) != providedState {
		w.Write([]byte(fmt.Sprintf(`state does not match "%s" "%s"`, providedState, storedState)))
		return http.StatusBadRequest, errors.New("Cronofy supplied incorrect state for OAuth Connect")
	}

	p.storeCronofyUser(mattermostUserID, res)

	provider := res.LinkingProfile.ProviderName
	email := res.LinkingProfile.ProfileName

	providerPretty := provider
	switch provider {
	case "google":
		providerPretty = "Google"
	case "live_connect":
		providerPretty = "Outlook"
	}

	go func() {
		s, err := createNotificationChannel(h, mattermostUserID)
		if err == nil {
			data := []byte(s)
			p.API.KVSet("cronofy_notification_channel", data)
		}
	}()

	var text string
	text = fmt.Sprintf(`You've successfully connected your %s account named "%s" to your Mattermost account.`, providerPretty, email)
	p.CreateBotDMtoMMUserId(mattermostUserID, text)
	w.Write([]byte(makeHTMLResponse(h, mattermostUserID, text, provider)))

	return http.StatusOK, nil
}

func newAccessTokenRequest(p *Plugin, code string) AccessTokenRequest {
	return AccessTokenRequest{
		ClientId:     p.getConfiguration().ClientID,
		ClientSecret: p.getConfiguration().ClientSecret,
		GrantType:    "authorization_code",
		Code:         code,
		RedirectUri:  p.getSiteURL() + "/plugins/cronofy/oauth/complete",
	}
}

func getAccessToken(p *Plugin, code string) (*AccessTokenResponse, error) {
	payload := newAccessTokenRequest(p, code)
	jsonPayload, err := json.Marshal(&payload)
	if err != nil {
		return nil, errors.Wrap(err, "new request failed 1")
	}

	timeout := time.Duration(5 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}

	reqURL := "https://api.cronofy.com/oauth/token"
	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, errors.Wrap(err, "new request failed 2")
	}

	req.Header.Add("Content-Type", "application/json; charset=utf-8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "new request failed 3")
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "new request failed 4")
	}

	var result AccessTokenResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, errors.Wrap(err, "new request failed 5")
	}

	return &result, nil
}
