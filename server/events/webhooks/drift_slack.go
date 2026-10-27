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

// driftTemplateSamples are rendered when a drift message template is parsed,
// so mistakes such as misspelled fields fail at startup instead of when drift
// is detected. Go templates only resolve fields on the branches they execute,
// so the samples cover drift, no drift, no projects, and unnamed projects with
// changes made outside of Terraform.
var driftTemplateSamples = []DriftResult{
	{
		AtlantisURL:       "https://atlantis.example.com",
		Repository:        "owner/repo",
		Ref:               "main",
		DetectionID:       "00000000-0000-0000-0000-000000000000",
		ProjectsWithDrift: 2,
		TotalProjects:     3,
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
				Path:           "changed-outside",
				Workspace:      "default",
				HasDrift:       true,
				ChangesOutside: true,
				Summary:        "\n**Note: Objects have changed outside of Terraform**\nNo changes. Your infrastructure matches the configuration.",
			},
			{
				ProjectName: "clean",
				Path:        "clean",
				Workspace:   "default",
				Summary:     "No changes. Your infrastructure matches the configuration.",
			},
		},
	},
	{
		AtlantisURL:   "https://atlantis.example.com",
		Repository:    "owner/repo",
		Ref:           "main",
		DetectionID:   "00000000-0000-0000-0000-000000000000",
		TotalProjects: 1,
		Projects: []DriftProjectResult{
			{
				ProjectName: "clean",
				Path:        "clean",
				Workspace:   "default",
				Summary:     "No changes. Your infrastructure matches the configuration.",
			},
		},
	},
	{
		AtlantisURL: "https://atlantis.example.com",
		Repository:  "owner/repo",
		Ref:         "main",
		DetectionID: "00000000-0000-0000-0000-000000000000",
	},
}

// driftTemplateFuncs returns the Sprig functions without the ones that read
// the process environment or do network lookups, so a template can't post
// server secrets such as VCS tokens to Slack.
func driftTemplateFuncs() template.FuncMap {
	funcs := sprig.TxtFuncMap()
	for _, name := range []string{"env", "expandenv", "getHostByName"} {
		delete(funcs, name)
	}
	return funcs
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
// use Go text/template syntax with the sprig functions, except those that
// read the environment or the network, and are rendered with a DriftResult.
func parseDriftTemplate(text string) (*template.Template, error) {
	tmpl, err := template.New("drift").Funcs(driftTemplateFuncs()).Parse(text)
	if err != nil {
		return nil, err
	}
	for _, sample := range driftTemplateSamples {
		if err := tmpl.Execute(io.Discard, slackDriftResult(sample)); err != nil {
			return nil, fmt.Errorf("rendering a sample drift result: %w", err)
		}
	}
	return tmpl, nil
}
