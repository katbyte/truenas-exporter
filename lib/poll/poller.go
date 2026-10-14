package poll

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/go-kt/outside"
)

// Area is one part of the NAS, asked about on a schedule of its own: its apps, say.
type Area string

// The areas, as the settings name them.
const (
	// AreaSystem is the NAS's own state. It is always asked about, since it is what says the NAS is answering at all.
	AreaSystem       Area = "system"
	AreaPools        Area = "pools"
	AreaDisks        Area = "disks"
	AreaDatasets     Area = "datasets"
	AreaSnapshots    Area = "snapshots"
	AreaAlerts       Area = "alerts"
	AreaServices     Area = "services"
	AreaJobs         Area = "jobs"
	AreaApps         Area = "apps"
	AreaVMs          Area = "vms"
	AreaShares       Area = "shares"
	AreaClients      Area = "clients"
	AreaTasks        Area = "tasks"
	AreaUpdates      Area = "updates"
	AreaInterfaces   Area = "interfaces"
	AreaCertificates Area = "certificates"
	AreaDirectory    Area = "directory"
	AreaKeyExpiry    Area = "key-expiry"
)

// Areas is every area the settings may leave out, in the order the settings list them.
var Areas = []Area{AreaPools, AreaDisks, AreaDatasets, AreaSnapshots, AreaAlerts, AreaServices, AreaJobs, AreaApps, AreaVMs, AreaShares, AreaClients, AreaTasks, AreaUpdates, AreaInterfaces, AreaCertificates, AreaDirectory, AreaKeyExpiry}

// Every is every area, the NAS's own state first, in the order the page lists them.
var Every = append([]Area{AreaSystem}, Areas...)

// staleAfter is how many intervals an answer is served for once its area stops being answered: long enough to ride out a failed poll or two, short
// enough that a NAS down for an hour is not still shown with the pools it had.
const staleAfter = 3

// Intervals is how often each area is asked about. A poll is also given that long to finish, so one never overlaps the next.
type Intervals map[Area]time.Duration

// DefaultIntervals is how often each area is asked about unless the settings say otherwise. What changes by the minute is asked about as often as a
// scrape every 30 seconds asks, and what changes only when somebody changes it or a task runs every few minutes. Counting snapshots has the NAS go
// through every one, so it is asked less often still, and a newer TrueNAS comes out seldom enough that the NAS need only look every few hours.
func DefaultIntervals() Intervals {
	fast, slow := 30*time.Second, 5*time.Minute

	return Intervals{
		AreaSystem: fast, AreaPools: fast, AreaAlerts: fast, AreaServices: fast, AreaJobs: fast, AreaApps: fast, AreaVMs: fast, AreaClients: fast,
		AreaDisks: slow, AreaDatasets: slow, AreaShares: slow, AreaTasks: slow, AreaInterfaces: slow, AreaCertificates: slow, AreaDirectory: slow, AreaKeyExpiry: slow,
		AreaSnapshots: 10 * time.Minute,
		AreaUpdates:   4 * time.Hour,
	}
}

// Answer is the last good answer to one question.
type Answer[T any] struct {
	Value T
	// At is when it was given, and the zero time for a question never answered.
	At time.Time
}

// Known says whether there is an answer to serve.
func (a Answer[T]) Known() bool { return !a.At.IsZero() }

// RoundState is how the last poll of one area went.
type RoundState struct {
	// Interval is how often the area is asked about.
	Interval time.Duration
	// Ran says a poll has finished since the start; until one has, the rest says nothing.
	Ran bool
	// OK says every question of the last poll was answered.
	OK   bool
	Took time.Duration
	// LastSuccess is the zero time until a poll succeeds.
	LastSuccess time.Time
	// Failures counts the polls that failed since the start.
	Failures int
}

// State is what is known of the NAS: the last good answer to each question, and how the last poll of each area went.
type State struct {
	System    Answer[System]
	Pools     Answer[[]Pool]
	BootPool  Answer[Pool]
	Disks     Answer[[]Disk]
	Datasets  Answer[[]Dataset]
	Snapshots Answer[[]SnapshotCount]
	Alerts    Answer[[]Alert]
	Services  Answer[[]Service]
	Jobs      Answer[[]Job]
	Apps      Answer[Apps]
	VMs       Answer[[]VM]
	Shares    Answer[[]Share]
	Clients   Answer[[]Clients]
	Tasks     Answer[[]Task]

	Update       Answer[Update]
	Interfaces   Answer[[]Interface]
	Certificates Answer[[]Certificate]
	Directory    Answer[Directory]
	Key          Answer[Key]

	// Rounds is how the last poll of each area asked about went. An area left out has none.
	Rounds map[Area]RoundState
}

