package poll_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/katbyte/truenas-exporter/lib/poll"
	"github.com/katbyte/truenas-exporter/lib/poll/polltest"
)

// every is how often the tests poll: the program's own defaults, every 30 seconds, 5 or 10 minutes, or 4 hours by area.
var every = poll.DefaultIntervals()

// errDown is a NAS that is not there.
var errDown = errors.New("connection refused")

func newPoller(t *testing.T, nas poll.NAS, skip ...poll.Area) *poll.Poller {
	t.Helper()

	p, err := poll.New(nas, every, skip...)
	if err != nil {
		t.Fatal(err)
	}

	return p
}

// A poll keeps what the NAS said, and how long the asking took.
func TestAPollKeepsItsAnswers(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{
			Slow:         2 * time.Second,
			SystemAnswer: poll.System{Version: "25.10.7", State: "READY"},
			PoolsAnswer:  []poll.Pool{{Name: "tank", Status: "ONLINE", Healthy: true}},
		}
		p := newPoller(t, nas)

		start := time.Now()
		p.Poll(t.Context(), poll.AreaSystem)
		p.Poll(t.Context(), poll.AreaPools)
		s := p.State()

		// every question about an area is put at once, so the poll takes as long as one of them
		if r := s.Rounds[poll.AreaPools]; !r.Ran || !r.OK || r.Took != 2*time.Second || !r.LastSuccess.Equal(start.Add(4*time.Second)) || r.Failures != 0 || r.Interval != 30*time.Second {
			t.Errorf("pools = %+v, want a poll every 30s that ran, answered and took 2s", r)
		}
		if !s.System.Known() || s.System.Value.Version != "25.10.7" || !s.Up() {
			t.Errorf("system = %+v, up %t; want the NAS's own state, and the NAS up", s.System, s.Up())
		}
		if !s.Pools.Known() || len(s.Pools.Value) != 1 || !s.BootPool.Known() {
			t.Errorf("pools = %+v, boot pool known %t; want the one pool and the boot pool", s.Pools, s.BootPool.Known())
		}
	})
}

// A poll asks about its own area and nothing else.
func TestAPollAsksAboutItsOwnArea(t *testing.T) {
	t.Parallel()

	for area, asked := range polltest.Questions {
		nas := &polltest.NAS{}
		newPoller(t, nas).Poll(t.Context(), area)

		for other, questions := range polltest.Questions {
			for _, question := range questions {
				if want := map[bool]int{true: 1, false: 0}[other == area]; nas.Asked(question) != want {
					t.Errorf("polling %s asked %s %d times, want %d (it asks %v)", area, question, nas.Asked(question), want, asked)
				}
			}
		}
	}
}

// One question failing must not cost the others their answers, nor the question the answer it last had: a NAS that refuses one read is still
// reported on for the rest.
func TestAFailedQuestionKeepsTheLastGoodAnswer(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{PoolsAnswer: make([]poll.Pool, 2), BootPoolAnswer: poll.Pool{Name: "boot-pool"}}
		p := newPoller(t, nas)

		p.Poll(t.Context(), poll.AreaPools)
		first := time.Now()

		time.Sleep(time.Minute)
		nas.Fail(polltest.BootPool, errDown)
		nas.PoolsAnswer = make([]poll.Pool, 3)
		p.Poll(t.Context(), poll.AreaPools)
		s := p.State()

		if r := s.Rounds[poll.AreaPools]; r.OK || r.Failures != 1 || !r.LastSuccess.Equal(first) {
			t.Errorf("pools = %+v, want one failure and the first poll as its last success", r)
		}
		if !s.BootPool.Known() || s.BootPool.Value.Name != "boot-pool" || !s.BootPool.At.Equal(first) {
			t.Errorf("boot pool = %+v, want the one from the first poll", s.BootPool)
		}
		if len(s.Pools.Value) != 3 {
			t.Errorf("pools = %d, want the 3 asked for after the boot pool failed", len(s.Pools.Value))
		}

		nas.Fail(polltest.BootPool, nil)
		p.Poll(t.Context(), poll.AreaPools)
		if r := p.State().Rounds[poll.AreaPools]; !r.OK || r.Failures != 1 {
			t.Errorf("pools = %+v, want it answering again with the one failure still counted", r)
		}
	})
}

