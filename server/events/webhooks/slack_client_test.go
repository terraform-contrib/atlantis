// Copyright 2017 HootSuite Media Inc.
// SPDX-License-Identifier: Apache-2.0
// Modified hereafter by contributors to runatlantis/atlantis.

package webhooks_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"text/template"

	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/webhooks"
	"github.com/runatlantis/atlantis/server/events/webhooks/mocks"
	"github.com/slack-go/slack"

	. "github.com/petergtz/pegomock/v4"
	. "github.com/runatlantis/atlantis/testing"
)

var underlying *mocks.MockUnderlyingSlackClient
var client webhooks.DefaultSlackClient
var result webhooks.ApplyResult

func TestAuthTest_Success(t *testing.T) {
	t.Log("When the underlying client succeeds, function should succeed")
	setup(t)
	err := client.AuthTest()
	Ok(t, err)
}

func TestAuthTest_Error(t *testing.T) {
	t.Log("When the underlying slack client errors, an error should be returned")
	setup(t)
	When(underlying.AuthTest()).ThenReturn(nil, errors.New(""))
	err := client.AuthTest()
	Assert(t, err != nil, "expected error")
}

func TestTokenIsSet(t *testing.T) {
	t.Log("When the Token is an empty string, function should return false")
	c := webhooks.DefaultSlackClient{
		Token: "",
	}
	Equals(t, false, c.TokenIsSet())

	t.Log("When the Token is not an empty string, function should return true")
	c.Token = "random"
	Equals(t, true, c.TokenIsSet())
}

/*
// The next 2 tests are commented out because they currently fail using the Pegamock's
// VerifyWasCalledOnce using variadic parameters.
// See issue https://github.com/petergtz/pegomock/issues/112
func TestPostMessage_Success(t *testing.T) {
	t.Log("When apply succeeds, function should succeed and indicate success")
	setup(t)

	attachments := []slack.Attachment{{
		Color: "good",
		Text:  "Apply succeeded for <url|runatlantis/atlantis>",
		Fields: []slack.AttachmentField{
			{
				Title: "Workspace",
				Value: result.Workspace,
				Short: true,
			},
			{
				Title: "User",
				Value: result.User.Username,
				Short: true,
			},
			{
				Title: "Directory",
				Value: result.Directory,
				Short: true,
			},
		},
	}}

	channel := "somechannel"
	err := client.PostMessage(channel, result)
	Ok(t, err)
	underlying.VerifyWasCalledOnce().PostMessage(
		channel,
		slack.MsgOptionAsUser(true),
		slack.MsgOptionText("", false),
		slack.MsgOptionAttachments(attachments[0]),
	)

	t.Log("When apply fails, function should succeed and indicate failure")
	result.Success = false
	attachments[0].Color = "danger"
	attachments[0].Text = "Apply failed for <url|runatlantis/atlantis>"

	err = client.PostMessage(channel, result)
	Ok(t, err)
	underlying.VerifyWasCalledOnce().PostMessage(
		channel,
		slack.MsgOptionAsUser(true),
		slack.MsgOptionText("", false),
		slack.MsgOptionAttachments(attachments[0]),
	)
}

func TestPostMessage_Error(t *testing.T) {
	t.Log("When the underlying slack client errors, an error should be returned")
	setup(t)

	attachments := []slack.Attachment{{
		Color: "good",
		Text:  "Apply succeeded for <url|runatlantis/atlantis>",
		Fields: []slack.AttachmentField{
			{
				Title: "Workspace",
				Value: result.Workspace,
				Short: true,
			},
			{
				Title: "User",
				Value: result.User.Username,
				Short: true,
			},
			{
				Title: "Directory",
				Value: result.Directory,
				Short: true,
			},
		},
	}}

	channel := "somechannel"
	When(underlying.PostMessage(
		channel,
		slack.MsgOptionAsUser(true),
		slack.MsgOptionText("", false),
		slack.MsgOptionAttachments(attachments[0]),
	)).ThenReturn("", "", errors.New(""))

	err := client.PostMessage(channel, result)
	Assert(t, err != nil, "expected error")
}
*/

func setup(t *testing.T) {
	RegisterMockTestingT(t)
	underlying = mocks.NewMockUnderlyingSlackClient()
	client = webhooks.DefaultSlackClient{
		Slack: underlying,
		Token: "sometoken",
	}
	result = webhooks.ApplyResult{
		Workspace: "production",
		Repo: models.Repo{
			FullName: "runatlantis/atlantis",
		},
		Pull: models.PullRequest{
			Num:        1,
			URL:        "url",
			BaseBranch: "main",
		},
		User: models.User{
			Username: "lkysow",
		},
		Success: true,
	}
}