// Up says whether the NAS answered the last time it was asked about its own state. One area failing says a read went wrong, not that the NAS is gone.
func (s *State) Up() bool { return s.Rounds[AreaSystem].OK }

// Answered says whether the last poll of every area asked about was answered in full.
func (s *State) Answered() bool {
	for _, r := range s.Rounds {
		if !r.OK {
			return false
		}
	}

	return true
}

// question asks the NAS one thing and keeps its answer.
type question struct {
	ask func(ctx context.Context, p *Poller) error
	// fresh forgets the answer once it is older than limit.
	fresh func(s *State, now time.Time, limit time.Duration)
}

// asking is the question method puts, its answer kept in the place into names. An answer is stored whole and never changed afterwards, so State can
// hand it out without copying.
func asking[T any](what string, method func(NAS, context.Context) (T, error), into func(*State) *Answer[T]) question {
	return question{
		ask: func(ctx context.Context, p *Poller) error {
			value, err := method(p.nas, ctx)
			if err != nil {
				return fmt.Errorf("%s: %w", what, err)
			}

			p.mu.Lock()
			defer p.mu.Unlock()

			*into(&p.state) = Answer[T]{Value: value, At: time.Now()}

			return nil
		},
		fresh: func(s *State, now time.Time, limit time.Duration) {
			if a := into(s); a.Known() && now.Sub(a.At) > limit {
				*a = Answer[T]{}
			}
		},
	}
}

// questions is what is asked of the NAS about each area.
var questions = map[Area][]question{
	AreaSystem:       {asking("system", NAS.System, func(s *State) *Answer[System] { return &s.System })},
	AreaPools:        {asking("pools", NAS.Pools, func(s *State) *Answer[[]Pool] { return &s.Pools }), asking("boot pool", NAS.BootPool, func(s *State) *Answer[Pool] { return &s.BootPool })},
	AreaDisks:        {asking("disks", NAS.Disks, func(s *State) *Answer[[]Disk] { return &s.Disks })},
	AreaDatasets:     {asking("datasets", NAS.Datasets, func(s *State) *Answer[[]Dataset] { return &s.Datasets })},
	AreaSnapshots:    {asking("snapshots", NAS.Snapshots, func(s *State) *Answer[[]SnapshotCount] { return &s.Snapshots })},
	AreaAlerts:       {asking("alerts", NAS.Alerts, func(s *State) *Answer[[]Alert] { return &s.Alerts })},
	AreaServices:     {asking("services", NAS.Services, func(s *State) *Answer[[]Service] { return &s.Services })},
	AreaJobs:         {asking("jobs", NAS.Jobs, func(s *State) *Answer[[]Job] { return &s.Jobs })},
	AreaApps:         {asking("apps", NAS.Apps, func(s *State) *Answer[Apps] { return &s.Apps })},
	AreaVMs:          {asking("virtual machines", NAS.VMs, func(s *State) *Answer[[]VM] { return &s.VMs })},
	AreaShares:       {asking("shares", NAS.Shares, func(s *State) *Answer[[]Share] { return &s.Shares })},
	AreaClients:      {asking("clients", NAS.Clients, func(s *State) *Answer[[]Clients] { return &s.Clients })},
	AreaTasks:        {asking("tasks", NAS.Tasks, func(s *State) *Answer[[]Task] { return &s.Tasks })},
	AreaUpdates:      {asking("update", NAS.Update, func(s *State) *Answer[Update] { return &s.Update })},
	AreaInterfaces:   {asking("interfaces", NAS.Interfaces, func(s *State) *Answer[[]Interface] { return &s.Interfaces })},
	AreaCertificates: {asking("certificates", NAS.Certificates, func(s *State) *Answer[[]Certificate] { return &s.Certificates })},
	AreaDirectory:    {asking("directory service", NAS.Directory, func(s *State) *Answer[Directory] { return &s.Directory })},
	AreaKeyExpiry:    {asking("API key", NAS.Key, func(s *State) *Answer[Key] { return &s.Key })},
}

// Poller asks one NAS about each area on that area's own schedule, and remembers the last good answer to each question.
type Poller struct {
	nas   NAS
	every Intervals
	// areas is the areas asked about, in the order of Every.
	areas []Area

	mu    sync.RWMutex
	state State
}