// Up is the NAS's own state: another area failing says a read went wrong, not that the NAS is gone.
func TestUpFollowsTheNASsOwnState(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{}
		p := newPoller(t, nas)

		if s := p.State(); s.Up() || s.Rounds[poll.AreaSystem].Ran {
			t.Error("a NAS never asked is up")
		}

		nas.Fail(polltest.Datasets, errDown)
		p.PollAll(t.Context())
		if s := p.State(); !s.Up() || s.Rounds[poll.AreaDatasets].OK || s.Answered() {
			t.Error("a failed poll of datasets took the NAS down, or did not fail")
		}

		nas.Fail("", errDown)
		p.Poll(t.Context(), poll.AreaSystem)
		if s := p.State(); s.Up() || s.Rounds[poll.AreaSystem].Failures != 1 {
			t.Errorf("system = %+v, want the NAS down after one failure", s.Rounds[poll.AreaSystem])
		}
	})
}

// An area left out is never asked about and has no answer, nor a poll to say how it went, and the rest go on without it. The NAS's own state is
// asked about even with every area left out, so whether it answers is still known.
func TestAnAreaLeftOutIsNeverAsked(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		left := []poll.Area{poll.AreaPools, poll.AreaShares, poll.AreaApps, poll.AreaDisks}
		nas := &polltest.NAS{}
		p := newPoller(t, nas, left...)

		p.PollAll(t.Context())
		p.Poll(t.Context(), poll.AreaApps)
		s := p.State()

		for area, questions := range polltest.Questions {
			skipped := false
			for _, l := range left {
				skipped = skipped || l == area
			}

			for _, question := range questions {
				if want := map[bool]int{true: 0, false: 1}[skipped]; nas.Asked(question) != want {
					t.Errorf("%s was asked %d times, want %d", question, nas.Asked(question), want)
				}
			}
			if _, polled := s.Rounds[area]; polled == skipped {
				t.Errorf("%s has a poll %t, want %t", area, polled, !skipped)
			}
		}
		if s.Pools.Known() || s.BootPool.Known() || s.Shares.Known() || s.Apps.Known() || s.Disks.Known() {
			t.Error("an area left out has an answer")
		}
		if !s.Answered() || !s.Up() || !s.Alerts.Known() || !s.Clients.Known() {
			t.Errorf("polls %+v; want every one answered, and the areas asked about known", s.Rounds)
		}

		nas = &polltest.NAS{}
		p = newPoller(t, nas, poll.Areas...)

		p.PollAll(t.Context())
		if s := p.State(); nas.Asked(polltest.System) != 1 || !s.System.Known() || !s.Up() || len(s.Rounds) != 1 {
			t.Errorf("with every area left out the NAS's own state was asked %d times, up is %t and %d areas were polled; want it asked once, up, and alone", nas.Asked(polltest.System), s.Up(), len(s.Rounds))
		}
	})
}

// A NAS that has stopped answering is served from its last answers for three of each area's intervals and no longer: after that they are no longer
// worth showing as current, least of all the health of its pools.
func TestAnAnswerIsDroppedOnceStale(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{PoolsAnswer: []poll.Pool{{Name: "tank", Healthy: true}}, DisksAnswer: make([]poll.Disk, 3)}
		p := newPoller(t, nas)
		p.PollAll(t.Context())
		nas.Fail("", errDown)

		time.Sleep(3 * 30 * time.Second)
		p.Poll(t.Context(), poll.AreaSystem)
		if s := p.State(); !s.Pools.Known() || !s.System.Known() || s.Up() {
			t.Errorf("after three intervals: pools known %t, system known %t, up %t; want the last answers still served and the NAS down", s.Pools.Known(), s.System.Known(), s.Up())
		}

		time.Sleep(time.Second)
		if s := p.State(); s.Pools.Known() || s.System.Known() || s.BootPool.Known() || s.Alerts.Known() || s.Services.Known() || s.Jobs.Known() || s.Apps.Known() || s.VMs.Known() || s.Clients.Known() {
			t.Error("the answers asked for every 30 seconds outlived three intervals")
		}
		if s := p.State(); !s.Disks.Known() || !s.Datasets.Known() {
			t.Error("the answers asked for every 5 minutes were dropped on another area's clock")
		}

		time.Sleep(3 * 5 * time.Minute)
		if s := p.State(); s.Disks.Known() || s.Datasets.Known() || s.Tasks.Known() || s.Shares.Known() || s.Certificates.Known() || s.Interfaces.Known() || s.Directory.Known() || s.Key.Known() {
			t.Error("the answers asked for every 5 minutes outlived three intervals")
		}
		if s := p.State(); !s.Snapshots.Known() {
			t.Error("the snapshot counts, asked for every 10 minutes, were dropped on another area's clock")
		}

		time.Sleep(3 * 5 * time.Minute)
		if s := p.State(); s.Snapshots.Known() {
			t.Error("the snapshot counts outlived three intervals")
		}
		if s := p.State(); !s.Update.Known() {
			t.Error("the update check, asked for every 4 hours, was dropped on another area's clock")
		}

		time.Sleep(12 * time.Hour)
		if s := p.State(); s.Update.Known() {
			t.Error("the update check outlived three intervals")
		}
	})
}

