package msgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"dispatch/internal/bounce"
)

// BounceService fetches unread NDR messages and marks them as read via MS Graph.
type BounceService struct {
	client  *Client
	baseURL string // empty = package-level production URL
}

func NewBounceService(client *Client) *BounceService {
	return &BounceService{client: client}
}

func (s *BounceService) apiBase() string {
	if s.baseURL != "" {
		return s.baseURL
	}
	return baseURL
}

// GetUnreadMessages fetches unread messages from the bounce mailbox.
// Graph API: GET /users/{mailbox}/messages?$filter=isRead eq false&$select=id,subject,body
func (s *BounceService) GetUnreadMessages(ctx context.Context, mailbox string) ([]bounce.NDRMessage, error) {
	u := fmt.Sprintf("%s/users/%s/messages?$filter=isRead+eq+false&$select=id,subject,body,toRecipients,receivedDateTime", s.apiBase(), mailbox)
	body, _, err := s.client.doWithRetry(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	})
	if err != nil {
		return nil, fmt.Errorf("list unread messages: %w", err)
	}
	return parseNDRMessages(body)
}

// MarkAsRead marks a message as read.
// Graph API: PATCH /users/{mailbox}/messages/{messageId}  body: {"isRead":true}
func (s *BounceService) MarkAsRead(ctx context.Context, mailbox, messageID string) error {
	u := fmt.Sprintf("%s/users/%s/messages/%s", s.apiBase(), mailbox, messageID)
	_, _, err := s.client.doWithRetry(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPatch, u, bytes.NewReader([]byte(`{"isRead":true}`)))
	})
	if err != nil {
		return fmt.Errorf("mark as read: %w", err)
	}
	return nil
}

func parseNDRMessages(data []byte) ([]bounce.NDRMessage, error) {
	var resp struct {
		Value []struct {
			ID      string `json:"id"`
			Subject string `json:"subject"`
			Body    struct {
				Content string `json:"content"`
			} `json:"body"`
			ToRecipients []struct {
				EmailAddress struct {
					Address string `json:"address"`
				} `json:"emailAddress"`
			} `json:"toRecipients"`
			ReceivedDateTime string `json:"receivedDateTime"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse messages response: %w", err)
	}
	msgs := make([]bounce.NDRMessage, len(resp.Value))
	for i, m := range resp.Value {
		recipient := ""
		if len(m.ToRecipients) > 0 {
			recipient = m.ToRecipients[0].EmailAddress.Address
		}
		receivedAt, _ := time.Parse(time.RFC3339, m.ReceivedDateTime)
		msgs[i] = bounce.NDRMessage{
			ID:         m.ID,
			Subject:    m.Subject,
			Body:       m.Body.Content,
			Recipient:  recipient,
			ReceivedAt: receivedAt,
		}
	}
	return msgs, nil
}
