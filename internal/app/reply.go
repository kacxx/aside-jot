package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kacxx/aside-jot/internal/capture"
)

// maxReplyRunes caps a reply put in an issue body, so a huge one does not
// bury the jot or exceed GitHub's body limit.
const maxReplyRunes = 8000

// replyBefore returns the agent's reply before time t, which is when the jot
// was captured: all the assistant text recorded since the last real user
// prompt. Claude Code writes each content block of a message as its own
// record, and a multi-step turn is many messages with tool calls between, so
// the reply is the text blocks of all of them, in order. Which user records
// start a turn is decided by userRecordKind. The jot's own prompt is blocked,
// so it is not in the transcript; the time alone says where it falls.
func replyBefore(path string, t time.Time) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot read transcript: %w", err)
	}
	defer f.Close()
	var parts []string
	// A slash command starts a new turn only once the agent answers it.
	commandPending := false
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		var rec struct {
			Type        string `json:"type"`
			Timestamp   string `json:"timestamp"`
			IsSidechain bool   `json:"isSidechain"`
			IsMeta      bool   `json:"isMeta"`
			Message     struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &rec) == nil && !rec.IsSidechain && !rec.IsMeta {
			ts, perr := time.Parse(time.RFC3339Nano, rec.Timestamp)
			if perr == nil && !ts.After(t) {
				switch rec.Type {
				case "assistant":
					if text := strings.TrimSpace(contentText(rec.Message.Content, "text")); text != "" {
						if commandPending {
							parts, commandPending = nil, false
						}
						parts = append(parts, text)
					}
				case "user":
					switch userRecordKind(contentText(rec.Message.Content, "text")) {
					case userPrompt:
						parts, commandPending = nil, false
					case userCommand:
						commandPending = true
					}
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return "", fmt.Errorf("cannot read transcript: %w", err)
			}
			break
		}
	}
	if len(parts) == 0 {
		return "", errors.New("no agent reply found before the jot in the transcript")
	}
	return strings.Join(parts, "\n\n"), nil
}

type userKind int

const (
	userOther   userKind = iota // not typed by the user as a request
	userPrompt                  // a prompt; starts a new turn
	userCommand                 // a slash command; may or may not get a reply
)

// notPrompts are the starts of user records that Claude Code writes itself.
// None starts a turn: an interrupt leaves the interrupted turn as the reply,
// a background task's notification arrives in the middle of one, and a local
// command's output is not something the agent answers.
var notPrompts = []string{
	"[Request interrupted by user",
	"<task-notification>",
	"<local-command-stdout>",
	"<local-command-stderr>",
}

// userRecordKind classifies the text of a user record. Tool results have no
// text and jots never reach the model, so neither is a prompt. A slash
// command is recorded the same way whether it is local, like /model, or
// expands to a prompt the agent answers, like a skill, so the caller decides
// from what follows.
func userRecordKind(text string) userKind {
	if strings.TrimSpace(text) == "" {
		return userOther
	}
	if _, ok := capture.Match(text); ok {
		return userOther
	}
	t := strings.TrimSpace(text)
	for _, p := range notPrompts {
		if strings.HasPrefix(t, p) {
			return userOther
		}
	}
	if strings.HasPrefix(t, "<command-name>") || strings.HasPrefix(t, "<command-message>") {
		return userCommand
	}
	return userPrompt
}

// contentText joins the non-blank blocks of the given type in a message's
// content, which is either a string or a list of typed blocks. The text is
// not trimmed, so a jot can be told apart from a prompt that only looks like
// one after trimming.
func contentText(raw json.RawMessage, kind string) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == kind && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// truncateReply cuts s to maxReplyRunes, noting the cut. A code block left
// open by the cut is closed, or the note and the rest of the issue body would
// render as code.
func truncateReply(s string) string {
	rs := []rune(s)
	if len(rs) <= maxReplyRunes {
		return s
	}
	cut := strings.TrimSpace(string(rs[:maxReplyRunes]))
	if fence := openFence(cut); fence != "" {
		cut += "\n" + fence
	}
	return cut + fmt.Sprintf("\n\n_[reply cut: showing the first %d of %d characters]_", maxReplyRunes, len(rs))
}

// openFence returns the fence (such as "```") of a fenced code block that s
// opens and does not close, or "" if there is none.
func openFence(s string) string {
	open := ""
	for _, line := range strings.Split(s, "\n") {
		l := strings.TrimLeft(line, " ")
		if open != "" {
			if strings.HasPrefix(l, open) && strings.Trim(strings.TrimRight(l, " \t"), open[:1]) == "" {
				open = ""
			}
			continue
		}
		for _, c := range []string{"`", "~"} {
			if strings.HasPrefix(l, c+c+c) {
				open = l[:len(l)-len(strings.TrimLeft(l, c))]
				break
			}
		}
	}
	return open
}

// agentReply finds the reply to attach for jot e, or says why it cannot.
func agentReply(e Entry) (string, error) {
	if e.Source != "claude" {
		return "", fmt.Errorf("--with-reply supports Claude Code jots only; jot #%d is from %q", e.ID, e.Source)
	}
	var meta struct {
		TranscriptPath string `json:"transcript_path"`
	}
	if json.Unmarshal(e.Metadata, &meta) != nil || meta.TranscriptPath == "" {
		return "", fmt.Errorf("jot #%d has no transcript path, so there is no agent reply to include", e.ID)
	}
	reply, err := replyBefore(meta.TranscriptPath, e.CreatedAt)
	if err != nil {
		return "", fmt.Errorf("jot #%d: %w (%s)", e.ID, err, meta.TranscriptPath)
	}
	return truncateReply(reply), nil
}
