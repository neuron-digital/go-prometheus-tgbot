package jira

import (
	"fmt"
	"github.com/andygrunwald/go-jira"
	"github.com/neuron-digital/go-prometheus-tgbot/utils"
	"net/url"
	"strings"
)

type Event struct {
	WebhookEvent       string                `json:"webhookEvent"`
	IssueEventTypeName string                `json:"issue_event_type_name,omitempty"`
	User               jira.User             `json:"user"`
	Issue              jira.Issue            `json:"issue,omitempty"`
	Changelog          jira.ChangelogHistory `json:"changelog,omitempty"`
	Comment            jira.Comment          `json:"comment,omitempty"`
}

type IssueCreated struct{ Event }
type IssueDeleted struct{ Event }
type IssueRenamed struct{ Event }
type IssueCommented struct{ Event }

type IssueGenericUpdate struct{ Event }
type IssueFieldUpdated struct{ Event }

type IssueTimeSpent struct{ Event }

type IssueAssigned struct{ Event }
type IssueUnassigned struct{ Event }
type IssueReassigned struct{ Event }

type IssueAttachmentCreated struct{ Event }
type IssueAttachmentDeleted struct{ Event }

type Unknown struct{ Event }

func (event IssueCreated) String() string {
	return fmt.Sprintf("%s created %s", event.User.DisplayName, event.getLink())
}

func (event IssueDeleted) String() string {
	return fmt.Sprintf("%s deleted %s", event.User.DisplayName, event.getLink())
}

func (event IssueRenamed) String() string {
	return fmt.Sprintf("%s renamed <a href=\"%s\">%s</a> to <a href=\"%s\">%s</a>", event.User.DisplayName, event.getUrl(), utils.Strike(event.Changelog.Items[0].FromString), event.getUrl(), event.Changelog.Items[0].ToString)
}

func (event IssueCommented) String() string {
	return fmt.Sprintf("%s\n<b>%s:</b> %s", event.getUrl(), event.User.DisplayName, event.Comment.Body)
}

func (event IssueAssigned) String() string {
	return fmt.Sprintf("%s assigned to %s", event.getLink(), event.Changelog.Items[0].ToString)
}

func (event IssueUnassigned) String() string {
	return fmt.Sprintf("%s unassigned from %s", event.getLink(), utils.Strike(event.Changelog.Items[0].FromString))
}

func (event IssueReassigned) String() string {
	return fmt.Sprintf("%s reassigned from %s to %s", event.getLink(), utils.Strike(event.Changelog.Items[0].FromString), event.Changelog.Items[0].ToString)
}

func (event IssueGenericUpdate) String() string {
	change := event.Changelog.Items[0]
	return fmt.Sprintf("%s. %s changed <b>%s</b> from \"%s\" to \"%s\"", event.getLink(), event.User.DisplayName, change.Field, utils.Strike(change.FromString), change.ToString)
}

func (event IssueAttachmentCreated) String() string {
	change := event.Changelog.Items[0]
	to, _ := change.To.(int)
	attachment := event.Issue.Fields.Attachments[to]
	return fmt.Sprintf("%s attached <a href=\"%s\">%s</a>\n<b>Type:</b> %s\n<b>Size:</b> %d bytes",
		event.User.DisplayName,
		attachment.Content, attachment.Filename, attachment.MimeType, attachment.Size)
}

func (event IssueAttachmentDeleted) String() string {
	change := event.Changelog.Items[0]
	return fmt.Sprintf("%s deleted %s", event.User.DisplayName, utils.Strike(change.FromString))
}

func (event IssueFieldUpdated) String() string {
	change := event.Changelog.Items[0]
	return fmt.Sprintf("%s. %s changed <b>%s</b> from \"%s\" to \"%s\"", event.getLink(), event.User.DisplayName, change.Field, utils.Strike(change.FromString), change.ToString)
}

func (event IssueTimeSpent) String() string {
	comment := event.Issue.Fields.Worklog.Worklogs[len(event.Issue.Fields.Worklog.Worklogs)-1].Comment
	if comment != "" {
		comment = "\n" + comment
	}

	remaining := event.Issue.Fields.TimeTracking.RemainingEstimate
	change := event.Changelog.Items[0]
	to, _ := change.To.(int)
	from, _ := change.From.(int)
	spent := to - from
	return fmt.Sprintf("%s\n%s contributed <b>%d</b> (%s remains)%s", event.getLink(), event.User.DisplayName, spent, remaining, comment)
}

func (event Unknown) String() string {
	return fmt.Sprintf("%+v", event.Event)
}

func withComment(s string, event Event) string {
	if event.Comment.Body == "" {
		return s
	}

	return s + "\n" + event.Comment.Body
}

func (event *Event) getUrl() string {
	if u, err := url.Parse(event.Issue.Self); err == nil {
		return fmt.Sprintf("%s://%s/browse/%s", u.Scheme, u.Host, event.Issue.Key)
	}
	return ""
}

func (event *Event) getLink() string {
	return fmt.Sprintf("<a href=\"%s\">%s</a>", event.getUrl(), event.Issue.Fields.Summary)
}

func (event Event) ComposeMessage() string {
	if len(event.Changelog.Items) > 1 {
		var changes []string
		for _, change := range event.Changelog.Items {
			eventCopy := event
			eventCopy.Changelog.Items = []jira.ChangelogItems{change}
			if msg := eventCopy.ComposeMessage(); msg != "" {
				changes = append(changes, msg)
			}
		}
		return strings.Join(changes, "\n\n")
	}

	switch event.WebhookEvent {
	case "jira:issue_created":
		return withComment(IssueCreated{event}.String(), event)

	case "jira:issue_deleted":
		return withComment(IssueDeleted{event}.String(), event)

	case "jira:issue_updated":
		switch event.IssueEventTypeName {
		case "issue_updated":
			switch event.Changelog.Items[0].Field {
			case "Attachment":
				if event.Changelog.Items[0].From == nil {
					return withComment(IssueAttachmentCreated{event}.String(), event)
				}

				if event.Changelog.Items[0].To == nil {
					return withComment(IssueAttachmentDeleted{event}.String(), event)
				}

			case "summary":
				return withComment(IssueRenamed{event}.String(), event)
			}

			return withComment(IssueFieldUpdated{event}.String(), event)

		case "issue_commented":
			return IssueCommented{event}.String()

		case "issue_assigned":
			switch {
			case event.Changelog.Items[0].From == nil:
				return withComment(IssueAssigned{event}.String(), event)
			case event.Changelog.Items[0].To == nil:
				return withComment(IssueUnassigned{event}.String(), event)
			default:
				return withComment(IssueReassigned{event}.String(), event)
			}

		case "issue_generic":
			return withComment(IssueGenericUpdate{event}.String(), event)
		}
	case "jira:worklog_updated":
		if event.Changelog.Items[0].Field == "timespent" {
			return withComment(IssueTimeSpent{event}.String(), event)
		} else {
			return ""
		}
	}

	return withComment(Unknown{event}.String(), event)
}
