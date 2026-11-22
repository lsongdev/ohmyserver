package modem

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/modules/cellular/internal/at"
)

const pollInterval = 5 * time.Second

var ErrOffline = errors.New("No connection to the modem")

type request struct {
	ctx  context.Context
	run  func(context.Context, *Device) error
	done chan error
}

type Status struct {
	Port      string     `json:"port,omitempty"`
	State     string     `json:"state"`
	Error     string     `json:"error,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Reading
}

type Monitor struct {
	mu       sync.RWMutex
	status   Status
	cancel   context.CancelFunc
	done     chan struct{}
	requests chan request
}

func Start(port string) *Monitor {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Monitor{status: Status{Port: port, State: "connecting"}, cancel: cancel, done: make(chan struct{}), requests: make(chan request, 1)}
	go m.run(ctx, port)
	return m
}

func (m *Monitor) Close() {
	m.cancel()
	<-m.done
}

// Snapshot is immutable. Each poll publishes a new status value.
func (m *Monitor) Snapshot() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Monitor) publish(s Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = s
}

func (m *Monitor) ListSMS(ctx context.Context) (Inbox, error) {
	var result Inbox
	err := m.execute(ctx, func(ctx context.Context, device *Device) (err error) {
		result, err = device.ListSMS(ctx)
		return err
	})
	return result, err
}

func (m *Monitor) SendSMS(ctx context.Context, number, text string) (Submission, error) {
	if err := ValidateSMS(number, text); err != nil {
		return Submission{}, err
	}
	var result Submission
	err := m.execute(ctx, func(ctx context.Context, device *Device) (err error) {
		result, err = device.SendSMS(ctx, number, text)
		return err
	})
	return result, err
}

func (m *Monitor) execute(ctx context.Context, run func(context.Context, *Device) error) error {
	req := request{ctx: ctx, run: run, done: make(chan error, 1)}
	select {
	case m.requests <- req:
	case <-ctx.Done():
		return ctx.Err()
	case <-m.done:
		return ErrOffline
	default:
		return ErrBusy
	}
	// Once accepted, wait for the result. A disconnected HTTP client must not
	// cause a submitted SMS to be retried or its modem response discarded.
	select {
	case err := <-req.done:
		return err
	case <-m.done:
		select {
		case err := <-req.done:
			return err
		default:
			return ErrOffline
		}
	}
}

var ErrBusy = errors.New("Modem is busy; try again shortly")

func (m *Monitor) run(ctx context.Context, override string) {
	defer close(m.done)
	var session *at.Session
	closeSession := func() {
		if session != nil {
			session.Close()
			session = nil
		}
	}
	defer closeSession()
	device := NewDevice(nil)
	port := override
	timer := time.NewTimer(0)
	defer timer.Stop()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case req := <-m.requests:
			err := req.ctx.Err()
			if err == nil {
				if session == nil {
					err = ErrOffline
				} else {
					err = req.run(ctx, device)
					var failure *SMSFailure
					var input SMSInputError
					var command InvalidCommand
					if err != nil && !replyError(err) && !errors.As(err, &failure) && !errors.As(err, &input) && !errors.As(err, &command) {
						closeSession()
						m.publish(Status{Port: port, State: "disconnected", Error: "Serial transaction interrupted"})
					}
				}
			}
			req.done <- err
		case <-timer.C:
			var err error
			if session == nil {
				port = override
				if port == "" {
					port, err = Discover()
				}
				if err == nil {
					session, err = at.Open(port)
				}
			}
			result := Status{Port: port, State: "connected"}
			if err == nil {
				device.session = session
				result.Reading, err = device.Read(ctx)
			}
			switch {
			case err == nil:
				now := time.Now()
				result.UpdatedAt = &now
			case errors.Is(err, ErrNotFound):
				result.State = "not_found"
			default:
				result.State, result.Error = "disconnected", err.Error()
				closeSession()
			}
			if ctx.Err() == nil {
				m.publish(result)
				timer.Reset(pollInterval)
			}
		}
	}
}
