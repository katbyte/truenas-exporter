package poll

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/go-kt/outside"
)

// Round is one of the two schedules a NAS is asked on.
type Round string

const (
	// RoundFast is what changes by the minute: pools and their scans, alerts, services, jobs, apps and the NAS's own state.
	RoundFast Round = "fast"
	// RoundSlow is what changes when somebody changes it or a task runs: disks, datasets, tasks, shares, certificates and updates.
	RoundSlow Round = "slow"
)

// Rounds is both rounds, in the order the page lists them.
var Rounds = []Round{RoundFast, RoundSlow}

// staleAfter is how many intervals an answer is served for once its round stops succeeding: long enough to ride out a failed round or two, short
// enough that a NAS down for an hour is not still shown with the pools it had.
const staleAfter = 3

// Intervals is how often each round runs. A round is also given that long to finish, so one never overlaps the next.
type Intervals struct {
	Fast time.Duration
	Slow time.Duration
}

// of is the interval of one round.
func (i Intervals) of(round Round) time.Duration {
	if round == RoundSlow {
		return i.Slow
	}

	return i.Fast
}

// Answer is the last good answer to one question.
type Answer[T any] struct {
	Value T
	// At is when it was given, and the zero time for a question never answered.
	At time.Time
}

// Known says whether there is an answer to serve.
func (a Answer[T]) Known() bool { return !a.At.IsZero() }

// RoundState is how one round last went.
type RoundState struct {
	// Ran says a round has finished since the start; until one has, the rest says nothing.
	Ran bool
	// OK says every question of the last round was answered.
	OK   bool
	Took time.Duration
	// LastSuccess is the zero time until a round succeeds.
	LastSuccess time.Time
	// Failures counts the rounds that failed since the start.
	Failures int
}

// State is what is known of the NAS: the last good answer to each question, and how the rounds that asked went.
type State struct {
	System   Answer[System]
	Pools    Answer[[]Pool]
	BootPool Answer[Pool]
	Alerts   Answer[[]Alert]
	Services Answer[[]Service]
	Jobs     Answer[[]Job]
	Apps     Answer[Apps]
	VMs      Answer[[]VM]
	Clients  Answer[[]Clients]

	Disks        Answer[[]Disk]
	Datasets     Answer[[]Dataset]
	Tasks        Answer[[]Task]
	Shares       Answer[[]Share]
	Certificates Answer[[]Certificate]
	Interfaces   Answer[[]Interface]
	Update       Answer[Update]
	Directory    Answer[Directory]
	Key          Answer[Key]

	FastRound RoundState
	SlowRound RoundState
}

// Up says whether the NAS answered the last fast round, the one that runs often enough to tell.
func (s *State) Up() bool { return s.FastRound.OK }

// Round is how one round last went.
func (s *State) Round(round Round) RoundState {
	if round == RoundSlow {
		return s.SlowRound
	}

	return s.FastRound
}

// roundOf is where one round's state is kept.
func (s *State) roundOf(round Round) *RoundState {
	if round == RoundSlow {
		return &s.SlowRound
	}

	return &s.FastRound
}

// Poller asks one NAS its questions on two schedules and remembers the last good answer to each.
type Poller struct {
	nas   NAS
	every Intervals

	mu    sync.RWMutex
	state State
}

// New is a poller for one NAS. It asks nothing until Run or Poll.
func New(nas NAS, every Intervals) (*Poller, error) {
	if every.Fast <= 0 || every.Slow <= 0 {
		return nil, fmt.Errorf("the intervals must be longer than nothing: fast %s, slow %s", every.Fast, every.Slow)
	}

	return &Poller{nas: nas, every: every}, nil
}

// Run polls until the context ends: both rounds at once on start, then each on its own interval.
func (p *Poller) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, round := range Rounds {
		wg.Go(func() { p.loop(ctx, round) })
	}
	wg.Wait()
}

