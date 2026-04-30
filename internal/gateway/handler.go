package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"codymail-go/internal/config"
	"codymail-go/internal/domain"
	"codymail-go/internal/hash"
	"codymail-go/internal/pii"
)

type senderLookup interface {
	Get(appTag string) (domain.Sender, error)
}

type quotaChecker interface {
	Check(appTag string, limit, requested int) error
	CurrentUsage(appTag string) (int, error)
}

type spamChecker interface {
	Check(hashVal string) error
}

type natsPublisher interface {
	Publish(ctx context.Context, msg *domain.MailRequestDO) error
}

type attachmentUploader interface {
	Upload(ctx context.Context, traceID string, attachments []domain.AttachmentDO) ([]domain.AttachmentDO, error)
}

// Handler is the HTTP handler for the mail gateway.
type Handler struct {
	cfg      config.Config
	senders  senderLookup
	quota    quotaChecker
	spam     spamChecker
	nats     natsPublisher
	attStore attachmentUploader
}

func NewHandler(cfg config.Config, senders senderLookup, quota quotaChecker, spam spamChecker, nats natsPublisher, attStore attachmentUploader) *Handler {
	return &Handler{cfg: cfg, senders: senders, quota: quota, spam: spam, nats: nats, attStore: attStore}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Post("/codymail/api/v1/mail/send", h.handleSend)
	r.Get("/health", h.handleHealth)
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/health/ready", h.handleHealth)
	return r
}

func (h *Handler) handleSend(w http.ResponseWriter, r *http.Request) {
	traceID := uuid.New().String()
	ctx := r.Context()

	var req domain.MailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, domain.ErrJSONParseError, "invalid JSON body", traceID)
		return
	}

	// Stage 1: bean validation
	if err := validateRequest(&req, h.cfg.MaxBodySize, h.cfg.MimeWhitelist, h.cfg.MaxTotalAttachmentMB); err != nil {
		h.writeValidationError(w, err, traceID)
		return
	}

	// Stage 2: sender lookup
	sender, err := h.senders.Get(req.AppTag)
	if err != nil {
		h.writeValidationError(w, err, traceID)
		return
	}

	// Stage 3: domain whitelist
	if err := checkDomains(sender, &req); err != nil {
		allRecips := append(append(req.Recipients, req.CcRecipients...), req.BccRecipients...)
		for _, addr := range allRecips {
			slog.WarnContext(ctx, "domain not whitelisted",
				slog.String("traceId", traceID),
				slog.String("recipient", pii.MaskEmail(addr)),
				slog.String("appTag", req.AppTag),
			)
		}
		h.writeValidationError(w, err, traceID)
		return
	}

	// Stage 4: quota
	recipientCount := len(req.Recipients) + len(req.CcRecipients) + len(req.BccRecipients)
	if err := h.quota.Check(req.AppTag, sender.DailyQuota, recipientCount); err != nil {
		h.writeQuotaError(w, err, sender.DailyQuota, traceID)
		return
	}

	// Stage 5: spam
	spamHash := hash.SpamHash(req.AppTag, req.Subject, req.Recipients, len(req.BodyContent), len(req.HtmlBodyContent))
	if err := h.spam.Check(spamHash); err != nil {
		h.writeValidationError(w, err, traceID)
		return
	}

	// Decode attachments and upload to Object Store
	attachments, err := decodeAttachments(req.Attachments)
	if err != nil {
		h.writeValidationError(w, err, traceID)
		return
	}
	if len(attachments) > 0 {
		attachments, err = h.attStore.Upload(ctx, traceID, attachments)
		if err != nil {
			slog.ErrorContext(ctx, "attachment upload failed",
				slog.String("traceId", traceID),
				slog.String("error", err.Error()),
			)
			writeError(w, http.StatusServiceUnavailable, domain.ErrNatsUnavailable,
				"Attachment storage unavailable. Bitte erneut versuchen.", traceID)
			return
		}
	}

	msg := &domain.MailRequestDO{
		TraceID:         traceID,
		AppTag:          req.AppTag,
		Sender:          sender.Email,
		Recipients:      req.Recipients,
		CcRecipients:    req.CcRecipients,
		BccRecipients:   req.BccRecipients,
		Subject:         req.Subject,
		BodyContent:     req.BodyContent,
		HtmlBodyContent: req.HtmlBodyContent,
		Attachments:     attachments,
		TraceContext:    req.TraceContext,
		Test:            sender.Test,
	}

	if err := h.nats.Publish(ctx, msg); err != nil {
		slog.ErrorContext(ctx, "NATS publish failed",
			slog.String("traceId", traceID),
			slog.String("appTag", req.AppTag),
			slog.String("error", err.Error()),
		)
		writeError(w, http.StatusServiceUnavailable, domain.ErrNatsUnavailable,
			"Broker nicht erreichbar. Bitte erneut versuchen.", traceID)
		return
	}

	slog.InfoContext(ctx, "mail dispatched to NATS",
		slog.String("traceId", traceID),
		slog.String("appTag", req.AppTag),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "SUCCESS", "traceId": traceID})
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "UP",
		"checks": []map[string]string{
			{"name": "nats", "status": "UP"},
		},
	})
}

func (h *Handler) writeValidationError(w http.ResponseWriter, err error, traceID string) {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		status := http.StatusBadRequest
		writeError(w, status, ve.Code, ve.Message, traceID)
		return
	}
	writeError(w, http.StatusInternalServerError, "", err.Error(), traceID)
}

func (h *Handler) writeQuotaError(w http.ResponseWriter, err error, limit int, traceID string) {
	var qe *domain.QuotaError
	if errors.As(err, &qe) {
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", max(0, limit-qe.Current)))
		writeError(w, http.StatusTooManyRequests, domain.ErrQuotaExceeded,
			"Daily recipient quota exceeded", traceID)
		return
	}
	var se *domain.QuotaStateError
	if errors.As(err, &se) {
		writeError(w, http.StatusServiceUnavailable, domain.ErrNatsUnavailable,
			"Quota service unavailable", traceID)
		return
	}
	writeError(w, http.StatusInternalServerError, "", err.Error(), traceID)
}

func writeError(w http.ResponseWriter, status int, code domain.ErrorCode, msg, traceID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(domain.ApiError{
		Status:  status,
		Code:    code,
		Message: msg,
		TraceID: traceID,
	})
}
