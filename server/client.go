package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/jeffreylo/cronofy"
	"github.com/pkg/errors"
)

type ICronofyClient interface {
	CronofyRequest(userID string, reqURL string, payload interface{}) (int, []byte, error)
	GetCalendars() ([]*cronofy.Calendar, error)
	GetEvents(options *cronofy.EventsRequest) (*cronofy.EventsResponse, error)
}

type CronofyClient struct {
	AccessToken string
	client      *cronofy.Client
}

func NewCronofyClient(accessToken string) *CronofyClient {
	c := cronofy.NewClient(&cronofy.Config{
		AccessToken: accessToken,
	})

	return &CronofyClient{
		AccessToken: accessToken,
		client:      c,
	}
}

func (c *CronofyClient) GetCalendars() ([]*cronofy.Calendar, error) {
	return c.client.GetCalendars()
}

func (c *CronofyClient) GetEvents(options *cronofy.EventsRequest) (*cronofy.EventsResponse, error) {
	return c.client.GetEvents(options)
}

func (c *CronofyClient) CronofyRequest(userID string, reqURL string, payload interface{}) (int, []byte, error) {
	accessToken := c.AccessToken

	timeout := time.Duration(5 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}

	jsonPayload, err := json.Marshal(&payload)
	if err != nil {
		return 0, nil, errors.Wrap(err, "CronofyRequest 1")
	}

	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return 0, nil, errors.Wrap(err, "CronofyRequest 2")
	}

	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	req.Header.Add("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, errors.Wrap(err, "CronofyRequest 3")
	}

	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, errors.Wrap(err, "CronofyRequest 4")
	}

	return resp.StatusCode, data, nil
}