// loop runs one round now and then every interval, until the context ends.
func (p *Poller) loop(ctx context.Context, round Round) {
	ticker := time.NewTicker(p.every.of(round))
	defer ticker.Stop()

	for {
		p.Poll(ctx, round)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Poll runs one round now and records how it went. A question that fails leaves its last good answer where it was, and does not stop the others
// being asked.
func (p *Poller) Poll(ctx context.Context, round Round) {
	within, cancel := context.WithTimeout(ctx, p.every.of(round))
	defer cancel()

	start := time.Now()
	err := p.ask(within, round)
	// a round cut short because the program is stopping says nothing about the NAS
	if err != nil && ctx.Err() != nil {
		return
	}

	p.report(round, p.record(round, time.Since(start), err), err)
}

// record keeps how a round went, and gives back how the one before it did.
func (p *Poller) record(round Round, took time.Duration, err error) RoundState {
	p.mu.Lock()
	defer p.mu.Unlock()

	r := p.state.roundOf(round)
	before := *r
	r.Ran, r.OK, r.Took = true, err == nil, took
	if err != nil {
		r.Failures++
	} else {
		r.LastSuccess = time.Now()
	}

	return before
}

// report logs a round only when it changes, since a NAS that is down fails every round the same way.
func (p *Poller) report(round Round, before RoundState, err error) {
	switch {
	case err != nil && (before.OK || !before.Ran):
		clog.Log.Warnf("the %s round failed, and will be tried again every %s: %s", round, p.every.of(round), said(err))
	case err != nil:
		clog.Log.Debugf("the %s round failed again: %s", round, said(err))
	case before.Ran && !before.OK:
		clog.Log.Infof("the %s round is answering again", round)
	case !before.Ran:
		clog.Log.Infof("the first %s round answered", round)
	}
}

// said is what went wrong as one line of a log: a round's failures come one to a line, and a NAS's own words for one may quote an address with a key
// on it.
func said(err error) string {
	return strings.ReplaceAll(outside.BlankAddresses(err.Error()), "\n", "; ")
}

// ask puts every question of one round at once, and gives back what failed. The NAS answers them over one connection, as its own pages do.
func (p *Poller) ask(ctx context.Context, round Round) error {
	questions := []func() error{
		func() error {
			return keep(ctx, p, "system", p.nas.System, func(s *State) *Answer[System] { return &s.System })
		},
		func() error {
			return keep(ctx, p, "pools", p.nas.Pools, func(s *State) *Answer[[]Pool] { return &s.Pools })
		},
		func() error {
			return keep(ctx, p, "boot pool", p.nas.BootPool, func(s *State) *Answer[Pool] { return &s.BootPool })
		},
		func() error {
			return keep(ctx, p, "alerts", p.nas.Alerts, func(s *State) *Answer[[]Alert] { return &s.Alerts })
		},
		func() error {
			return keep(ctx, p, "services", p.nas.Services, func(s *State) *Answer[[]Service] { return &s.Services })
		},
		func() error {
			return keep(ctx, p, "jobs", p.nas.Jobs, func(s *State) *Answer[[]Job] { return &s.Jobs })
		},
		func() error { return keep(ctx, p, "apps", p.nas.Apps, func(s *State) *Answer[Apps] { return &s.Apps }) },
		func() error {
			return keep(ctx, p, "virtual machines", p.nas.VMs, func(s *State) *Answer[[]VM] { return &s.VMs })
		},
		func() error {
			return keep(ctx, p, "clients", p.nas.Clients, func(s *State) *Answer[[]Clients] { return &s.Clients })
		},
	}

	if round == RoundSlow {
		questions = []func() error{
			func() error {
				return keep(ctx, p, "disks", p.nas.Disks, func(s *State) *Answer[[]Disk] { return &s.Disks })
			},
			func() error {
				return keep(ctx, p, "datasets", p.nas.Datasets, func(s *State) *Answer[[]Dataset] { return &s.Datasets })
			},
			func() error {
				return keep(ctx, p, "tasks", p.nas.Tasks, func(s *State) *Answer[[]Task] { return &s.Tasks })
			},
			func() error {
				return keep(ctx, p, "shares", p.nas.Shares, func(s *State) *Answer[[]Share] { return &s.Shares })
			},
			func() error {
				return keep(ctx, p, "certificates", p.nas.Certificates, func(s *State) *Answer[[]Certificate] { return &s.Certificates })
			},
			func() error {
				return keep(ctx, p, "interfaces", p.nas.Interfaces, func(s *State) *Answer[[]Interface] { return &s.Interfaces })
			},
			func() error {
				return keep(ctx, p, "update", p.nas.Update, func(s *State) *Answer[Update] { return &s.Update })
			},
			func() error {
				return keep(ctx, p, "directory service", p.nas.Directory, func(s *State) *Answer[Directory] { return &s.Directory })
			},
			func() error { return keep(ctx, p, "API key", p.nas.Key, func(s *State) *Answer[Key] { return &s.Key }) },
		}
	}

	failed := make([]error, len(questions))

	var wg sync.WaitGroup
	for i, question := range questions {
		wg.Go(func() { failed[i] = question() })
	}
	wg.Wait()

	return errors.Join(failed...)
}

// keep puts one question and keeps its answer in the place into names. An answer is stored whole and never changed afterwards, so State can hand it
// out without copying.
func keep[T any](ctx context.Context, p *Poller, what string, question func(context.Context) (T, error), into func(*State) *Answer[T]) error {
	value, err := question(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	*into(&p.state) = Answer[T]{Value: value, At: time.Now()}

	return nil
}

// State is what is known of the NAS now, without asking it anything. An answer older than three of its round's intervals is left out: the NAS has
// stopped answering, and what it last said is no longer worth showing as current.
func (p *Poller) State() State {
	p.mu.RLock()
	defer p.mu.RUnlock()

	s := p.state
	now := time.Now()
	fast, slow := staleAfter*p.every.Fast, staleAfter*p.every.Slow

	fresh(&s.System, now, fast)
	fresh(&s.Pools, now, fast)
	fresh(&s.BootPool, now, fast)
	fresh(&s.Alerts, now, fast)
	fresh(&s.Services, now, fast)
	fresh(&s.Jobs, now, fast)
	fresh(&s.Apps, now, fast)
	fresh(&s.VMs, now, fast)
	fresh(&s.Clients, now, fast)

	fresh(&s.Disks, now, slow)
	fresh(&s.Datasets, now, slow)
	fresh(&s.Tasks, now, slow)
	fresh(&s.Shares, now, slow)
	fresh(&s.Certificates, now, slow)
	fresh(&s.Interfaces, now, slow)
	fresh(&s.Update, now, slow)
	fresh(&s.Directory, now, slow)
	fresh(&s.Key, now, slow)

	return s
}

// fresh forgets an answer older than limit.
func fresh[T any](a *Answer[T], now time.Time, limit time.Duration) {
	if a.Known() && now.Sub(a.At) > limit {
		*a = Answer[T]{}
	}
}
