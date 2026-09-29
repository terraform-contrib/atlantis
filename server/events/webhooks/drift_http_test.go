// Copyright 2025 The Atlantis Authors
// SPDX-License-Identifier: Apache-2.0

package webhooks_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/runatlantis/atlantis/server/events/webhooks"
	"github.com/runatlantis/atlantis/server/logging"
	. "github.com/runatlantis/atlantis/testing"
)

func TestDriftHttpWebhook_Send(t *testing.T) {
	var receivedBody webhooks.DriftResult
	var rawBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Equals(t, "POST", r.Method)
		Equals(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		Ok(t, err)
		err = json.Unmarshal(body, &receivedBody)
		Ok(t, err)
		err = json.Unmarshal(body, &rawBody)
		Ok(t, err)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hook := webhooks.DriftHttpWebhook{
		Client: &webhooks.HttpClient{Client: http.DefaultClient},
		URL:    server.URL,
	}

	err := hook.Send(logging.NewNoopLogger(t), driftResult)
	Ok(t, err)

	Equals(t, driftResult.Repository, receivedBody.Repository)
	Equals(t, driftResult.DetectionID, receivedBody.DetectionID)
	Equals(t, driftResult.ProjectsWithDrift, receivedBody.ProjectsWithDrift)
	Equals(t, driftResult.TotalProjects, receivedBody.TotalProjects)
	Equals(t, len(driftResult.Projects), len(receivedBody.Projects))

	// The instance identity and per-project fields are part of the payload
	// contract, so check them by their JSON keys.
	Equals(t, "https://atlantis.example.com", rawBody["atlantis_url"])
	projects, ok := rawBody["projects"].([]any)
	Assert(t, ok && len(projects) == 2, "expected two projects, got %v", rawBody["projects"])
	project, ok := projects[0].(map[string]any)
	Assert(t, ok, "expected a project object, got %T", projects[0])
	Equals(t, "project1", project["project_name"])
	Equals(t, "infra/project1", project["path"])
	Equals(t, "Plan: 1 to add, 2 to change, 0 to destroy.", project["summary"])
	Equals(t, false, project["changes_outside"])
}

func TestDriftHttpWebhook_SendWithHeaders(t *testing.T) {
	expectedHeaders := map[string][]string{
		"Authorization": {"Bearer drift-token"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Equals(t, "Bearer drift-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hook := webhooks.DriftHttpWebhook{
		Client: &webhooks.HttpClient{Client: http.DefaultClient, Headers: expectedHeaders},
		URL:    server.URL,
	}

	err := hook.Send(logging.NewNoopLogger(t), driftResult)
	Ok(t, err)
}

func TestDriftHttpWebhook_Send202(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	hook := webhooks.DriftHttpWebhook{
		Client: &webhooks.HttpClient{Client: http.DefaultClient},
		URL:    server.URL,
	}

	err := hook.Send(logging.NewNoopLogger(t), driftResult)
	Ok(t, err)
}

func TestDriftHttpWebhook_Send500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	hook := webhooks.DriftHttpWebhook{
		Client: &webhooks.HttpClient{Client: http.DefaultClient},
		URL:    server.URL,
	}

	err := hook.Send(logging.NewNoopLogger(t), driftResult)
	Assert(t, err != nil, "expected error on 500")
	ErrContains(t, "status 500", err)
}

func TestDriftHttpWebhook_Send500RedactsURLCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed https://user:secret@example.com/hooks/sensitive?token=super-secret"))
	}))
	defer server.Close()

	urlWithCredentials := strings.Replace(server.URL, "http://", "http://user:secret@", 1) + "/hooks/sensitive?token=super-secret"
	hook := webhooks.DriftHttpWebhook{
		Client: &webhooks.HttpClient{Client: http.DefaultClient},
		URL:    urlWithCredentials,
	}

	err := hook.Send(logging.NewNoopLogger(t), driftResult)
	Assert(t, err != nil, "expected error on 500")
	errText := err.Error()
	Assert(t, !strings.Contains(errText, "user:secret"), "error leaked basic auth credentials: %s", errText)
	Assert(t, !strings.Contains(errText, "super-secret"), "error leaked query token: %s", errText)
	Assert(t, !strings.Contains(errText, "/hooks/sensitive"), "error leaked path: %s", errText)
	ErrContains(t, "status 500", err)
}

type failingRoundTripper struct{}

func (f failingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("request failed for " + req.URL.String())
}

func TestDriftHttpWebhook_SendRequestErrorRedactsURLCredentials(t *testing.T) {
	hook := webhooks.DriftHttpWebhook{
		Client: &webhooks.HttpClient{Client: &http.Client{Transport: failingRoundTripper{}}},
		URL:    "https://user:secret@example.com/hooks/sensitive?access_token=super-secret",
	}

	err := hook.Send(logging.NewNoopLogger(t), driftResult)
	Assert(t, err != nil, "expected request error")
	errText := err.Error()
	Assert(t, !strings.Contains(errText, "user:secret"), "error leaked basic auth credentials: %s", errText)
	Assert(t, !strings.Contains(errText, "super-secret"), "error leaked query token: %s", errText)
}
