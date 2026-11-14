package ui

import "time"

// ChatLocale holds the texts of the AI chat components (Bubble,
// BubbleList, Sender, ThoughtChain, ToolCall, Approval, Conversations,
// Prompts). Copy, Copied, Retry, Edit, Save and Cancel come from the
// common Locale fields.
type ChatLocale struct {
	_ [0]func()

	User, Assistant, System string // role names (accessible names of bubbles)
	Messages                string // accessible name of a BubbleList
	ScrollToBottom          string // BubbleList "back to bottom" button
	Thinking                string // loading bubble (accessible name)
	Generating              string // streaming bubble (accessible description)

	Placeholder         string // Sender placeholder
	Send, Stop, Attach  string // Sender buttons
	Suggestions         string // accessible name of the @ / command popup
	Uploading, Failed   string // attachment states
	EditPlaceholder     string // inline edit of a user message
	Pending, Running    string // step states
	Done, Error         string
	StepsFmt            string // chain progress, args: done, total ("%d/%d")
	SecondsFmt          string // duration under a minute, arg: seconds as float ("%.1fs")
	MinutesFmt          string // longer duration, args: minutes, seconds ("%dm %ds")
	ToolCall            string // accessible name prefix of a tool card
	Arguments, Result   string // tool card sections
	ApprovalTitle       string // default Approval title
	AllowOnce           string
	AllowSession        string
	AllowAlways         string
	Deny                string
	Allowed, Denied     string // decided Approval
	FeedbackPlaceholder string // optional note sent with a decision
	RiskWarning         string // risk tag of a warning-level request
	RiskDanger          string // risk tag of a dangerous request
	Conversations       string // accessible name of the list
	Rename              string
	Untitled            string // conversation without a title
	Today, Yesterday    string // time groups
	Last7Days           string
	Last30Days          string
	Earlier             string
	Prompts             string // accessible name of a Prompts group
}

// chatSteps formats a chain progress ("3/5").
func (l *Locale) chatSteps(done, total int) string { return locFmt(l.Chat.StepsFmt, done, total) }

// chatDuration formats a step or tool duration ("1.2s", "2m 5s").
func (l *Locale) chatDuration(d time.Duration) string {
	if d < time.Minute {
		return locFmt(l.Chat.SecondsFmt, d.Seconds())
	}
	s := int(d / time.Second)
	return locFmt(l.Chat.MinutesFmt, s/60, s%60)
}
