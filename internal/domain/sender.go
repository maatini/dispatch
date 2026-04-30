package domain

type Sender struct {
	AppTag         string `json:"appTag"`
	Email          string `json:"email"`
	Test           bool   `json:"test"`
	DailyQuota     int    `json:"dailyQuota"`     // -1 or 0 = unlimited
	AllowedDomains string `json:"allowedDomains"` // comma-separated, empty = all allowed
}
