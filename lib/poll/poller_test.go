package poll_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/katbyte/truenas-metrics/lib/poll"
	"github.com/katbyte/truenas-metrics/lib/poll/polltest"
)

// every is how often the tests poll: the program's own defaults.
var every = poll.Intervals{Fast: 30 * time.Second, Slow: 5 * time.Minute}

// errDown is a NAS that is not there.
var errDown = errors.New("connection refused")

func newPoller(t *testing.T, nas poll.NAS) *poll.Poller {
	t.Helper()

	p, err := poll.New(nas, every)
	if err != nil {
		t.Fatal(err)
	}

	return p
}

// A round keeps what the NAS said, and how long the asking took, and leaves the other round's questions unasked.
func TestARoundKeepsItsAnswers(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{
			Slow:         2 * time.Second,
			SystemAnswer: poll.System{Version: "25.10.7", State: "READY"},
			PoolsAnswer:  []poll.Pool{{Name: "tank", Status: "ONLINE", Healthy: true}},
		}
		p := newPoller(t, nas)

		start := time.Now()
		p.Poll(t.Context(), poll.RoundFast)
		s := p.State()

		// every question is put at once, so the round takes as long as one of them
		if r := s.FastRound; !r.Ran || !r.OK || r.Took != 2*time.Second || !r.LastSuccess.Equal(start.Add(2*time.Second)) || r.Failures != 0 {
			t.Errorf("fast round = %+v, want one that ran, answered and took 2s", r)
		}
		if !s.Up() {
			t.Error("the NAS answered and is not up")
		}
		if !s.System.Known() || s.System.Value.Version != "25.10.7" {
			t.Errorf("system = %+v, want the NAS's", s.System)
		}
		if !s.Pools.Known() || len(s.Pools.Value) != 1 || !s.Alerts.Known() {
			t.Errorf("pools = %+v and alerts = %+v, want both answered", s.Pools, s.Alerts)
		}

		for _, question := range polltest.Fast {
			if nas.Asked(question) != 1 {
				t.Errorf("%s was asked %d times in a fast round, want once", question, nas.Asked(question))
			}
		}
		for _, question := range polltest.Slow {
			if nas.Asked(question) != 0 {
				t.Errorf("%s was asked in a fast round", question)
			}
		}
		if s.SlowRound.Ran || s.Disks.Known() || s.Datasets.Known() {
			t.Errorf("the slow round ran with the fast one: %+v", s.SlowRound)
		}
	})
}

// The slow round asks its own questions and none of the fast one's.
func TestTheSlowRoundAsksItsOwnQuestions(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{DisksAnswer: make([]poll.Disk, 4)}
		p := newPoller(t, nas)

		p.Poll(t.Context(), poll.RoundSlow)
		s := p.State()

		for _, question := range polltest.Slow {
			if nas.Asked(question) != 1 {
				t.Errorf("%s was asked %d times in a slow round, want once", question, nas.Asked(question))
			}
		}
		for _, question := range polltest.Fast {
			if nas.Asked(question) != 0 {
				t.Errorf("%s was asked in a slow round", question)
			}
		}
		if !s.SlowRound.OK || len(s.Disks.Value) != 4 || !s.Key.Known() || !s.Update.Known() || s.Up() {
			t.Errorf("slow round = %+v with %d disks, want it answered and the NAS not yet called up", s.SlowRound, len(s.Disks.Value))
		}
	})
}

// One question failing must not cost the others their answers, nor the question the answer it last had: a NAS that refuses one read is still
// reported on for the rest.
func TestAFailedQuestionKeepsTheLastGoodAnswer(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{TasksAnswer: []poll.Task{{}, {}}, DisksAnswer: make([]poll.Disk, 3)}
		p := newPoller(t, nas)

		p.Poll(t.Context(), poll.RoundSlow)
		first := time.Now()

		time.Sleep(time.Minute)
		nas.Fail(polltest.Tasks, errDown)
		nas.DisksAnswer = make([]poll.Disk, 4)
		p.Poll(t.Context(), poll.RoundSlow)
		s := p.State()

		if r := s.SlowRound; r.OK || r.Failures != 1 || !r.LastSuccess.Equal(first) {
			t.Errorf("slow round = %+v, want one failure and the first round as its last success", r)
		}
		if !s.Tasks.Known() || len(s.Tasks.Value) != 2 || !s.Tasks.At.Equal(first) {
			t.Errorf("tasks = %+v, want the two from the first round", s.Tasks)
		}
		if len(s.Disks.Value) != 4 {
			t.Errorf("disks = %d, want the 4 asked for after tasks failed", len(s.Disks.Value))
		}

		nas.Fail(polltest.Tasks, nil)
		p.Poll(t.Context(), poll.RoundSlow)
		if r := p.State().SlowRound; !r.OK || r.Failures != 1 {
			t.Errorf("slow round = %+v, want it answering again with the one failure still counted", r)
		}
	})
}

