package adapters

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

// ---------------------------------------------------------------- fakes

// fakeService implements only Service, so it must advertise health and nothing else.
type fakeService struct {
	id, name string

	mu      sync.Mutex
	health  domain.Health
	err     error
	delay   time.Duration
	calls   int
	lastCtx context.Context
}

func (f *fakeService) ID() string   { return f.id }
func (f *fakeService) Name() string { return f.name }

func (f *fakeService) Health(ctx context.Context) (domain.Health, error) {
	f.mu.Lock()
	delay, err, health := f.delay, f.err, f.health
	f.calls++
	f.lastCtx = ctx
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return domain.Health{}, ctx.Err()
		}
	}
	if err != nil {
		return domain.Health{}, err
	}
	if health.Status == "" {
		health.Status = domain.StatusHealthy
		health.Reachable = true
	}
	return health, nil
}

func (f *fakeService) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeService) set(health domain.Health, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.health, f.err = health, err
}

// fakeControllable additionally implements Controllable.
type fakeControllable struct {
	*fakeService
	invoked []string
}

func (f *fakeControllable) Actions(_ context.Context) []domain.Action {
	return []domain.Action{{ID: "restart", Label: "Restart", Risk: domain.RiskDisruptive}}
}

func (f *fakeControllable) Invoke(_ context.Context, actionID string) (*host.Job, error) {
	f.invoked = append(f.invoked, actionID)
	if actionID != "restart" {
		return nil, errors.New("unknown action")
	}
	return nil, nil
}

// ---------------------------------------------------------------- capabilities

// TestCapabilitiesOfDiscoversByTypeAssertion is the mechanism ADR-0005 rests on: an
// adapter declares what it can do by implementing an interface, not by returning a list
// that could disagree with its behaviour.
func TestCapabilitiesOfDiscoversByTypeAssertion(t *testing.T) {
	plain := &fakeService{id: "plain"}
	controllable := &fakeControllable{fakeService: &fakeService{id: "controllable"}}

	plainCaps := CapabilitiesOf(plain)
	if len(plainCaps) != 1 || plainCaps[0].ID != domain.CapabilityHealth.ID {
		t.Errorf("plain service capabilities = %v, want [health] only", plainCaps)
	}

	controlCaps := CapabilitiesOf(controllable)
	if len(controlCaps) != 2 {
		t.Fatalf("controllable capabilities = %v, want health and control", controlCaps)
	}
	if !HasCapability(controllable, domain.CapabilityControl.ID) {
		t.Error("controllable service does not advertise control")
	}
	if HasCapability(plain, domain.CapabilityControl.ID) {
		t.Error("plain service advertises control it cannot perform")
	}
}

// TestHealthCapabilityIsUnconditional: Health is part of Service, so no adapter can exist
// without it. Discovering it by assertion would imply it were optional.
func TestHealthCapabilityIsUnconditional(t *testing.T) {
	for _, s := range []Service{
		&fakeService{id: "a"},
		&fakeControllable{fakeService: &fakeService{id: "b"}},
	} {
		if !HasCapability(s, domain.CapabilityHealth.ID) {
			t.Errorf("%s does not advertise health", s.ID())
		}
		if CapabilitiesOf(s)[0].ID != domain.CapabilityHealth.ID {
			t.Error("health is not listed first")
		}
	}
}

// TestCapabilitiesCarryLabels: a client that predates a capability renders its label
// rather than an empty box (ADR-0007), so a capability without one is a client bug
// waiting to happen.
func TestCapabilitiesCarryLabels(t *testing.T) {
	caps := CapabilitiesOf(&fakeControllable{fakeService: &fakeService{id: "x"}})
	for _, c := range caps {
		if c.ID == "" || c.Label == "" {
			t.Errorf("capability %+v is missing an id or label", c)
		}
	}
}

// TestUnimplementedCapabilitiesAreNotAdvertised guards the probe table against a copy
// -paste error that would make every adapter claim now_playing.
func TestUnimplementedCapabilitiesAreNotAdvertised(t *testing.T) {
	controllable := &fakeControllable{fakeService: &fakeService{id: "x"}}
	for _, id := range []string{domain.CapabilityNowPlaying.ID, domain.CapabilityTransfers.ID} {
		if HasCapability(controllable, id) {
			t.Errorf("service advertises %q, which it does not implement", id)
		}
	}
}
