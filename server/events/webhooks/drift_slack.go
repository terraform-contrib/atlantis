// Copyright 2025 The Atlantis Authors
// SPDX-License-Identifier: Apache-2.0

package webhooks

import (
	"fmt"
	"io"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/runatlantis/atlantis/server/logging"
)

// driftTemplateSample is rendered when a drift message template is parsed, so
// mistakes such as misspelled fields fail at startup instead of when drift is
// detected.
var driftTemplateSample = DriftResult{
	AtlantisURL:       "https://atlantis.example.com",
	Repository:        "owner/repo",
	Ref:               "main",
	DetectionID:       "00000000-0000-0000-0000-000000000000",
	ProjectsWithDrift: 1,
	TotalProjects:     2,
	Projects: []DriftProjectResult{
		{
			ProjectName: "drifted",
			Path:        "drifted",
			Workspace:   "default",
			HasDrift:    true,
			ToChange:    1,
			Summary:     "Plan: 0 to add, 1 to change, 0 to destroy.",
		},
		{
			ProjectName: "clean",
			Path:        "clean",
			Workspace:   "default",
			Summary:     "No changes. Your infrastructure matches the configuration.",
		},
	},
}

// DriftSlackWebhook sends drift notifications to Slack.
type DriftSlackWebhook struct {
	Client  SlackClient
	Channel string
	// Template replaces the default message when set.
	Template *template.Template
}

// Send sends the drift result to Slack.
func (s *DriftSlackWebhook) Send(_ logging.SimpleLogging, result DriftResult) error {
	return s.Client.PostDriftMessage(s.Channel, result, s.Template)
}

// parseDriftTemplate parses a custom Slack drift message template. Templates
// use Go text/template syntax with the sprig functions, and are rendered with
// a DriftResult.
func parseDriftTemplate(text string) (*template.Template, error) {
	tmpl, err := template.New("drift").Funcs(sprig.TxtFuncMap()).Parse(text)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(io.Discard, slackDriftResult(driftTemplateSample)); err != nil {
		return nil, fmt.Errorf("rendering a sample drift result: %w", err)
	}
	return tmpl, nil
}
