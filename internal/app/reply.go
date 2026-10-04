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
)

// maxReplyRunes caps a reply put in an issue body, so a huge one does not
// bury the jot or exceed GitHub's body limit.
const maxReplyRunes = 8000

// replyBefore returns the agent's reply before time t, which is when the jot
// was captured: all the assistant text recorded since the last real user
// prompt. Claude Code writes each content block of a message as its own
// record, and a multi-step turn is many messages with tool calls between, so
// the reply is the text blocks of all of them, in order. Tool results come
// back as user records but are not prompts. The jot's own prompt is blocked,
// so it is not in the transcript; the time alone says where it falls.
func replyBefore(path string, t time.Time) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot read transcript: %w", err)
	}
	defer f.Close()
	var parts []string
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
					if text := contentText(rec.Message.Content, "text"); text != "" {
						parts = append(parts, text)
					}
				case "user":
					// A prompt starts a new turn. A tool result does not, and
					// neither does a jot (">>"), which is blocked from the model.
					if text := contentText(rec.Message.Content, "text"); text != "" && !strings.HasPrefix(text, ">>") {
						parts = nil
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

// contentText joins the blocks of the given type in a message's content,
// which is either a string or a list of typed blocks.
func contentText(raw json.RawMessage, kind string) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
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
			parts = append(parts, strings.TrimSpace(b.Text))
		}
	}
	return strings.Join(parts, "\n\n")
}

// truncateReply cuts s to maxReplyRunes, noting the cut.
func truncateReply(s string) string {
	rs := []rune(s)
	if len(rs) <= maxReplyRunes {
		return s
	}
	return strings.TrimSpace(string(rs[:maxReplyRunes])) +
		fmt.Sprintf("\n\n_[reply cut: showing the first %d of %d characters]_", maxReplyRunes, len(rs))
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
