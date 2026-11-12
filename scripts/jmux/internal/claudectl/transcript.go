package claudectl

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Message is one turn of the visible conversation in a claude transcript.
type Message struct {
	Role string
	At   time.Time
	Text string
}

// Transcript is the newest transcript claude has written for the workspace
// at path. Claude keys transcripts by cwd, so the workspace need not exist.
func Transcript(path string) (string, error) {
	dir := filepath.Join(os.Getenv("HOME"), ".claude", "projects", projectSlug(path))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", errors.New("no claude transcripts for " + path)
	}
	newest, at := "", time.Time{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(at) {
			newest, at = filepath.Join(dir, e.Name()), info.ModTime()
		}
	}
	if newest == "" {
		return "", errors.New("no claude transcripts for " + path)
	}
	return newest, nil
}

// projectSlug mirrors claude's project directory naming.
func projectSlug(path string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, path)
}

type transcriptEntry struct {
	Type        string    `json:"type"`
	IsMeta      bool      `json:"isMeta"`
	IsSidechain bool      `json:"isSidechain"`
	Timestamp   time.Time `json:"timestamp"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// Tail returns the last n messages of a transcript: the prompts and replies
// a person would see, with tool traffic and injected context left out.
func Tail(r io.Reader, n int) ([]Message, error) {
	var msgs []Message
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if m, ok := messageOf(line); ok {
				if last := len(msgs) - 1; last >= 0 && msgs[last].Role == m.Role {
					msgs[last].Text += "\n\n" + m.Text
				} else {
					msgs = append(msgs, m)
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	if n < len(msgs) {
		msgs = msgs[len(msgs)-n:]
	}
	return msgs, nil
}

func messageOf(line []byte) (Message, bool) {
	var e transcriptEntry
	if json.Unmarshal(line, &e) != nil || e.IsMeta || e.IsSidechain {
		return Message{}, false
	}
	if e.Type != "user" && e.Type != "assistant" {
		return Message{}, false
	}
	text := contentText(e.Message.Content)
	if text == "" {
		return Message{}, false
	}
	return Message{Role: e.Type, At: e.Timestamp, Text: text}, true
}

func contentText(raw json.RawMessage) string {
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
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, strings.TrimSpace(b.Text))
		}
	}
	return strings.Join(parts, "\n\n")
}