// fakeSlackAPI records chat.postMessage requests made by a real slack client.
type fakeSlackAPI struct {
	server   *httptest.Server
	channels []string
	posted   [][]slack.Attachment
}

func newFakeSlackAPI(t *testing.T) *fakeSlackAPI {
	f := &fakeSlackAPI{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Equals(t, "/chat.postMessage", r.URL.Path)
		Ok(t, r.ParseForm())
		var attachments []slack.Attachment
		Ok(t, json.Unmarshal([]byte(r.PostForm.Get("attachments")), &attachments))
		f.channels = append(f.channels, r.PostForm.Get("channel"))
		f.posted = append(f.posted, attachments)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true, "channel": "C123", "ts": "1.0"}`))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeSlackAPI) client() webhooks.DefaultSlackClient {
	return webhooks.DefaultSlackClient{
		Slack: slack.New("xoxb-test", slack.OptionAPIURL(f.server.URL+"/")),
		Token: "xoxb-test",
	}
}

func driftSlackTemplate(t *testing.T, text string) *template.Template {
	t.Helper()
	slackClient := mocks.NewMockSlackClient()
	When(slackClient.TokenIsSet()).ThenReturn(true)
	sender, err := webhooks.NewDriftWebhookSender([]webhooks.Config{{
		Event:    webhooks.DriftEvent,
		Kind:     webhooks.SlackKind,
		Channel:  "drift-alerts",
		Template: text,
	}}, webhooks.Clients{Slack: slackClient})
	Ok(t, err)
	return sender.Webhooks[0].(*webhooks.DriftSlackWebhook).Template
}

func TestPostDriftMessage_Default(t *testing.T) {
	RegisterMockTestingT(t)
	api := newFakeSlackAPI(t)
	c := api.client()

	err := c.PostDriftMessage("drift-alerts", driftResult, nil)
	Ok(t, err)
	Equals(t, []string{"drift-alerts"}, api.channels)
	Equals(t, 1, len(api.posted[0]))
	attachment := api.posted[0][0]
	Equals(t, "danger", attachment.Color)
	Equals(t, "Drift detected in owner/repo", attachment.Text)
	Equals(t, []string{"fields"}, attachment.MarkdownIn)
	Equals(t, slack.AttachmentField{Title: "Atlantis", Value: "https://atlantis.example.com", Short: true}, attachment.Fields[0])
	last := attachment.Fields[len(attachment.Fields)-1]
	Equals(t, slack.AttachmentField{
		Title: "Drifted projects",
		Value: "• project: `project1` dir: `infra/project1` workspace: `default` — Plan: 1 to add, 2 to change, 0 to destroy.",
	}, last)
}

func TestPostDriftMessage_Template(t *testing.T) {
	RegisterMockTestingT(t)
	api := newFakeSlackAPI(t)
	c := api.client()
	tmpl := driftSlackTemplate(t, "*[prod]* {{ .ProjectsWithDrift }} drifted in {{ .Repository }}")

	err := c.PostDriftMessage("drift-alerts", driftResult, tmpl)
	Ok(t, err)
	Equals(t, [][]slack.Attachment{{{
		Color:      "danger",
		Text:       "*[prod]* 1 drifted in owner/repo",
		MarkdownIn: []string{"text"},
	}}}, api.posted)
}

func TestPostDriftMessage_TemplateRendersNothing(t *testing.T) {
	RegisterMockTestingT(t)
	api := newFakeSlackAPI(t)
	c := api.client()
	tmpl := driftSlackTemplate(t, "{{ if .ProjectsWithDrift }}drift in {{ .Repository }}{{ end }}")
	noDrift := webhooks.DriftResult{Repository: "owner/repo", Ref: "main", TotalProjects: 2}

	err := c.PostDriftMessage("drift-alerts", noDrift, tmpl)
	Ok(t, err)
	Equals(t, 0, len(api.posted))
}

func TestPostDriftMessage_TemplateError(t *testing.T) {
	RegisterMockTestingT(t)
	api := newFakeSlackAPI(t)
	c := api.client()
	// Startup validation rejects this template, so build it directly to check
	// how a runtime rendering error is reported.
	tmpl := template.Must(template.New("drift").Parse("{{ (index .Projects 1).ProjectName }}"))

	err := c.PostDriftMessage("drift-alerts", webhooks.DriftResult{Repository: "owner/repo"}, tmpl)
	ErrContains(t, "rendering drift message template", err)
	Equals(t, 0, len(api.posted))
}
