package atlas

import "time"

type Usage struct {
	Input     int64 `json:"input"`
	Cached    int64 `json:"cached"`
	Output    int64 `json:"output"`
	Reasoning int64 `json:"reasoning"`
	Total     int64 `json:"total"`
}
type Session struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Project  string `json:"project"`
	Title    string `json:"title"`
	Model    string `json:"model"`
	Created  string `json:"created"`
	Updated  string `json:"updated"`
	Parent   string `json:"parent"`
	Size     int64  `json:"size"`
	Mtime    int64  `json:"mtime"`
	Archived bool   `json:"archived"`
	Missing  bool   `json:"missing"`
	Warning  string `json:"warning"`
	Usage    Usage  `json:"usage"`
}
type Project struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Sessions int    `json:"sessions"`
	Bytes    int64  `json:"bytes"`
	Usage    Usage  `json:"usage"`
}
type Day struct {
	Date  string `json:"date"`
	Total int64  `json:"total"`
}
type Storage struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Error string `json:"error"`
}
type Snapshot struct {
	Projects  []Project `json:"projects"`
	Usage     Usage     `json:"usage"`
	Sessions  int       `json:"sessions"`
	Bytes     int64     `json:"bytes"`
	Days      []Day     `json:"days"`
	Storage   []Storage `json:"storage"`
	Home      string    `json:"home"`
	Database  string    `json:"database"`
	TempRoots []string  `json:"tempRoots"`
	LastScan  string    `json:"lastScan"`
}
type ScanStatus struct {
	Running  bool     `json:"running"`
	Phase    string   `json:"phase"`
	Done     int      `json:"done"`
	Total    int      `json:"total"`
	Errors   []string `json:"errors"`
	Finished string   `json:"finished"`
}
type SessionQuery struct {
	Search  string `json:"search"`
	Project string `json:"project"`
	State   string `json:"state"`
	Sort    string `json:"sort"`
	Page    int    `json:"page"`
}
type SessionPage struct {
	Items []Session `json:"items"`
	Total int       `json:"total"`
}
type Message struct {
	Role string `json:"role"`
	Text string `json:"text"`
	Time string `json:"time"`
}
type Detail struct {
	Session   Session   `json:"session"`
	Messages  []Message `json:"messages"`
	Truncated bool      `json:"truncated"`
}
type Evidence struct {
	SessionID string `json:"sessionId"`
	Project   string `json:"project"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
}
type TempFile struct {
	Path     string     `json:"path"`
	Name     string     `json:"name"`
	Bytes    int64      `json:"bytes"`
	Modified string     `json:"modified"`
	Mtime    int64      `json:"mtime"`
	IsDir    bool       `json:"isDir"`
	Link     bool       `json:"link"`
	Evidence []Evidence `json:"evidence"`
	Error    string     `json:"error"`
}
type CleanItem struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Bytes   int64  `json:"bytes"`
	Blocked string `json:"blocked"`
	Mtime   int64  `json:"mtime"`
}
type CleanPlan struct {
	Token   string      `json:"token"`
	Kind    string      `json:"kind"`
	Items   []CleanItem `json:"items"`
	Bytes   int64       `json:"bytes"`
	Expires time.Time   `json:"expires"`
}
type CleanResult struct {
	ID      string `json:"id"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}
