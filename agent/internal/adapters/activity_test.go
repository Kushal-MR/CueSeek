package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// activeService is a fake implementing both activity capabilities, so the poller's
// collection path can be exercised without a real adapter.
type activeService struct {
	fakeService

	playing    domain.NowPlaying
	playingErr error

	moving    domain.Transfers
	movingErr error
}

func (a *activeService) NowPlaying(context.Context) (domain.NowPlaying, error) {
	return a.playing, a.playingErr
}

func (a *activeService) Transfers(context.Context) (domain.Transfers, error) {
	return a.moving, a.movingErr
}

func newActivePoller(t *testing.T, svc *activeService) *Poller {
	t.Helper()
	svc.id = "svc"

	r := NewRegistry()
	if err := r.RegisterFactory("active", func(cfg config.Service, _ Deps) (Service, error) {
		return svc, nil
	}); err != nil {
		t.Fatalf("RegisterFactory: %v", err)
	}
	cfg := config.Config{Services: []config.Service{{ID: "svc", Type: "active"}}}
	if err := r.Build(cfg, Deps{}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return NewPoller(r, cfg)
}

// TestActivityIsCollectedOnThePollPath is ADR-0003's rule applied to the new capabilities:
// a client request must never cause an upstream call, so the payload has to be sitting in
// the cache before anyone asks for it.
func TestActivityIsCollectedOnThePollPath(t *testing.T) {
	svc := &activeService{
		playing: domain.NowPlaying{Sessions: 2, Transcoding: 1},
		moving:  domain.Transfers{Active: 3, Total: 9},
	}
	p := newActivePoller(t, svc)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p.Start(ctx)

	waitFor(t, "the first poll", func() bool {
		s, _ := p.Cache().Get("svc")
		return s.NowPlaying != nil && s.Transfers != nil
	})

	s, _ := p.Cache().Get("svc")
	if s.NowPlaying.Sessions != 2 || s.NowPlaying.Transcoding != 1 {
		t.Errorf("NowPlaying = %+v", s.NowPlaying)
	}
	if s.Transfers.Active != 3 || s.Transfers.Total != 9 {
		t.Errorf("Transfers = %+v", s.Transfers)
	}

	cancel()
	p.Wait()
}

// TestActivityFailureLeavesHealthAlone is the boundary that matters most.
//
// A media server that answers its health endpoint and refuses /Sessions is **up**.
// Reporting it as unhealthy would send an operator hunting an outage that is not
// happening, which is the same class of error as conflating "unreachable" with "degraded".
func TestActivityFailureLeavesHealthAlone(t *testing.T) {
	svc := &activeService{
		playingErr: errors.New("sessions refused"),
		movingErr:  errors.New("torrents refused"),
	}
	svc.set(domain.Health{Status: domain.StatusHealthy, Reachable: true}, nil)
	p := newActivePoller(t, svc)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p.Start(ctx)

	waitFor(t, "the first poll", func() bool {
		s, _ := p.Cache().Get("svc")
		return s.Health.Status == domain.StatusHealthy
	})

	s, _ := p.Cache().Get("svc")
	// Nil, not zero. "We could not ask" and "nothing is playing" are different facts, and
	// a zero value would let a client render "0 sessions" as though it were observed.
	if s.NowPlaying != nil {
		t.Errorf("NowPlaying = %+v, want nil after a failed read", s.NowPlaying)
	}
	if s.Transfers != nil {
		t.Errorf("Transfers = %+v, want nil after a failed read", s.Transfers)
	}

	cancel()
	p.Wait()
}

// TestActivityIsNotCollectedForAnUnreachableService — a service the agent cannot form an
// opinion about has no activity worth reporting, and asking would spend another timeout on
// something already known to be unresponsive.
func TestActivityIsNotCollectedForAnUnreachableService(t *testing.T) {
	svc := &activeService{
		playing: domain.NowPlaying{Sessions: 5},
		moving:  domain.Transfers{Active: 5},
	}
	svc.set(domain.Health{}, errors.New("cannot form an opinion"))
	p := newActivePoller(t, svc)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p.Start(ctx)

	waitFor(t, "the failed poll", func() bool {
		s, _ := p.Cache().Get("svc")
		return s.Health.Status == domain.StatusUnreachable
	})

	s, _ := p.Cache().Get("svc")
	if s.NowPlaying != nil || s.Transfers != nil {
		t.Error("activity was collected for a service that could not be observed")
	}

	cancel()
	p.Wait()
}

// TestSampledListsAreBounded is the property that keeps an SSE frame small.
//
// The cap is enforced by the poller rather than trusted to each adapter, because an
// adapter that forgot would be discovered only as a slow stream on somebody's phone.
func TestSampledListsAreBounded(t *testing.T) {
	sessions := make([]domain.PlaybackSession, 40)
	items := make([]domain.TransferItem, 400)
	svc := &activeService{
		playing: domain.NowPlaying{Sessions: 40, Items: sessions},
		moving:  domain.Transfers{Active: 400, Total: 400, Items: items},
	}
	p := newActivePoller(t, svc)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p.Start(ctx)

	waitFor(t, "the first poll", func() bool {
		s, _ := p.Cache().Get("svc")
		return s.NowPlaying != nil && s.Transfers != nil
	})

	s, _ := p.Cache().Get("svc")
	if len(s.NowPlaying.Items) != domain.MaxActivityItems {
		t.Errorf("sessions sample = %d, want %d", len(s.NowPlaying.Items), domain.MaxActivityItems)
	}
	if len(s.Transfers.Items) != domain.MaxActivityItems {
		t.Errorf("transfers sample = %d, want %d", len(s.Transfers.Items), domain.MaxActivityItems)
	}
	// The counts stay true, which is the whole reason they are separate from the sample.
	if s.NowPlaying.Sessions != 40 || s.Transfers.Total != 400 {
		t.Errorf("counts were trimmed along with the sample: %d / %d",
			s.NowPlaying.Sessions, s.Transfers.Total)
	}

	cancel()
	p.Wait()
}

// TestBoundedLeavesShortListsAlone — the cap must not allocate or copy in the common case.
func TestBoundedLeavesShortListsAlone(t *testing.T) {
	short := []domain.TransferItem{{ID: "a"}, {ID: "b"}}
	if got := domain.Bounded(short); len(got) != 2 {
		t.Errorf("Bounded trimmed a short list to %d", len(got))
	}
	if domain.Bounded[domain.TransferItem](nil) != nil {
		t.Error("Bounded should leave nil alone")
	}
}

// TestAServiceWithoutActivityCapabilitiesStoresNothing — the vast majority of adapters
// will implement neither, and they must not pay for the feature.
func TestAServiceWithoutActivityCapabilitiesStoresNothing(t *testing.T) {
	plain := &fakeService{}
	p := newTestPoller(t, map[string]*fakeService{"svc": plain}, time.Hour, time.Second)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p.Start(ctx)

	waitFor(t, "the first poll", func() bool {
		s, _ := p.Cache().Get("svc")
		return s.Health.Status == domain.StatusHealthy
	})

	s, _ := p.Cache().Get("svc")
	if s.NowPlaying != nil || s.Transfers != nil {
		t.Error("a service implementing neither capability got a payload")
	}

	cancel()
	p.Wait()
}
