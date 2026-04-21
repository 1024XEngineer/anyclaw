package extension

func builtinExtensionManifests() []Manifest {
	return []Manifest{
		{ID: "email", Name: "Email", Version: "1.0.0", Description: "Inbound and outbound email automation hooks", Kind: "channel", Builtin: true, Channels: []string{"email"}},
		{ID: "sms", Name: "SMS", Version: "1.0.0", Description: "SMS notifications and reply workflows", Kind: "channel", Builtin: true, Channels: []string{"sms"}},
		{ID: "webhook", Name: "Webhook", Version: "1.0.0", Description: "Generic webhook ingestion and dispatch", Kind: "hook", Builtin: true},
		{ID: "rss", Name: "RSS Watcher", Version: "1.0.0", Description: "RSS feed ingestion and summarization", Kind: "hook", Builtin: true},
		{ID: "github-webhook", Name: "GitHub Webhook", Version: "1.0.0", Description: "GitHub issue and PR event adapter", Kind: "hook", Builtin: true, Skills: []string{"github", "github-issues"}},
		{ID: "gitlab", Name: "GitLab", Version: "1.0.0", Description: "GitLab notifications and merge event routing", Kind: "channel", Builtin: true, Channels: []string{"gitlab"}},
		{ID: "jira", Name: "Jira", Version: "1.0.0", Description: "Jira issue stream and workflow hooks", Kind: "tool", Builtin: true},
		{ID: "confluence", Name: "Confluence", Version: "1.0.0", Description: "Confluence page sync and note surfaces", Kind: "tool", Builtin: true},
		{ID: "notion", Name: "Notion", Version: "1.0.0", Description: "Notion knowledge capture and publishing", Kind: "tool", Builtin: true},
		{ID: "linear", Name: "Linear", Version: "1.0.0", Description: "Linear issue routing and sprint actions", Kind: "tool", Builtin: true},
		{ID: "reddit", Name: "Reddit", Version: "1.0.0", Description: "Reddit thread ingestion and moderation hooks", Kind: "channel", Builtin: true, Channels: []string{"reddit"}},
		{ID: "x", Name: "X", Version: "1.0.0", Description: "X posting and mention handling surface", Kind: "channel", Builtin: true, Channels: []string{"x"}},
		{ID: "mastodon", Name: "Mastodon", Version: "1.0.0", Description: "Mastodon timeline and social publishing support", Kind: "channel", Builtin: true, Channels: []string{"mastodon"}},
		{ID: "linkedin", Name: "LinkedIn", Version: "1.0.0", Description: "LinkedIn publishing and outreach automation", Kind: "channel", Builtin: true, Channels: []string{"linkedin"}},
		{ID: "youtube", Name: "YouTube", Version: "1.0.0", Description: "YouTube publishing and comment triage", Kind: "channel", Builtin: true, Channels: []string{"youtube"}},
		{ID: "outlook", Name: "Outlook", Version: "1.0.0", Description: "Outlook calendar and mailbox integration", Kind: "channel", Builtin: true, Channels: []string{"outlook"}},
		{ID: "zoom", Name: "Zoom", Version: "1.0.0", Description: "Zoom meeting event hooks and summaries", Kind: "hook", Builtin: true},
		{ID: "webex", Name: "Webex", Version: "1.0.0", Description: "Webex meetings and chat event adapter", Kind: "channel", Builtin: true, Channels: []string{"webex"}},
		{ID: "basecamp", Name: "Basecamp", Version: "1.0.0", Description: "Basecamp project update integration", Kind: "tool", Builtin: true},
		{ID: "asana", Name: "Asana", Version: "1.0.0", Description: "Asana project and task synchronization", Kind: "tool", Builtin: true},
		{ID: "clickup", Name: "ClickUp", Version: "1.0.0", Description: "ClickUp task and workflow hooks", Kind: "tool", Builtin: true},
		{ID: "webhook-relay", Name: "Webhook Relay", Version: "1.0.0", Description: "Shared webhook relay and routing helper", Kind: "hook", Builtin: true},
	}
}
