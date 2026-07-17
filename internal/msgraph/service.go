package msgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"dispatch/internal/domain"
)

const (
	inlineThresholdBytes = 3 * 1024 * 1024
	chunkSize            = 4 * 327_680 // ~1.25 MB
)

// Service sends emails via Microsoft Graph API.
type Service struct {
	client      *Client
	rateLimiter *RateLimiter
	baseURL     string // empty → package-level production URL
}

func NewService(client *Client, rateLimiter *RateLimiter) *Service {
	return &Service{client: client, rateLimiter: rateLimiter, baseURL: baseURL}
}

func (s *Service) SendEmail(ctx context.Context, req domain.MailRequestDO) error {
	if err := s.rateLimiter.Wait(ctx, req.Sender); err != nil {
		return fmt.Errorf("rate limiter: %w", err)
	}

	totalSize := 0
	for _, a := range req.Attachments {
		totalSize += len(a.Content)
	}

	if totalSize < inlineThresholdBytes {
		return s.sendInline(ctx, req)
	}
	return s.sendViaUploadSession(ctx, req)
}

func (s *Service) sendInline(ctx context.Context, req domain.MailRequestDO) error {
	payload := buildGraphEmail(req, true)
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal sendMail: %w", err)
	}

	sendURL := fmt.Sprintf("%s/users/%s/sendMail", s.baseURL, req.Sender)
	_, _, err = s.client.doWithRetry(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, sendURL, bytes.NewReader(data))
	})
	return err
}

func (s *Service) sendViaUploadSession(ctx context.Context, req domain.MailRequestDO) error {
	draftID, err := s.createDraft(ctx, req)
	if err != nil {
		return err
	}

	cleanup := func() {
		delURL := fmt.Sprintf("%s/users/%s/messages/%s", s.baseURL, req.Sender, draftID)
		r, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, delURL, nil)
		_, _, _ = s.client.do(context.Background(), r)
	}

	for _, att := range req.Attachments {
		var attErr error
		if len(att.Content) < inlineThresholdBytes {
			attErr = s.addSmallAttachment(ctx, req.Sender, draftID, att)
		} else {
			attErr = s.uploadLargeAttachment(ctx, req.Sender, draftID, att)
		}
		if attErr != nil {
			cleanup()
			return attErr
		}
	}

	sendURL := fmt.Sprintf("%s/users/%s/messages/%s/send", s.baseURL, req.Sender, draftID)
	_, _, err = s.client.doWithRetry(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, sendURL, http.NoBody)
	})
	if err != nil {
		cleanup()
	}
	return err
}

func (s *Service) createDraft(ctx context.Context, req domain.MailRequestDO) (string, error) {
	payload := buildGraphEmail(req, false)
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal draft: %w", err)
	}

	draftURL := fmt.Sprintf("%s/users/%s/messages", s.baseURL, req.Sender)
	body, _, err := s.client.doWithRetry(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, draftURL, bytes.NewReader(data))
	})
	if err != nil {
		return "", err
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse draft response: %w", err)
	}
	return resp.ID, nil
}

func (s *Service) addSmallAttachment(ctx context.Context, sender, draftID string, att domain.AttachmentDO) error {
	payload := map[string]any{
		"@odata.type":  "#microsoft.graph.fileAttachment",
		"name":         att.Name,
		"contentType":  att.ContentType,
		"contentBytes": att.Content, // json.Marshal encodes []byte as base64
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal small attachment: %w", err)
	}

	attURL := fmt.Sprintf("%s/users/%s/messages/%s/attachments", s.baseURL, sender, draftID)
	_, _, err = s.client.doWithRetry(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, attURL, bytes.NewReader(data))
	})
	return err
}

