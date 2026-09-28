// Copyright 2026 The Atlantis Authors
// SPDX-License-Identifier: Apache-2.0

package webhooks

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rivo/uniseg"
	"github.com/runatlantis/atlantis/server/events/models"
	. "github.com/runatlantis/atlantis/testing"
	"github.com/slack-go/slack"
)

func attachmentField(attachments []slack.Attachment, title string) (slack.AttachmentField, bool) {
	if len(attachments) == 0 {
		return slack.AttachmentField{}, false
	}
	for _, f := range attachments[0].Fields {
		if f.Title == title {
			return f, true
		}
	}
	return slack.AttachmentField{}, false
}

func descriptionField(attachments []slack.Attachment) (slack.AttachmentField, bool) {
	return attachmentField(attachments, "Description")
}

func applyResultWithBody(body string) ApplyResult {
	return ApplyResult{
		Workspace: "default",
		Repo:      models.Repo{FullName: "owner/repo"},
		Pull: models.PullRequest{
			Num:        1,
			URL:        "url",
			BaseBranch: "main",
			HeadBranch: "feature-branch",
			Body:       body,
		},
		User:      models.User{Username: "user"},
		Success:   true,
		Directory: "dir",
	}
}

func TestCreateAttachments_UsesHeadBranch(t *testing.T) {
	c := DefaultSlackClient{}
	attachments := c.createAttachments(applyResultWithBody(""))
	field, ok := attachmentField(attachments, "Branch")
	Assert(t, ok, "expected a Branch field")
	Equals(t, slack.AttachmentField{
		Title: "Branch",
		Value: "feature-branch",
		Short: true,
	}, field)
}

func TestCreateAttachments_NoDescriptionWhenBodyEmpty(t *testing.T) {
	c := DefaultSlackClient{}
	attachments := c.createAttachments(applyResultWithBody(""))
	_, ok := descriptionField(attachments)
	Equals(t, false, ok)
}

func TestCreateAttachments_IncludesDescription(t *testing.T) {
	c := DefaultSlackClient{}
	attachments := c.createAttachments(applyResultWithBody("a pull request description"))
	field, ok := descriptionField(attachments)
	Assert(t, ok, "expected a Description field")
	Equals(t, "a pull request description", field.Value)
	Equals(t, false, field.Short)
}

func TestCreateAttachments_TruncatesLongDescription(t *testing.T) {
	c := DefaultSlackClient{}
	attachments := c.createAttachments(applyResultWithBody(strings.Repeat("a", 1500)))
	field, ok := descriptionField(attachments)
	Assert(t, ok, "expected a Description field")
	// The result is capped at maxDescriptionGraphemeClusters, ellipsis included.
	Equals(t, maxDescriptionGraphemeClusters, uniseg.GraphemeClusterCount(field.Value))
	Assert(t, strings.HasSuffix(field.Value, "…"), "expected truncated body to end with an ellipsis")
}

func TestCreateAttachments_DoesNotTruncateAtLimit(t *testing.T) {
	c := DefaultSlackClient{}
	body := strings.Repeat("a", maxDescriptionGraphemeClusters)
	attachments := c.createAttachments(applyResultWithBody(body))
	field, ok := descriptionField(attachments)
	Assert(t, ok, "expected a Description field")
	Equals(t, body, field.Value)
}

func TestCreateAttachments_TruncatesDescriptionAtGraphemeBoundary(t *testing.T) {
	c := DefaultSlackClient{}
	body := strings.Repeat("a", maxDescriptionGraphemeClusters-2) + "🧑‍💻bc"
	attachments := c.createAttachments(applyResultWithBody(body))
	field, ok := descriptionField(attachments)
	Assert(t, ok, "expected a Description field")
	Equals(t, maxDescriptionGraphemeClusters, uniseg.GraphemeClusterCount(field.Value))
	Assert(t, strings.HasSuffix(field.Value, "🧑‍💻…"), "expected truncation to preserve the final emoji grapheme cluster")
	Assert(t, !strings.Contains(field.Value, "b"), "expected truncation before the next grapheme cluster")
}

func driftResultWithProjects(projects ...DriftProjectResult) DriftResult {
	drifted := 0
	for _, p := range projects {
		if p.HasDrift {
			drifted++
		}
	}
	return DriftResult{
		AtlantisURL:       "https://atlantis.prod.example.com",
		Repository:        "owner/repo",
		Ref:               "main",
		DetectionID:       "det-123",
		ProjectsWithDrift: drifted,
		TotalProjects:     len(projects),
		Projects:          projects,
	}
}

