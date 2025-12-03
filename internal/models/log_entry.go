package models

type LogEntry struct {
	TimeLocal      string `json:"time_local"`
	RemoteAddr     string `json:"remote_addr"`
	RemoteUser     string `json:"remote_user"`
	Request        string `json:"request"`
	Status         string `json:"status"`
	BodyBytesSent  string `json:"body_bytes_sent"`
	HTTPReferer    string `json:"http_referer"`
	HTTPUserAgent  string `json:"http_user_agent"`
	XForwardedFor  string `json:"http_x_forwarded_for"`
	RequestMethod  string `json:"request_method"`
	RequestURI     string `json:"request_uri"`
	RequestTime    string `json:"request_time"`
}