// Up is the fast round's, the one that runs often enough to tell: a slow round that fails says a read went wrong, not that the NAS is gone.
func TestUpFollowsTheFastRound(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{}
		p := newPoller(t, nas)

		if s := p.State(); s.Up() || s.FastRound.Ran {
			t.Error("a NAS never asked is up")
		}

		nas.Fail(polltest.Datasets, errDown)
		p.Poll(t.Context(), poll.RoundFast)
		p.Poll(t.Context(), poll.RoundSlow)
		if s := p.State(); !s.Up() || s.SlowRound.OK {
			t.Error("a failed slow round took the NAS down, or did not fail")
		}

		nas.Fail("", errDown)
		p.Poll(t.Context(), poll.RoundFast)
		if s := p.State(); s.Up() || s.FastRound.Failures != 1 {
			t.Errorf("fast round = %+v, want the NAS down after one failure", s.FastRound)
		}
	})
}

// A NAS that has stopped answering is served from its last answers for three intervals and no longer: after that they are no longer worth showing
// as current, least of all the health of its pools.
func TestAnAnswerIsDroppedOnceStale(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{PoolsAnswer: []poll.Pool{{Name: "tank", Healthy: true}}, DisksAnswer: make([]poll.Disk, 3)}
		p := newPoller(t, nas)
		p.Poll(t.Context(), poll.RoundFast)
		p.Poll(t.Context(), poll.RoundSlow)
		nas.Fail("", errDown)

		time.Sleep(3 * every.Fast)
		p.Poll(t.Context(), poll.RoundFast)
		if s := p.State(); !s.Pools.Known() || !s.System.Known() || s.Up() {
			t.Errorf("after three intervals: pools known %t, system known %t, up %t; want the last answers still served and the NAS down", s.Pools.Known(), s.System.Known(), s.Up())
		}

		time.Sleep(time.Second)
		if s := p.State(); s.Pools.Known() || s.System.Known() || s.BootPool.Known() || s.Alerts.Known() || s.Services.Known() || s.Jobs.Known() || s.Apps.Known() || s.VMs.Known() || s.Clients.Known() {
			t.Error("the fast round's answers outlived three intervals")
		}
		if s := p.State(); !s.Disks.Known() || !s.Datasets.Known() {
			t.Error("the slow round's answers were dropped on the fast round's clock")
		}

		time.Sleep(3 * every.Slow)
		if s := p.State(); s.Disks.Known() || s.Datasets.Known() || s.Tasks.Known() || s.Shares.Known() || s.Certificates.Known() || s.Interfaces.Known() || s.Update.Known() || s.Directory.Known() || s.Key.Known() {
			t.Error("the slow round's answers outlived three intervals")
		}
	})
}

// Run asks at once, so the page has something on it as soon as it is served, and then on each round's own interval until it is told to stop.
func TestRunPollsOnBothSchedules(t *testing.T) {
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
		if nas.Asked(polltest.Pools) != 1 || nas.Asked(polltest.Disks) != 1 {
			t.Errorf("on start: pools asked %d times and disks %d, want once each", nas.Asked(polltest.Pools), nas.Asked(polltest.Disks))
		}

		// five minutes of a round every 30 seconds is 10 more, and one more of the round every five minutes
		time.Sleep(every.Slow + time.Second)
		synctest.Wait()
		if nas.Asked(polltest.Pools) != 11 || nas.Asked(polltest.Disks) != 2 {
			t.Errorf("after five minutes: pools asked %d times and disks %d, want 11 and 2", nas.Asked(polltest.Pools), nas.Asked(polltest.Disks))
		}

		cancel()
		<-done
	})
}

// A round is given until the next is due and no longer, so a NAS that hangs is reported down rather than holding every later round up behind it.
func TestARoundIsGivenItsInterval(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		nas := &polltest.NAS{Slow: time.Hour}
		p := newPoller(t, nas)

		p.Poll(t.Context(), poll.RoundFast)

		if r := p.State().FastRound; !r.Ran || r.OK || r.Took != every.Fast || r.Failures != 1 {
			t.Errorf("fast round = %+v, want one that failed after its 30s", r)
		}
	})
}

// A round cut short because the program is stopping says nothing about the NAS, and is not counted against it.
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
		p.Poll(ctx, poll.RoundFast)

		if r := p.State().FastRound; r.Ran || r.Failures != 0 {
			t.Errorf("fast round = %+v, want nothing recorded for a round that was stopped", r)
		}
	})
}

// An interval of nothing would poll without pause.
func TestNewRefusesAnIntervalOfNothing(t *testing.T) {
	t.Parallel()

	for _, bad := range []poll.Intervals{{}, {Fast: time.Second}, {Slow: time.Second}, {Fast: -time.Second, Slow: time.Second}} {
		if _, err := poll.New(&polltest.NAS{}, bad); err == nil {
			t.Errorf("New accepted %+v", bad)
		}
	}
}

// How deep a dataset is decides whether it is on the page, so the count has to be the slashes in its name and nothing cleverer.
func TestADatasetsDepthIsHowFarBelowItsPoolItIs(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]int{"tank": 0, "tank/media": 1, "tank/media/films": 2, "": 0} {
		if got := (&poll.Dataset{Name: name}).Depth(); got != want {
			t.Errorf("depth of %q = %d, want %d", name, got, want)
		}
	}
}
