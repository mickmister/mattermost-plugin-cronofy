// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License for license information.

package main

import (
	"net/http"
	"strconv"

	"github.com/pkg/errors"

	"github.com/mattermost/mattermost-server/plugin"
)

const (
	routeOAuthComplete    = "/oauth/complete"
	routeSetParticipation = "/participation"
	routeWebhook          = "/webhook"
)

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	status, err := handleHTTPRequest(p, w, r)
	if err != nil {
		p.API.LogError("ERROR: ", "Status", strconv.Itoa(status), "Error", err.Error(), "Host", r.Host, "RequestURI", r.RequestURI, "Method", r.Method, "query", r.URL.Query().Encode())
		http.Error(w, err.Error(), status)
		return
	}
	switch status {
	case http.StatusOK:
		// pass through
	case 0:
		status = http.StatusOK
	default:
		w.WriteHeader(status)
	}
	p.API.LogDebug("OK: ", "Status", strconv.Itoa(status), "Host", r.Host, "RequestURI", r.RequestURI, "Method", r.Method, "query", r.URL.Query().Encode())
}

func handleHTTPRequest(p *Plugin, w http.ResponseWriter, r *http.Request) (int, error) {
	h := &Handler{p}

	switch r.URL.Path {
	case routeOAuthComplete, routeOAuthComplete + ".html":
		return httpOAuthComplete(h, w, r)
	case routeWebhook:
		return httpWebhook(h, w, r)
	case routeSetParticipation:
		return httpSetParticipation(h, w, r)
	}

	return http.StatusNotFound, errors.New("not found")
}