func fieldTitles(attachment slack.Attachment) []string {
	var titles []string
	for _, f := range attachment.Fields {
		titles = append(titles, f.Title)
	}
	return titles
}

func TestCreateDriftAttachment_IdentifiesInstanceAndDriftedProjects(t *testing.T) {
	c := DefaultSlackClient{}
	attachment := c.createDriftAttachment(driftResultWithProjects(
		DriftProjectResult{
			ProjectName: "app",
			Path:        "envs/prod/app",
			Workspace:   "default",
			HasDrift:    true,
			ToAdd:       1,
			Summary:     "Plan: 1 to add, 0 to change, 0 to destroy.",
		},
		DriftProjectResult{
			Path:      ".",
			Workspace: "staging",
			HasDrift:  true,
			ToDestroy: 1,
			Summary:   "Plan: 0 to add, 0 to change, 1 to destroy.",
		},
		DriftProjectResult{
			ProjectName: "clean",
			Path:        "envs/prod/clean",
			Workspace:   "default",
			Summary:     "No changes. Your infrastructure matches the configuration.",
		},
	))

	Equals(t, slackFailureColour, attachment.Color)
	Equals(t, "Drift detected in owner/repo", attachment.Text)
	Equals(t, []string{"fields"}, attachment.MarkdownIn)
	Equals(t, []string{"Atlantis", "Repository", "Ref", "Projects with drift", "Detection ID", "Drifted projects"}, fieldTitles(attachment))

	atlantis, _ := attachmentField([]slack.Attachment{attachment}, "Atlantis")
	Equals(t, slack.AttachmentField{Title: "Atlantis", Value: "https://atlantis.prod.example.com", Short: true}, atlantis)
	count, _ := attachmentField([]slack.Attachment{attachment}, "Projects with drift")
	Equals(t, "2 / 3", count.Value)

	drifted, _ := attachmentField([]slack.Attachment{attachment}, "Drifted projects")
	Equals(t, false, drifted.Short)
	Equals(t, "• project: `app` dir: `envs/prod/app` workspace: `default` — Plan: 1 to add, 0 to change, 0 to destroy.\n"+
		"• dir: `.` workspace: `staging` — Plan: 0 to add, 0 to change, 1 to destroy.", drifted.Value)
}

func TestCreateDriftAttachment_NoDrift(t *testing.T) {
	c := DefaultSlackClient{}
	attachment := c.createDriftAttachment(driftResultWithProjects(
		DriftProjectResult{ProjectName: "clean", Path: "clean", Workspace: "default"},
	))

	Equals(t, slackSuccessColour, attachment.Color)
	Equals(t, "No drift in owner/repo", attachment.Text)
	Equals(t, []string{"Atlantis", "Repository", "Ref", "Projects with drift", "Detection ID"}, fieldTitles(attachment))
}

func TestCreateDriftAttachment_OmitsEmptyAtlantisURL(t *testing.T) {
	c := DefaultSlackClient{}
	result := driftResultWithProjects()
	result.AtlantisURL = ""
	attachment := c.createDriftAttachment(result)

	_, ok := attachmentField([]slack.Attachment{attachment}, "Atlantis")
	Equals(t, false, ok)
}

func TestCreateDriftAttachment_CapsDriftedProjects(t *testing.T) {
	var projects []DriftProjectResult
	for i := range maxDriftProjectsListed + 5 {
		projects = append(projects, DriftProjectResult{
			ProjectName: fmt.Sprintf("project%d", i),
			Path:        fmt.Sprintf("project%d", i),
			Workspace:   "default",
			HasDrift:    true,
		})
	}
	c := DefaultSlackClient{}
	attachment := c.createDriftAttachment(driftResultWithProjects(projects...))

	drifted, ok := attachmentField([]slack.Attachment{attachment}, "Drifted projects")
	Assert(t, ok, "expected a Drifted projects field")
	lines := strings.Split(drifted.Value, "\n")
	Equals(t, maxDriftProjectsListed+1, len(lines))
	Equals(t, "• project: `project19` dir: `project19` workspace: `default`", lines[maxDriftProjectsListed-1])
	Equals(t, "…and 5 more", lines[maxDriftProjectsListed])
}

