// Copyright 2017 HootSuite Media Inc.
// SPDX-License-Identifier: Apache-2.0
// Modified hereafter by contributors to runatlantis/atlantis.

package webhooks

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/rivo/uniseg"
	"github.com/slack-go/slack"
)

const (
	slackSuccessColour = "good"
	slackFailureColour = "danger"
	// maxDescriptionGraphemeClusters is a readability cap for the pull request
	// description, counted in user-visible grapheme clusters. It includes the
	// trailing ellipsis when the description is truncated.
	maxDescriptionGraphemeClusters = 1000
	// maxDriftProjectsListed caps how many drifted projects the default drift
	// message lists, so a large repository can't exceed Slack's message size
	// limits.
	maxDriftProjectsListed = 20
)

// slackEscaper escapes the characters Slack treats as control sequences in
// message text, so values can't create links or mentions.
var slackEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

//go:generate go tool pegomock generate --package mocks -o mocks/mock_slack_client.go SlackClient

// SlackClient handles making API calls to Slack.
type SlackClient interface {
	AuthTest() error
	TokenIsSet() bool
	PostMessage(channel string, applyResult ApplyResult) error
	// PostDriftMessage posts driftResult to channel. If tmpl is nil the
	// default message is posted. Otherwise tmpl is rendered with the result,
	// and nothing is posted if it renders only whitespace.
	PostDriftMessage(channel string, driftResult DriftResult, tmpl *template.Template) error
}

//go:generate go tool pegomock generate --package mocks -o mocks/mock_underlying_slack_client.go UnderlyingSlackClient

// UnderlyingSlackClient wraps the nlopes/slack.Client implementation so
// we can mock it during tests.
type UnderlyingSlackClient interface {
	AuthTest() (response *slack.AuthTestResponse, error error)
	GetConversations(conversationParams *slack.GetConversationsParameters) (channels []slack.Channel, nextCursor string, err error)
	PostMessage(channelID string, options ...slack.MsgOption) (string, string, error)
}

type DefaultSlackClient struct {
	Slack UnderlyingSlackClient
	Token string
}

func NewSlackClient(token string) SlackClient {
	return &DefaultSlackClient{
		Slack: slack.New(token),
		Token: token,
	}
}

func (d *DefaultSlackClient) AuthTest() error {
	_, err := d.Slack.AuthTest()
	return err
}

func (d *DefaultSlackClient) TokenIsSet() bool {
	return d.Token != ""
}

func (d *DefaultSlackClient) PostMessage(channel string, applyResult ApplyResult) error {
	attachments := d.createAttachments(applyResult)
	_, _, err := d.Slack.PostMessage(
		channel,
		slack.MsgOptionAsUser(true),
		slack.MsgOptionText("", false),
		slack.MsgOptionAttachments(attachments[0]),
	)
	return err
}

func (d *DefaultSlackClient) PostDriftMessage(channel string, driftResult DriftResult, tmpl *template.Template) error {
	var attachment slack.Attachment
	if tmpl == nil {
		attachment = d.createDriftAttachment(driftResult)
	} else {
		text, err := renderDriftTemplate(tmpl, driftResult)
		if err != nil {
			return err
		}
		if text == "" {
			// The template chose not to notify about this result, for
			// example a no-drift heartbeat.
			return nil
		}
		attachment = slack.Attachment{
			Color:      driftColour(driftResult),
			Text:       text,
			MarkdownIn: []string{"text"},
		}
	}
	_, _, err := d.Slack.PostMessage(
		channel,
		slack.MsgOptionAsUser(true),
		slack.MsgOptionText("", false),
		slack.MsgOptionAttachments(attachment),
	)
	return err
}

func (d *DefaultSlackClient) createDriftAttachment(result DriftResult) slack.Attachment {
	result = slackDriftResult(result)
	text := fmt.Sprintf("No drift in %s", result.Repository)
	if result.ProjectsWithDrift > 0 {
		text = fmt.Sprintf("Drift detected in %s", result.Repository)
	}

	var fields []slack.AttachmentField
	if result.AtlantisURL != "" {
		fields = append(fields, slack.AttachmentField{
			Title: "Atlantis",
			Value: result.AtlantisURL,
			Short: true,
		})
	}
	fields = append(fields,
		slack.AttachmentField{
			Title: "Repository",
			Value: result.Repository,
			Short: true,
		},
		slack.AttachmentField{
			Title: "Ref",
			Value: result.Ref,
			Short: true,
		},
		slack.AttachmentField{
			Title: "Projects with drift",
			Value: fmt.Sprintf("%d / %d", result.ProjectsWithDrift, result.TotalProjects),
			Short: true,
		},
		slack.AttachmentField{
			Title: "Detection ID",
			Value: result.DetectionID,
			Short: true,
		},
	)
	if drifted := driftedProjectsList(result.Projects); drifted != "" {
		fields = append(fields, slack.AttachmentField{
			Title: "Drifted projects",
			Value: drifted,
			Short: false,
		})
	}

	return slack.Attachment{
		Color:  driftColour(result),
		Text:   text,
		Fields: fields,
		// The drifted projects list uses code spans.
		MarkdownIn: []string{"fields"},
	}
}

func driftColour(result DriftResult) string {
	if result.ProjectsWithDrift > 0 {
		return slackFailureColour
	}
	return slackSuccessColour
}

