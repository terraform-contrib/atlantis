// Copyright 2025 The Atlantis Authors
// SPDX-License-Identifier: Apache-2.0

package webhooks_test

import (
	"testing"
	"text/template"

	. "github.com/petergtz/pegomock/v4"
	"github.com/runatlantis/atlantis/server/events/webhooks"
	"github.com/runatlantis/atlantis/server/events/webhooks/mocks"
	"github.com/runatlantis/atlantis/server/logging"
	. "github.com/runatlantis/atlantis/testing"
)

func TestDriftSlackWebhook_Send(t *testing.T) {
	RegisterMockTestingT(t)
	client := mocks.NewMockSlackClient()
	channel := "drift-alerts"
	hook := webhooks.DriftSlackWebhook{
		Client:  client,
		Channel: channel,
	}

	result := webhooks.DriftResult{
		Repository:        "owner/repo",
		Ref:               "main",
		DetectionID:       "det-456",
		ProjectsWithDrift: 1,
		TotalProjects:     1,
	}

	_ = hook.Send(logging.NewNoopLogger(t), result)
	client.VerifyWasCalledOnce().PostDriftMessage(channel, result, nil)
}

func TestDriftSlackWebhook_SendNoDrift(t *testing.T) {
	RegisterMockTestingT(t)
	client := mocks.NewMockSlackClient()
	channel := "drift-alerts"
	hook := webhooks.DriftSlackWebhook{
		Client:  client,
		Channel: channel,
	}

	result := webhooks.DriftResult{
		Repository:        "owner/repo",
		Ref:               "main",
		DetectionID:       "det-789",
		ProjectsWithDrift: 0,
		TotalProjects:     3,
	}

	err := hook.Send(logging.NewNoopLogger(t), result)
	Ok(t, err)
	client.VerifyWasCalledOnce().PostDriftMessage(channel, result, nil)
}

func TestDriftSlackWebhook_SendWithTemplate(t *testing.T) {
	RegisterMockTestingT(t)
	client := mocks.NewMockSlackClient()
	channel := "drift-alerts"
	tmpl := template.Must(template.New("drift").Parse("Drift in {{ .Repository }}"))
	hook := webhooks.DriftSlackWebhook{
		Client:   client,
		Channel:  channel,
		Template: tmpl,
	}

	err := hook.Send(logging.NewNoopLogger(t), driftResult)
	Ok(t, err)
	client.VerifyWasCalledOnce().PostDriftMessage(channel, driftResult, tmpl)
}