func (s *Service) uploadLargeAttachment(ctx context.Context, sender, draftID string, att domain.AttachmentDO) error {
	sessionPayload := map[string]any{
		"AttachmentItem": map[string]any{
			"attachmentType": "file",
			"name":           att.Name,
			"size":           len(att.Content),
		},
	}
	sessionData, err := json.Marshal(sessionPayload)
	if err != nil {
		return fmt.Errorf("marshal upload session: %w", err)
	}

	sessionURL := fmt.Sprintf("%s/users/%s/messages/%s/attachments/createUploadSession", s.baseURL, sender, draftID)
	body, _, err := s.client.doWithRetry(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, sessionURL, bytes.NewReader(sessionData))
	})
	if err != nil {
		return err
	}

	var session struct {
		UploadURL string `json:"uploadUrl"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		return fmt.Errorf("parse upload session: %w", err)
	}

	return s.uploadChunks(ctx, session.UploadURL, att.Content)
}

func (s *Service) uploadChunks(ctx context.Context, uploadURL string, content []byte) error {
	total := len(content)
	for start := 0; start < total; start += chunkSize {
		end := start + chunkSize
		if end > total {
			end = total
		}
		chunk := content[start:end]
		rangeHeader := fmt.Sprintf("bytes %d-%d/%d", start, end-1, total)

		req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(chunk))
		if err != nil {
			return fmt.Errorf("build chunk request: %w", err)
		}
		req.Header.Set("Content-Range", rangeHeader)
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(chunk)))

		resp, err := s.client.http.Do(req)
		if err != nil {
			return &GraphTransientError{Cause: err}
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= 400 {
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				return &GraphTransientError{StatusCode: resp.StatusCode}
			}
			return &GraphPermanentError{StatusCode: resp.StatusCode, Body: string(body)}
		}
	}
	return nil
}

type graphEmail struct {
	Message         graphMessage `json:"message"`
	SaveToSentItems bool         `json:"saveToSentItems"`
}

type graphMessage struct {
	Subject                string               `json:"subject,omitempty"`
	Body                   graphBody            `json:"body"`
	ToRecipients           []graphRecipient     `json:"toRecipients"`
	CcRecipients           []graphRecipient     `json:"ccRecipients,omitempty"`
	BccRecipients          []graphRecipient     `json:"bccRecipients,omitempty"`
	Attachments            []graphAttach        `json:"attachments,omitempty"`
	InternetMessageHeaders []graphMessageHeader `json:"internetMessageHeaders,omitempty"`
}

type graphBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type graphRecipient struct {
	EmailAddress struct {
		Address string `json:"address"`
	} `json:"emailAddress"`
}

type graphAttach struct {
	ODataType    string `json:"@odata.type"`
	Name         string `json:"name"`
	ContentType  string `json:"contentType"`
	ContentBytes []byte `json:"contentBytes"`
}

type graphMessageHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func buildGraphEmail(req domain.MailRequestDO, includeAttachments bool) graphEmail {
	body := graphBody{ContentType: "Text", Content: req.BodyContent}
	if req.HtmlBodyContent != "" {
		body = graphBody{ContentType: "HTML", Content: req.HtmlBodyContent}
	}

	toGraphRecipients := func(addrs []string) []graphRecipient {
		result := make([]graphRecipient, len(addrs))
		for i, a := range addrs {
			result[i].EmailAddress.Address = a
		}
		return result
	}

	msg := graphMessage{
		Subject:       req.Subject,
		Body:          body,
		ToRecipients:  toGraphRecipients(req.Recipients),
		CcRecipients:  toGraphRecipients(req.CcRecipients),
		BccRecipients: toGraphRecipients(req.BccRecipients),
		InternetMessageHeaders: []graphMessageHeader{
			{Name: "X-Dispatch-TraceId", Value: req.TraceID},
		},
	}

	if includeAttachments {
		for _, a := range req.Attachments {
			msg.Attachments = append(msg.Attachments, graphAttach{
				ODataType:    "#microsoft.graph.fileAttachment",
				Name:         a.Name,
				ContentType:  a.ContentType,
				ContentBytes: a.Content,
			})
		}
	}

	return graphEmail{Message: msg, SaveToSentItems: false}
}