// Run asks about everything at once, so the page has something on it as soon as it is served, and then about each area on its own interval until
// it is told to stop.
func TestRunPollsEachAreaOnItsOwnSchedule(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{}
		p := newPoller(t, nas)

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			p.Run(ctx)
			close(done)
		}()

		synctest.Wait()
		for _, question := range []string{polltest.Pools, polltest.Disks, polltest.Snapshots} {
			if nas.Asked(question) != 1 {
				t.Errorf("on start: %s asked %d times, want once", question, nas.Asked(question))
			}
		}

		// ten minutes is 20 more polls every 30 seconds, 2 more every 5 minutes, and 1 more every 10
		time.Sleep(10*time.Minute + time.Second)
		synctest.Wait()
		for question, want := range map[string]int{polltest.Pools: 21, polltest.Disks: 3, polltest.Snapshots: 2} {
			if nas.Asked(question) != want {
				t.Errorf("after ten minutes: %s asked %d times, want %d", question, nas.Asked(question), want)
			}
		}

		cancel()
		<-done
	})
}

// A poll is given until the next is due and no longer, so a NAS that hangs is reported down rather than holding every later poll up behind it.
func TestAPollIsGivenItsInterval(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{Slow: time.Hour}
		p := newPoller(t, nas)

		p.Poll(t.Context(), poll.AreaSystem)
		p.Poll(t.Context(), poll.AreaSnapshots)
		s := p.State()

		if r := s.Rounds[poll.AreaSystem]; !r.Ran || r.OK || r.Took != 30*time.Second || r.Failures != 1 {
			t.Errorf("system = %+v, want a poll that failed after its 30s", r)
		}
		if r := s.Rounds[poll.AreaSnapshots]; r.OK || r.Took != 10*time.Minute {
			t.Errorf("snapshots = %+v, want a poll that failed after its 10m", r)
		}
	})
}

// A poll cut short because the program is stopping says nothing about the NAS, and is not counted against it.
func TestStoppingIsNotAFailure(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{Slow: time.Minute}
		p := newPoller(t, nas)

		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			time.Sleep(time.Second)
			cancel()
		}()
		p.Poll(ctx, poll.AreaSystem)

		if r := p.State().Rounds[poll.AreaSystem]; r.Ran || r.Failures != 0 {
			t.Errorf("system = %+v, want nothing recorded for a poll that was stopped", r)
		}
	})
}

// An area with an interval of nothing, or none at all, would be asked about without pause. One left out needs none.
func TestNewRefusesAnIntervalOfNothing(t *testing.T) {
	t.Parallel()

	for _, area := range poll.Every {
		for _, bad := range []time.Duration{0, -time.Second} {
			intervals := poll.DefaultIntervals()
			intervals[area] = bad

			if _, err := poll.New(&polltest.NAS{}, intervals); err == nil {
				t.Errorf("New accepted %s every %s", area, bad)
			}
		}
	}

	intervals := poll.DefaultIntervals()
	delete(intervals, poll.AreaSnapshots)
	if _, err := poll.New(&polltest.NAS{}, intervals); err == nil {
		t.Error("New accepted snapshots with no interval")
	}
	if _, err := poll.New(&polltest.NAS{}, intervals, poll.AreaSnapshots); err != nil {
		t.Errorf("New refused snapshots left out with no interval: %v", err)
	}
}

// How deep a dataset is decides whether it is on the page, so the count has to be the slashes in its name and nothing cleverer.
func TestADatasetsDepthIsHowFarBelowItsPoolItIs(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]int{"tank": 0, "tank/media": 1, "tank/media/films": 2, "": 0} {
		if got := (&poll.Dataset{Name: name}).Depth(); got != want {
			t.Errorf("depth of %q = %d, want %d", name, got, want)
		}
		if got := (&poll.SnapshotCount{Dataset: name}).Depth(); got != want {
			t.Errorf("depth of the snapshots of %q = %d, want %d", name, got, want)
		}
	}
}