func TestCreateDriftAttachment_FlattensChangesOutsideSummary(t *testing.T) {
	c := DefaultSlackClient{}
	attachment := c.createDriftAttachment(driftResultWithProjects(DriftProjectResult{
		ProjectName:    "iam",
		Path:           "iam",
		Workspace:      "default",
		HasDrift:       true,
		ChangesOutside: true,
		ToChange:       1,
		Summary:        "\n**Note: Objects have changed outside of Terraform**\nPlan: 0 to add, 1 to change, 0 to destroy.",
	}))

	drifted, _ := attachmentField([]slack.Attachment{attachment}, "Drifted projects")
	Equals(t, "• project: `iam` dir: `iam` workspace: `default` — Note: Objects have changed outside of Terraform. Plan: 0 to add, 1 to change, 0 to destroy.", drifted.Value)
}

func TestCreateDriftAttachment_EscapesValues(t *testing.T) {
	c := DefaultSlackClient{}
	result := driftResultWithProjects(DriftProjectResult{
		ProjectName: "<!channel> `x`",
		Path:        "a&b",
		Workspace:   "<https://example.com|ws>",
		HasDrift:    true,
	})
	result.Repository = "owner/<repo>"
	attachment := c.createDriftAttachment(result)

	Equals(t, "Drift detected in owner/&lt;repo&gt;", attachment.Text)
	drifted, _ := attachmentField([]slack.Attachment{attachment}, "Drifted projects")
	Equals(t, "• project: `&lt;!channel&gt; 'x'` dir: `a&amp;b` workspace: `&lt;https://example.com|ws&gt;`", drifted.Value)
}

func TestFlattenDriftSummary(t *testing.T) {
	cases := map[string]string{
		"": "",
		"Plan: 1 to add, 0 to change, 0 to destroy.":                                                                        "Plan: 1 to add, 0 to change, 0 to destroy.",
		"\n**Note: Objects have changed outside of Terraform**\nPlan: 0 to add, 1 to change, 0 to destroy.":                 "Note: Objects have changed outside of Terraform. Plan: 0 to add, 1 to change, 0 to destroy.",
		"\n**Note: Objects have changed outside of Terraform**\nNo changes. Your infrastructure matches the configuration.": "Note: Objects have changed outside of Terraform. No changes. Your infrastructure matches the configuration.",
		"\n**Note: Objects have changed outside of Terraform**\n":                                                           "Note: Objects have changed outside of Terraform",
	}
	for summary, exp := range cases {
		Equals(t, exp, flattenDriftSummary(summary))
	}
}

func TestRenderDriftTemplate(t *testing.T) {
	tmpl, err := parseDriftTemplate(`
*[prod]* drift in {{ .Repository | upper }} ({{ .ProjectsWithDrift }}/{{ .TotalProjects }}) from {{ .AtlantisURL }}
{{- range .Projects }}{{ if .HasDrift }}
• {{ .ProjectName }} in {{ .Path }}: {{ .Summary }}
{{- end }}{{ end }}
`)
	Ok(t, err)
	result := driftResultWithProjects(
		DriftProjectResult{
			ProjectName:    "<!here>",
			Path:           "a&b",
			Workspace:      "default",
			HasDrift:       true,
			ChangesOutside: true,
			Summary:        "\n**Note: Objects have changed outside of Terraform**\nPlan: 0 to add, 1 to change, 0 to destroy.",
		},
		DriftProjectResult{ProjectName: "clean", Path: "clean", Workspace: "default"},
	)

	text, err := renderDriftTemplate(tmpl, result)
	Ok(t, err)
	Equals(t, "*[prod]* drift in OWNER/REPO (1/2) from https://atlantis.prod.example.com\n"+
		"• &lt;!here&gt; in a&amp;b: Note: Objects have changed outside of Terraform. Plan: 0 to add, 1 to change, 0 to destroy.", text)
	// The result is shared by every drift webhook, so it must not be modified.
	Equals(t, "<!here>", result.Projects[0].ProjectName)
	Equals(t, "\n**Note: Objects have changed outside of Terraform**\nPlan: 0 to add, 1 to change, 0 to destroy.", result.Projects[0].Summary)
}

func TestRenderDriftTemplate_BlankOutput(t *testing.T) {
	tmpl, err := parseDriftTemplate("{{ if .ProjectsWithDrift }}Drift in {{ .Repository }}{{ end }}\n")
	Ok(t, err)

	text, err := renderDriftTemplate(tmpl, driftResultWithProjects(DriftProjectResult{ProjectName: "clean"}))
	Ok(t, err)
	Equals(t, "", text)
}
