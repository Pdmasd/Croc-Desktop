package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/schollz/croc/v11/src/codephrase"
	"github.com/schollz/croc/v11/src/croc"
)

type TransferState string

const (
	StateInitializing   TransferState = "Initializing"
	StateConnecting     TransferState = "Connecting"
	StateAuthenticating TransferState = "Authenticating"
	StateTransferring   TransferState = "Transferring"
	StateCompleted      TransferState = "Completed"
	StateFailed         TransferState = "Failed"
	StateCancelled      TransferState = "Cancelled"
)

type TransferEvent struct {
	ID             string        `json:"id"`
	Direction      string        `json:"direction"`
	State          TransferState `json:"state"`
	Transferred    int64         `json:"transferred"`
	Total          int64         `json:"total"`
	BytesPerSecond int64         `json:"bytesPerSecond"`
	Error          string        `json:"error,omitempty"`
	UpdatedAt      time.Time     `json:"updatedAt"`
}

type TransferStart struct {
	Transfer  TransferEvent `json:"transfer"`
	ShareCode string        `json:"shareCode,omitempty"`
}

type activeTransfer struct {
	event     TransferEvent
	cancel    context.CancelFunc
	lastBytes int64
	lastAt    time.Time
}

type TransferManager struct {
	mu           sync.Mutex
	publishMu    sync.Mutex
	active       *activeTransfer
	last         TransferEvent
	emit         func(TransferEvent)
	now          func() time.Time
	generateCode func() (string, error)
}

func NewTransferManager(emit func(TransferEvent)) *TransferManager {
	return &TransferManager{emit: emit, now: time.Now, generateCode: codephrase.Generate}
}

func (m *TransferManager) StartSend(paths []string, options croc.Options) (TransferStart, error) {
	if len(paths) == 0 {
		return TransferStart{}, errors.New("select at least one file")
	}
	files, emptyFolders, folders, err := croc.GetFilesInfoWithExactExclusionsContext(context.Background(), paths, false, false, nil, nil)
	if err != nil {
		return TransferStart{}, fmt.Errorf("prepare files: %w", err)
	}
	if len(files) == 0 {
		return TransferStart{}, errors.New("no sendable files were selected")
	}
	secret, err := m.generateCode()
	if err != nil {
		return TransferStart{}, fmt.Errorf("generate share code: %w", err)
	}
	options.SharedSecret, options.IsSender = secret, true
	return m.start("send", options, secret, func(ctx context.Context, opts croc.Options) error {
		client, err := croc.NewCtx(ctx, opts)
		if err != nil {
			return err
		}
		return client.Send(files, emptyFolders, folders)
	})
}

func (m *TransferManager) StartReceive(secret string, options croc.Options) (TransferStart, error) {
	if strings.TrimSpace(secret) == "" {
		return TransferStart{}, errors.New("enter a Croc share code")
	}
	options.SharedSecret, options.IsSender = secret, false
	return m.start("receive", options, "", func(ctx context.Context, opts croc.Options) error {
		client, err := croc.NewCtx(ctx, opts)
		if err != nil {
			return err
		}
		return client.Receive()
	})
}

func (m *TransferManager) start(direction string, options croc.Options, secret string, run func(context.Context, croc.Options) error) (TransferStart, error) {
	m.publishMu.Lock()
	m.mu.Lock()
	if m.active != nil && !terminalState(m.active.event.State) {
		m.mu.Unlock()
		m.publishMu.Unlock()
		return TransferStart{}, errors.New("a transfer is already active")
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := m.now()
	id := fmt.Sprintf("%x", now.UnixNano())
	active := &activeTransfer{event: TransferEvent{ID: id, Direction: direction, State: StateInitializing, UpdatedAt: now}, cancel: cancel, lastAt: now}
	initial := active.event
	m.active, m.last = active, active.event
	m.mu.Unlock()
	m.publish(initial)
	m.publishMu.Unlock()
	options.OnPhase = func(phase string) { m.setState(id, TransferState(phase), "") }
	options.OnProgress = func(done, total int64) { m.setProgress(id, done, total) }
	go func() {
		err := run(ctx, options)
		m.finish(id, err)
	}()
	return TransferStart{Transfer: initial, ShareCode: secret}, nil
}

func (m *TransferManager) setState(id string, state TransferState, message string) {
	m.publishMu.Lock()
	m.mu.Lock()
	if m.active == nil || m.active.event.ID != id || terminalState(m.active.event.State) {
		m.mu.Unlock()
		m.publishMu.Unlock()
		return
	}
	m.active.event.State = state
	m.active.event.Error = message
	m.active.event.UpdatedAt = m.now()
	event := m.active.event
	m.last = event
	m.mu.Unlock()
	m.publish(event)
	m.publishMu.Unlock()
}

func (m *TransferManager) setProgress(id string, done, total int64) {
	m.publishMu.Lock()
	m.mu.Lock()
	if m.active == nil || m.active.event.ID != id || terminalState(m.active.event.State) {
		m.mu.Unlock()
		m.publishMu.Unlock()
		return
	}
	now := m.now()
	delta := done - m.active.lastBytes
	elapsed := now.Sub(m.active.lastAt).Seconds()
	if delta > 0 && elapsed > 0 {
		m.active.event.BytesPerSecond = int64(float64(delta) / elapsed)
	}
	m.active.lastBytes, m.active.lastAt = done, now
	m.active.event.Transferred, m.active.event.Total = done, total
	m.active.event.UpdatedAt = now
	event := m.active.event
	m.last = event
	m.mu.Unlock()
	m.publish(event)
	m.publishMu.Unlock()
}

func (m *TransferManager) finish(id string, err error) {
	m.publishMu.Lock()
	m.mu.Lock()
	if m.active == nil || m.active.event.ID != id || terminalState(m.active.event.State) {
		m.mu.Unlock()
		m.publishMu.Unlock()
		return
	}
	switch {
	case errors.Is(err, context.Canceled):
		m.active.event.State = StateCancelled
	case err != nil:
		m.active.event.State, m.active.event.Error = StateFailed, err.Error()
	default:
		m.active.event.State = StateCompleted
	}
	m.active.event.UpdatedAt = m.now()
	m.active.cancel = nil
	event := m.active.event
	m.last = event
	m.mu.Unlock()
	m.publish(event)
	m.publishMu.Unlock()
}

func (m *TransferManager) Cancel() error {
	m.mu.Lock()
	if m.active == nil || terminalState(m.active.event.State) {
		m.mu.Unlock()
		return errors.New("no active transfer")
	}
	cancel := m.active.cancel
	id := m.active.event.ID
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.setState(id, StateCancelled, "")
	return nil
}

func (m *TransferManager) Snapshot() TransferEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil {
		return m.active.event
	}
	return m.last
}

func (m *TransferManager) publish(event TransferEvent) {
	if m.emit != nil {
		m.emit(event)
	}
}
func terminalState(state TransferState) bool {
	return state == StateCompleted || state == StateFailed || state == StateCancelled
}
