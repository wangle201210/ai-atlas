package atlas

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const parserVersion = "5"

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
	Offset    int64           `json:"-"`
	Length    int64           `json:"-"`
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
	CallID    string          `json:"call_id"`
	Arguments json.RawMessage `json:"arguments"`
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
type indexedMessage struct {
	Offset, Length int64
	Role, Time     string
}
type parsed struct {
	Messages []indexedMessage
	Session  Session
	Events   []usageEvent
	Refs     []Evidence
}

// Lines can include screenshots. Read with a buffered reader rather than Scanner's 64 KiB limit.
// Reject >32 MiB records explicitly; callers retain the previous complete index on failure.
func readRecords(path string, visit func(record, payload) error) error {
	return readRecordsContext(context.Background(), path, visit)
}
func readRecordsContext(ctx context.Context, path string, visit func(record, payload) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	rd := bufio.NewReaderSize(f, 256*1024)
	var offset int64
	lineNo := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var line []byte
		var readErr error
		for {
			part, err := rd.ReadSlice('\n')
			line = append(line, part...)
			if len(line) > 32*1024*1024 {
				return errors.New("日志单行超过 32 MiB")
			}
			if errors.Is(err, bufio.ErrBufferFull) {
				if e := ctx.Err(); e != nil {
					return e
				}
				continue
			}
			readErr = err
			break
		}
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		start := offset
		offset += int64(len(line))
		lineNo++
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r record
		if err := json.Unmarshal(line, &r); err != nil {
			return fmt.Errorf("第 %d 行无效或尚未写完: %w", lineNo, err)
		}
		r.Offset = start
		r.Length = int64(len(line))
		var p payload
		if err := json.Unmarshal(r.Payload, &p); err != nil {
			return fmt.Errorf("第 %d 行 payload 无效: %w", lineNo, err)
		}
		if err := visit(r, p); err != nil {
			return err
		}
	}
}

var absolutePath = regexp.MustCompile(`/(?:private/)?tmp/[^\s"'<>\\\x60;,()\[\]{}]+|/var/folders/[^\s"'<>\\\x60;,()\[\]{}]+|/Users/[^\s"'<>\\\x60;,()\[\]{}]+/\.codex/\.?tmp/[^\s"'<>\\\x60;,()\[\]{}]+`)

func parseSession(path string, info os.FileInfo) (parsed, error) {
	return parseSessionContext(context.Background(), path, info)
}
func parseSessionContext(ctx context.Context, path string, info os.FileInfo) (parsed, error) {
	out := parsed{Session: Session{Path: path, Size: info.Size(), Mtime: info.ModTime().UnixNano(), Archived: strings.Contains(path, string(filepath.Separator)+"archived_sessions"+string(filepath.Separator))}}
	project, turn := "", ""
	var prev counters
	seen := map[string]bool{}
	refs := map[string]bool{}
	creations := map[string][]string{}
	line := 0
	err := readRecordsContext(ctx, path, func(r record, p payload) error {
		line++
		if r.Type == "session_meta" {
			// Fork rollouts can replay their parent's session_meta after their
			// own header. Only the first header owns this physical log file.
			if out.Session.ID == "" {
				if p.ID == "" {
					return errors.New("会话首个 session_meta 缺少 ID")
				}
				out.Session.ID = p.ID
				out.Session.Project = p.Cwd
				out.Session.Created = cmp.Or(p.Timestamp, r.Timestamp)
				out.Session.Parent = p.Parent
			}
			// Replayed history can still supply context for its usage events.
			project = cmp.Or(p.Cwd, project)
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
		if r.Type == "response_item" && p.Type == "message" && (p.Role == "user" || p.Role == "assistant") {
			text := messageText(p)
			if text != "" && !strings.HasPrefix(text, "<environment_context>") && !strings.HasPrefix(text, "<codex_internal_context") {
				out.Messages = append(out.Messages, indexedMessage{r.Offset, r.Length, p.Role, r.Timestamp})
			}
		}
		if r.Type == "token_usage_record" || p.Type == "token_usage_record" {
			out.Session.Warning = "存在独立压缩用量记录，当前统计可能不完整"
		}
		if p.Type == "token_count" && p.Info != nil {
			out.Session.HasUsage = true
			if turn == "" {
				out.Session.Warning = "缺少请求 turn ID，跨会话去重可能不完整"
			}
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
			text = argumentText(p.Arguments)
			if p.Type == "custom_tool_call" {
				text = string(p.Input)
			}
		}
		if r.Type == "response_item" && (p.Type == "function_call_output" || p.Type == "custom_tool_call_output") {
			kind = "工具输出"
			text = string(p.Output)
		}
		if p.Type == "function_call" && (strings.Contains(p.Name, "exec_command") || strings.Contains(p.Name, "shell")) {
			var args struct {
				Cmd     string `json:"cmd"`
				Command string `json:"command"`
			}
			if json.Unmarshal([]byte(argumentText(p.Arguments)), &args) == nil {
				cmd := cmp.Or(args.Cmd, args.Command)
				for _, match := range mkdirCommand.FindAllStringSubmatch(cmd, -1) {
					creations[p.CallID] = append(creations[p.CallID], canonicalTmp(match[1]))
				}
			}
		}
		if p.Type == "function_call_output" && p.CallID != "" {
			text := string(p.Output)
			if strings.Contains(text, `exit_code\":0`) || strings.Contains(text, `"exit_code":0`) || strings.Contains(text, "Process exited with code 0") {
				for _, path := range creations[p.CallID] {
					out.Refs = append(out.Refs, Evidence{Project: project, Path: path, Kind: "创建操作记录"})
				}
			}
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
	out.Session.Completeness = "complete"
	if !out.Session.HasUsage {
		out.Session.Completeness = "none"
	} else if out.Session.Warning != "" || out.Session.Parent != "" {
		out.Session.Completeness = "partial"
	}
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

func messageText(p payload) string {
	var b strings.Builder
	for _, c := range p.Content {
		if c.Text != "" {
			b.WriteString(c.Text)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

var mkdirCommand = regexp.MustCompile(`(?:^|[;\n])\s*mkdir\s+(?:-p\s+)?["']?(/(?:tmp|private/tmp|var/folders)/[^\s"';]+)`)

func argumentText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return string(raw)
}
