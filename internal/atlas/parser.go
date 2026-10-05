package atlas

import (
	"bufio"
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type counters struct {
	Input     int64 `json:"input_tokens"`
	Cached    int64 `json:"cached_input_tokens"`
	Output    int64 `json:"output_tokens"`
	Reasoning int64 `json:"reasoning_output_tokens"`
	Total     int64 `json:"total_tokens"`
}
type usageEvent struct {
	Key, Project, Day string
	Usage             Usage
}
type record struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}
type payload struct {
	Input     json.RawMessage `json:"input"`
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Cwd       string          `json:"cwd"`
	Timestamp string          `json:"timestamp"`
	Parent    string          `json:"forked_from_id"`
	Model     string          `json:"model"`
	TurnID    string          `json:"turn_id"`
	Role      string          `json:"role"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Output    json.RawMessage `json:"output"`
	Text      string          `json:"text"`
	Message   string          `json:"message"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Info *struct {
		Total counters `json:"total_token_usage"`
		Last  counters `json:"last_token_usage"`
	} `json:"info"`
}
type parsed struct {
	Session Session
	Events  []usageEvent
	Refs    []Evidence
}

// Lines can include screenshots. Read with a buffered reader rather than Scanner's 64 KiB limit.
// Reject >32 MiB records explicitly; callers retain the previous complete index on failure.
func readRecords(path string, visit func(record, payload) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	rd := bufio.NewReaderSize(f, 256*1024)
	lineNo := 0
	for {
		var line []byte
		for {
			part, more, e := rd.ReadLine()
			if e != nil {
				if e == io.EOF && len(line) == 0 {
					return nil
				}
				return e
			}
			line = append(line, part...)
			if len(line) > 32*1024*1024 {
				return fmt.Errorf("record exceeds 32 MiB")
			}
			if !more {
				break
			}
		}
		lineNo++
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r record
		if err = json.Unmarshal(line, &r); err != nil {
			if _, e := rd.Peek(1); e == io.EOF {
				return fmt.Errorf("末尾记录尚未写完，请重新扫描")
			}
			return fmt.Errorf("第 %d 行: %w", lineNo, err)
		}
		var p payload
		if err = json.Unmarshal(r.Payload, &p); err != nil {
			continue
		}
		if err = visit(r, p); err != nil {
			return err
		}
	}
}

var absolutePath = regexp.MustCompile(`/(?:private/)?tmp/[^\s"'<>\\\x60;,()\[\]{}]+|/var/folders/[^\s"'<>\\\x60;,()\[\]{}]+|/Users/[^\s"'<>\\\x60;,()\[\]{}]+/\.codex/\.?tmp/[^\s"'<>\\\x60;,()\[\]{}]+`)

func parseSession(path string, info os.FileInfo) (parsed, error) {
	out := parsed{Session: Session{Path: path, Size: info.Size(), Mtime: info.ModTime().UnixNano(), Archived: strings.Contains(path, string(filepath.Separator)+"archived_sessions"+string(filepath.Separator))}}
	project, turn := "", ""
	var prev counters
	seen := map[string]bool{}
	refs := map[string]bool{}
	line := 0
	err := readRecords(path, func(r record, p payload) error {
		line++
		if r.Type == "session_meta" {
			out.Session.ID = p.ID
			out.Session.Project = p.Cwd
			out.Session.Created = cmp.Or(p.Timestamp, r.Timestamp)
			out.Session.Parent = p.Parent
			project = p.Cwd
		}
		if r.Type == "turn_context" {
			project = cmp.Or(p.Cwd, project)
			turn = p.TurnID
			out.Session.Model = cmp.Or(p.Model, out.Session.Model)
		}
		out.Session.Updated = cmp.Or(r.Timestamp, out.Session.Updated)
		if out.Session.Title == "" && r.Type == "event_msg" && p.Type == "user_message" {
			out.Session.Title = shortText(p.Message, 100)
		}
		if out.Session.Title == "" && r.Type == "response_item" && p.Type == "message" && p.Role == "user" {
			for _, c := range p.Content {
				if !strings.HasPrefix(c.Text, "<") && !strings.HasPrefix(c.Text, "#") {
					out.Session.Title = shortText(c.Text, 100)
					break
				}
			}
		}
		if p.Type == "token_count" && p.Info != nil {
			now := p.Info.Total
			if now == prev {
				return nil
			}
			// A reset starts a new counter segment. last_token_usage is preferred over replaying a total.
			d := counters{Input: now.Input - prev.Input, Cached: now.Cached - prev.Cached, Output: now.Output - prev.Output, Reasoning: now.Reasoning - prev.Reasoning, Total: now.Total - prev.Total}
			if d.Input < 0 || d.Output < 0 || d.Total < 0 {
				d = p.Info.Last
				out.Session.Warning = "检测到计数器重置，按最后请求用量续计"
			}
			if prev.Total == 0 && p.Info.Last.Total > 0 && now.Total > p.Info.Last.Total {
				d = p.Info.Last
				out.Session.Warning = "日志起点含历史累计，未计入缺失的历史请求"
			}
			prev = now
			identity := cmp.Or(turn, r.Timestamp+out.Session.ID)
			sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%+v|%+v", identity, now, p.Info.Last)))
			key := hex.EncodeToString(sum[:])
			if seen[key] {
				return nil
			}
			seen[key] = true
			day := ""
			if t, e := time.Parse(time.RFC3339Nano, r.Timestamp); e == nil {
				day = t.Local().Format("2006-01-02")
			}
			out.Events = append(out.Events, usageEvent{key, project, day, Usage{max(0, d.Input), max(0, d.Cached), max(0, d.Output), max(0, d.Reasoning), max(0, d.Total)}})
		}
		// Only tool arguments/results are evidence. Merely discussing a path is not attribution.
		kind, text := "", ""
		if r.Type == "response_item" && (p.Type == "function_call" || p.Type == "custom_tool_call") {
			kind = "命令引用"
			text = p.Arguments
			if p.Type == "custom_tool_call" {
				text = string(p.Input)
			}
		}
		if r.Type == "response_item" && (p.Type == "function_call_output" || p.Type == "custom_tool_call_output") {
			kind = "工具输出"
			text = string(p.Output)
		}
		if kind != "" {
			text = strings.ReplaceAll(text, `\n`, "\n")
			text = strings.ReplaceAll(text, `\"`, `"`)
			for _, path := range absolutePath.FindAllString(text, -1) {
				path = canonicalTmp(strings.TrimRight(path, ".:"))
				key := path + project + kind
				if refs[key] {
					continue
				}
				refs[key] = true
				out.Refs = append(out.Refs, Evidence{Project: project, Path: path, Kind: kind})
			}
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	if out.Session.ID == "" {
		return out, fmt.Errorf("缺少 session_meta")
	}
	out.Session.Title = cmp.Or(out.Session.Title, "未命名会话")
	if out.Session.Parent != "" {
		out.Session.Warning = cmp.Or(out.Session.Warning, "分叉历史按 turn ID 与用量快照去重；缺失父日志时归属可能不完整")
	}
	for i := range out.Refs {
		out.Refs[i].SessionID = out.Session.ID
	}
	return out, nil
}
func canonicalTmp(p string) string {
	p = filepath.Clean(p)
	if strings.HasPrefix(p, "/private/tmp/") {
		p = strings.TrimPrefix(p, "/private")
	}
	if strings.HasPrefix(p, "/private/var/") {
		p = strings.TrimPrefix(p, "/private")
	}
	return p
}
func shortText(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}