// New is a poller for one NAS that asks about every area but those in skip, each as often as every says. It asks nothing until Run or Poll.
func New(nas NAS, every Intervals, skip ...Area) (*Poller, error) {
	areas := slices.DeleteFunc(slices.Clone(Every), func(a Area) bool { return a != AreaSystem && slices.Contains(skip, a) })

	rounds := map[Area]RoundState{}
	for _, area := range areas {
		if every[area] <= 0 {
			return nil, fmt.Errorf("%s has to be asked about every so often, and not every %s", area, every[area])
		}

		rounds[area] = RoundState{Interval: every[area]}
	}

	return &Poller{nas: nas, every: maps.Clone(every), areas: areas, state: State{Rounds: rounds}}, nil
}

// Run polls until the context ends: every area at once on start, then each on its own interval.
func (p *Poller) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, area := range p.areas {
		wg.Go(func() { p.loop(ctx, area) })
	}
	wg.Wait()
}

// PollAll polls every area once, all at once, as Run does on start.
func (p *Poller) PollAll(ctx context.Context) {
	var wg sync.WaitGroup
	for _, area := range p.areas {
		wg.Go(func() { p.Poll(ctx, area) })
	}
	wg.Wait()
}

// loop polls one area now and then every interval, until the context ends.
func (p *Poller) loop(ctx context.Context, area Area) {
	ticker := time.NewTicker(p.every[area])
	defer ticker.Stop()

	for {
		p.Poll(ctx, area)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Poll asks about one area now and records how it went. A question that fails leaves its last good answer where it was, and does not stop the
// others being asked. An area left out is not asked about.
func (p *Poller) Poll(ctx context.Context, area Area) {
	if !slices.Contains(p.areas, area) {
		return
	}

	within, cancel := context.WithTimeout(ctx, p.every[area])
	defer cancel()

	start := time.Now()
	err := p.ask(within, area)
	// a poll cut short because the program is stopping says nothing about the NAS
	if err != nil && ctx.Err() != nil {
		return
	}

	p.report(area, p.record(area, time.Since(start), err), err)
}

// ask puts every question about one area at once, and gives back what failed. The NAS answers them over one connection, as its own pages do.
func (p *Poller) ask(ctx context.Context, area Area) error {
	failed := make([]error, len(questions[area]))

	var wg sync.WaitGroup
	for i, q := range questions[area] {
		wg.Go(func() { failed[i] = q.ask(ctx, p) })
	}
	wg.Wait()

	return errors.Join(failed...)
}

// record keeps how a poll went, and gives back how the one before it did.
func (p *Poller) record(area Area, took time.Duration, err error) RoundState {
	p.mu.Lock()
	defer p.mu.Unlock()

	r := p.state.Rounds[area]
	before := r
	r.Ran, r.OK, r.Took = true, err == nil, took
	if err != nil {
		r.Failures++
	} else {
		r.LastSuccess = time.Now()
	}
	p.state.Rounds[area] = r

	return before
}

// report logs a poll only when it changes, since a NAS that is down fails every poll the same way. Of the polls that go well, only the first of the
// NAS's own state is logged: it says the NAS is there.
func (p *Poller) report(area Area, before RoundState, err error) {
	switch {
	case err != nil && (before.OK || !before.Ran):
		clog.Log.Warnf("asking about %s failed, and will be tried again every %s: %s", area, p.every[area], said(err))
	case err != nil:
		clog.Log.Debugf("asking about %s failed again: %s", area, said(err))
	case before.Ran && !before.OK:
		clog.Log.Infof("%s is answered again", area)
	case !before.Ran && area == AreaSystem:
		clog.Log.Infof("the NAS answered")
	}
}

// said is what went wrong as one line of a log: a poll's failures come one to a line, and a NAS's own words for one may quote an address with a key
// on it.
func said(err error) string {
	return strings.ReplaceAll(outside.BlankAddresses(err.Error()), "\n", "; ")
}

// State is what is known of the NAS now, without asking it anything. An answer older than three of its area's intervals is left out: the NAS has
// stopped answering, and what it last said is no longer worth showing as current.
func (p *Poller) State() State {
	p.mu.RLock()
	defer p.mu.RUnlock()

	s := p.state
	s.Rounds = maps.Clone(p.state.Rounds)

	now := time.Now()
	for _, area := range p.areas {
		for _, q := range questions[area] {
			q.fresh(&s, now, staleAfter*p.every[area])
		}
	}

	return s
}
