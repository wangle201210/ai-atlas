package atlas

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
)

// SessionMessages reads only indexed message records, not tool output or screenshots
// elsewhere in the rollout. Pages are chronological within a newest-first page order.
func (s *Service) SessionMessages(id, query string, page int) (Detail, error) {
	v, err := s.session(id)
	if err != nil {
		return Detail{}, err
	}
	out := Detail{Session: v, Messages: []Message{}, Page: max(0, page)}
	if v.Missing {
		return out, nil
	}
	if v.ParserVersion != parserVersion {
		return out, errors.New("此会话尚未建立消息索引，请刷新索引后查看")
	}
	info, err := os.Stat(v.Path)
	if err != nil {
		return out, err
	}
	if info.Size() != v.Size || info.ModTime().UnixNano() != v.Mtime {
		return out, errors.New("日志已变化，请刷新索引后查看最新消息")
	}
	rows, err := s.db.Query("SELECT offset,length,role,time FROM messages WHERE session_id=? ORDER BY seq DESC", id)
	if err != nil {
		return out, err
	}
	refs := []indexedMessage{}
	for rows.Next() {
		var m indexedMessage
		if err = rows.Scan(&m.Offset, &m.Length, &m.Role, &m.Time); err != nil {
			rows.Close()
			return out, err
		}
		refs = append(refs, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	f, err := os.Open(v.Path)
	if err != nil {
		return out, err
	}
	defer f.Close()
	query = strings.ToLower(strings.TrimSpace(query))
	skip := out.Page * 50
	if query == "" {
		out.Total = len(refs)
		start := min(skip, len(refs))
		refs = refs[start:min(start+50, len(refs))]
		skip = 0
	}
	for _, m := range refs {
		if m.Length <= 0 || m.Length > 32*1024*1024 {
			return out, errors.New("消息索引长度无效，请重建索引")
		}
		data := make([]byte, m.Length)
		if _, err = f.ReadAt(data, m.Offset); err != nil {
			return out, err
		}
		var r record
		var p payload
		if err = json.Unmarshal(data, &r); err != nil {
			return out, err
		}
		if err = json.Unmarshal(r.Payload, &p); err != nil {
			return out, err
		}
		text := messageText(p)
		if query != "" {
			if !strings.Contains(strings.ToLower(text), query) {
				continue
			}
			out.Total++
			if out.Total <= skip || len(out.Messages) >= 50 {
				continue
			}
		}
		if len([]rune(text)) > 24000 {
			out.Truncated = true
		}
		out.Messages = append(out.Messages, Message{m.Role, shortText(text, 24000), m.Time})
	}
	for i, j := 0, len(out.Messages)-1; i < j; i, j = i+1, j-1 {
		out.Messages[i], out.Messages[j] = out.Messages[j], out.Messages[i]
	}
	return out, nil
}
