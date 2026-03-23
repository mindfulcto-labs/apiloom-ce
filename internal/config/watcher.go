package config

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	defaultDebounceInterval = 100 * time.Millisecond
	maxBackoff              = 30 * time.Second
	listenChannel           = "config_changes"
)

// WatcherOption configures optional Watcher behavior.
type WatcherOption func(*Watcher)

// WithLogger sets the logger for the watcher.
func WithLogger(logger *slog.Logger) WatcherOption {
	return func(w *Watcher) {
		w.logger = logger
	}
}

// WithDebounceInterval sets the debounce window for coalescing NOTIFY events.
func WithDebounceInterval(d time.Duration) WatcherOption {
	return func(w *Watcher) {
		w.debounceInterval = d
	}
}

// Watcher listens for Postgres NOTIFY events on a dedicated connection and
// triggers config reload via the Loader. It reconnects with exponential
// backoff on connection loss.
type Watcher struct {
	dsn              string
	loader           *Loader
	logger           *slog.Logger
	debounceInterval time.Duration
}

// NewWatcher creates a Watcher that listens for config_changed notifications
// on a dedicated pgx.Conn (NOT from the pool).
func NewWatcher(dsn string, loader *Loader, opts ...WatcherOption) *Watcher {
	w := &Watcher{
		dsn:              dsn,
		loader:           loader,
		logger:           slog.Default(),
		debounceInterval: defaultDebounceInterval,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Run starts the LISTEN/NOTIFY loop. It blocks until ctx is cancelled
// (returning nil) or an unrecoverable error occurs. On connection errors,
// it reconnects with exponential backoff.
func (w *Watcher) Run(ctx context.Context) error {
	attempt := 0

	for {
		if ctx.Err() != nil {
			return nil
		}

		err := w.listenLoop(ctx)
		if err == nil {
			// Clean shutdown via context cancellation.
			return nil
		}

		if ctx.Err() != nil {
			return nil
		}

		// Connection error -- reconnect with backoff.
		delay := backoff(attempt)
		w.logger.Warn("watcher connection lost, reconnecting",
			"attempt", attempt,
			"delay", delay,
			"error", err,
		)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}

		attempt++
	}
}

// listenLoop opens a dedicated connection, issues LISTEN, and processes
// notifications until the connection drops or context is cancelled.
func (w *Watcher) listenLoop(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, w.dsn)
	if err != nil {
		return fmt.Errorf("connecting for LISTEN: %w", err)
	}
	defer func() {
		// Use background context for close since parent ctx may be cancelled.
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn.Close(closeCtx)
	}()

	_, err = conn.Exec(ctx, "LISTEN "+listenChannel)
	if err != nil {
		return fmt.Errorf("issuing LISTEN: %w", err)
	}

	w.logger.Info("watcher listening for notifications", "channel", listenChannel)

	// Track the most recent org ID from notifications for reload context.
	var lastOrgID uuid.UUID
	var lastOrgMu sync.Mutex

	// Set up debounced reload.
	d := newDebouncer(w.debounceInterval, func() {
		reloadCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Inject org context from the notification or from the parent context.
		lastOrgMu.Lock()
		orgID := lastOrgID
		lastOrgMu.Unlock()

		if orgID == uuid.Nil {
			// Fall back to org from the parent context.
			if id, ctxErr := OrgFromContext(ctx); ctxErr == nil {
				orgID = id
			}
		}

		if orgID != uuid.Nil {
			reloadCtx = ContextWithOrg(reloadCtx, orgID)
		}

		if err := w.loader.Reload(reloadCtx); err != nil {
			w.logger.Error("reload after notification failed", "error", err)
		}
	})

	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("waiting for notification: %w", err)
		}

		w.logger.Debug("received notification",
			"channel", notification.Channel,
			"payload", notification.Payload,
		)

		// Parse organization_id from notification payload.
		if orgID := parseOrgIDFromPayload(notification.Payload); orgID != uuid.Nil {
			lastOrgMu.Lock()
			lastOrgID = orgID
			lastOrgMu.Unlock()
		}

		d.trigger()
	}
}

// debouncer coalesces rapid triggers into a single function call after
// the specified interval.
type debouncer struct {
	mu       sync.Mutex
	interval time.Duration
	timer    *time.Timer
	fn       func()
}

// newDebouncer creates a debouncer that calls fn after interval of inactivity.
func newDebouncer(interval time.Duration, fn func()) *debouncer {
	return &debouncer{
		interval: interval,
		fn:       fn,
	}
}

// trigger resets the debounce timer. If no further triggers arrive within
// the interval, fn is called.
func (d *debouncer) trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.interval, d.fn)
}

// notifyPayload represents the JSON payload from config_changes NOTIFY.
type notifyPayload struct {
	Table          string `json:"table"`
	Action         string `json:"action"`
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
}

// parseOrgIDFromPayload extracts the organization_id from a NOTIFY JSON payload.
// Returns uuid.Nil if parsing fails or the field is missing.
func parseOrgIDFromPayload(payload string) uuid.UUID {
	var p notifyPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return uuid.Nil
	}
	id, err := uuid.Parse(p.OrganizationID)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// backoff returns an exponential backoff duration with jitter for the given
// attempt number. Base is 1s, max is 30s. Uses crypto/rand for jitter.
func backoff(attempt int) time.Duration {
	base := time.Duration(1) << uint(attempt) * time.Second
	if base > maxBackoff {
		base = maxBackoff
	}

	// Add jitter: 0 to 1 second using crypto/rand.
	jitterMax := int64(time.Second)
	n, err := rand.Int(rand.Reader, big.NewInt(jitterMax))
	if err != nil {
		// Fallback: no jitter on rand failure (should never happen).
		return base
	}

	return base + time.Duration(n.Int64())
}