// driftedProjectsList returns one line per drifted project, listing at most
// maxDriftProjectsListed of them. The projects must already be prepared by
// slackDriftResult.
func driftedProjectsList(projects []DriftProjectResult) string {
	var lines []string
	drifted := 0
	for _, p := range projects {
		if !p.HasDrift {
			continue
		}
		drifted++
		if drifted > maxDriftProjectsListed {
			continue
		}
		// Identify the project the same way pull request comments do.
		line := "• "
		if p.ProjectName != "" {
			line += fmt.Sprintf("project: %s ", slackCode(p.ProjectName))
		}
		line += fmt.Sprintf("dir: %s workspace: %s", slackCode(p.Path), slackCode(p.Workspace))
		if p.Summary != "" {
			line += " — " + p.Summary
		}
		lines = append(lines, line)
	}
	if hidden := drifted - maxDriftProjectsListed; hidden > 0 {
		lines = append(lines, fmt.Sprintf("…and %d more", hidden))
	}
	return strings.Join(lines, "\n")
}

// slackCode formats s as a Slack code span. Backticks are replaced because
// Slack has no way to escape them inside a code span.
func slackCode(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "'") + "`"
}

// slackDriftResult returns a copy of result prepared for Slack message text:
// string values are escaped so they can't create links or mentions, and
// project summaries are flattened to a single line. The caller's result is
// not modified because it is shared by every configured drift webhook.
func slackDriftResult(result DriftResult) DriftResult {
	result.AtlantisURL = slackEscaper.Replace(result.AtlantisURL)
	result.Repository = slackEscaper.Replace(result.Repository)
	result.Ref = slackEscaper.Replace(result.Ref)
	result.DetectionID = slackEscaper.Replace(result.DetectionID)
	projects := make([]DriftProjectResult, 0, len(result.Projects))
	for _, p := range result.Projects {
		p.ProjectName = slackEscaper.Replace(p.ProjectName)
		p.Path = slackEscaper.Replace(p.Path)
		p.Workspace = slackEscaper.Replace(p.Workspace)
		p.Summary = slackEscaper.Replace(flattenDriftSummary(p.Summary))
		p.Error = slackEscaper.Replace(p.Error)
		projects = append(projects, p)
	}
	result.Projects = projects
	return result
}

// flattenDriftSummary joins the lines of a drift summary into one sentence and
// drops markdown emphasis. Summaries of plans that detected changes made
// outside of Terraform start with a "**Note: ...**" line, which Slack would
// otherwise show with stray asterisks.
func flattenDriftSummary(summary string) string {
	var lines []string
	for line := range strings.Lines(summary) {
		if line = strings.TrimSpace(strings.ReplaceAll(line, "**", "")); line != "" {
			lines = append(lines, line)
		}
	}
	for i := range len(lines) - 1 {
		if !strings.HasSuffix(lines[i], ".") {
			lines[i] += "."
		}
	}
	return strings.Join(lines, " ")
}

// renderDriftTemplate renders a custom drift message. Values are prepared by
// slackDriftResult first, so formatting, links, and mentions can only come
// from the template itself. Surrounding whitespace is trimmed.
func renderDriftTemplate(tmpl *template.Template, result DriftResult) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, slackDriftResult(result)); err != nil {
		return "", fmt.Errorf("rendering drift message template: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

func (d *DefaultSlackClient) createAttachments(applyResult ApplyResult) []slack.Attachment {
	var colour string
	var successWord string
	if applyResult.Success {
		colour = slackSuccessColour
		successWord = "succeeded"
	} else {
		colour = slackFailureColour
		successWord = "failed"
	}

	text := fmt.Sprintf("Apply %s for <%s|%s>", successWord, applyResult.Pull.URL, applyResult.Repo.FullName)
	directory := applyResult.Directory
	// Since "." looks weird, replace it with "/" to make it clear this is the root.
	if directory == "." {
		directory = "/"
	}

	attachment := slack.Attachment{
		Color: colour,
		Text:  text,
		Fields: []slack.AttachmentField{
			{
				Title: "Workspace",
				Value: applyResult.Workspace,
				Short: true,
			},
			{
				Title: "Branch",
				Value: applyResult.Pull.HeadBranch,
				Short: true,
			},
			{
				Title: "User",
				Value: applyResult.User.Username,
				Short: true,
			},
			{
				Title: "Directory",
				Value: directory,
				Short: true,
			},
		},
	}

	// Include the pull request description when present, rendered as a
	// full-width field and truncated so a long description can't exceed Slack's
	// message size limits.
	if applyResult.Pull.Body != "" {
		attachment.Fields = append(attachment.Fields, slack.AttachmentField{
			Title: "Description",
			Value: truncateGraphemeClusters(applyResult.Pull.Body, maxDescriptionGraphemeClusters),
			Short: false,
		})
	}

	return []slack.Attachment{attachment}
}

// truncateGraphemeClusters returns s unchanged when it has at most max
// grapheme clusters. Otherwise it is cut at a cluster boundary to max-1
// clusters with an ellipsis appended. It walks the string without allocating a
// slice for the whole input.
func truncateGraphemeClusters(s string, max int) string {
	if max <= 0 {
		return ""
	}

	count := 0
	keepUntil := 0
	offset := 0
	state := -1
	rest := s
	for len(rest) > 0 {
		cluster, remaining, _, nextState := uniseg.FirstGraphemeClusterInString(rest, state)
		if count == max-1 {
			keepUntil = offset
		}
		if count == max {
			return s[:keepUntil] + "…"
		}
		offset += len(cluster)
		rest = remaining
		state = nextState
		count++
	}
	return s
}
