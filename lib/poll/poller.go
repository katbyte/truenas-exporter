package poll

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/katbyte/go-kt/prometheus/poller"
	"github.com/prometheus/client_golang/prometheus"
)

// Namespace starts the name of every reading on the page, the poller's own of how the asking goes among them.
const Namespace = "truenas"

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

// State is what is known of the NAS: the last good answer to each question, and how the last poll of each area went.
type State struct {
	System    poller.Answer[System]
	Pools     poller.Answer[[]Pool]
	BootPool  poller.Answer[Pool]
	Disks     poller.Answer[[]Disk]
	Datasets  poller.Answer[[]Dataset]
	Snapshots poller.Answer[[]SnapshotCount]
	Alerts    poller.Answer[[]Alert]
	Services  poller.Answer[[]Service]
	Jobs      poller.Answer[[]Job]
	Apps      poller.Answer[Apps]
	VMs       poller.Answer[[]VM]
	Shares    poller.Answer[[]Share]
	Clients   poller.Answer[[]Clients]
	Tasks     poller.Answer[[]Task]

	Update       poller.Answer[Update]
	Interfaces   poller.Answer[[]Interface]
	Certificates poller.Answer[[]Certificate]
	Directory    poller.Answer[Directory]
	Key          poller.Answer[Key]

	// Rounds is how the last poll of each area asked about went. An area left out has none.
	Rounds map[Area]poller.Round
}

// Up says whether the NAS answered the last time it was asked about its own state. One area failing says a read went wrong, not that the NAS is gone.
func (s *State) Up() bool { return s.Rounds[AreaSystem].OK }

// Poller asks one NAS about each area on that area's own schedule, keeps the last good answer to each question, and writes how the asking goes as
// readings of its own: truenas_poll_duration_seconds and the rest.
type Poller struct {
	asks *poller.Poller[struct{}]
	// areas is the areas asked about, in the order of Every.
	areas []Area

	system       *poller.Kept[System]
	pools        *poller.Kept[[]Pool]
	bootPool     *poller.Kept[Pool]
	disks        *poller.Kept[[]Disk]
	datasets     *poller.Kept[[]Dataset]
	snapshots    *poller.Kept[[]SnapshotCount]
	alerts       *poller.Kept[[]Alert]
	services     *poller.Kept[[]Service]
	jobs         *poller.Kept[[]Job]
	apps         *poller.Kept[Apps]
	vms          *poller.Kept[[]VM]
	shares       *poller.Kept[[]Share]
	clients      *poller.Kept[[]Clients]
	tasks        *poller.Kept[[]Task]
	update       *poller.Kept[Update]
	interfaces   *poller.Kept[[]Interface]
	certificates *poller.Kept[[]Certificate]
	directory    *poller.Kept[Directory]
	key          *poller.Kept[Key]
}

// New is a poller for one NAS that asks about every area but those in skip, each as often as every says. It asks nothing until Run or Poll.
func New(nas NAS, every Intervals, skip ...Area) (*Poller, error) {
	asks, err := poller.New(poller.Options[struct{}]{Namespace: Namespace, Kind: "NAS"})
	if err != nil {
		return nil, err
	}

	p := &Poller{asks: asks, areas: slices.DeleteFunc(slices.Clone(Every), func(a Area) bool { return a != AreaSystem && slices.Contains(skip, a) })}
	for _, area := range p.areas {
		if every[area] <= 0 {
			return nil, fmt.Errorf("%s has to be asked about every so often, and not every %s", area, every[area])
		}

		p.ask(nas, area, asks.Schedule(string(area), every[area]))
	}

	return p, nil
}

// ask puts the questions about one area on its schedule. They are put at once, and the NAS answers them over one connection, as its own pages do.
func (p *Poller) ask(nas NAS, area Area, s *poller.Schedule[struct{}]) {
	switch area {
	case AreaSystem:
		p.system = poller.AskOne(s, "system", nas.System)
	case AreaPools:
		p.pools = poller.AskOne(s, "pools", nas.Pools)
		p.bootPool = poller.AskOne(s, "boot pool", nas.BootPool)
	case AreaDisks:
		p.disks = poller.AskOne(s, "disks", nas.Disks)
	case AreaDatasets:
		p.datasets = poller.AskOne(s, "datasets", nas.Datasets)
	case AreaSnapshots:
		p.snapshots = poller.AskOne(s, "snapshots", nas.Snapshots)
	case AreaAlerts:
		p.alerts = poller.AskOne(s, "alerts", nas.Alerts)
	case AreaServices:
		p.services = poller.AskOne(s, "services", nas.Services)
	case AreaJobs:
		p.jobs = poller.AskOne(s, "jobs", nas.Jobs)
	case AreaApps:
		p.apps = poller.AskOne(s, "apps", nas.Apps)
	case AreaVMs:
		p.vms = poller.AskOne(s, "virtual machines", nas.VMs)
	case AreaShares:
		p.shares = poller.AskOne(s, "shares", nas.Shares)
	case AreaClients:
		p.clients = poller.AskOne(s, "clients", nas.Clients)
	case AreaTasks:
		p.tasks = poller.AskOne(s, "tasks", nas.Tasks)
	case AreaUpdates:
		p.update = poller.AskOne(s, "update", nas.Update)
	case AreaInterfaces:
		p.interfaces = poller.AskOne(s, "interfaces", nas.Interfaces)
	case AreaCertificates:
		p.certificates = poller.AskOne(s, "certificates", nas.Certificates)
	case AreaDirectory:
		p.directory = poller.AskOne(s, "directory service", nas.Directory)
	case AreaKeyExpiry:
		p.key = poller.AskOne(s, "API key", nas.Key)
	}
}

// Run polls until the context ends: every area at once on start, then each on its own interval.
func (p *Poller) Run(ctx context.Context) { p.asks.Run(ctx) }

// PollAll polls every area once, all at once, as Run does on start, and says which failed. Why each did is in the log.
func (p *Poller) PollAll(ctx context.Context) error { return p.asks.PollAll(ctx) }

// Poll asks about one area now, records how it went, and says what failed and why. A question that fails leaves its last good answer where it was,
// and does not stop the others being asked. An area left out is not asked about.
func (p *Poller) Poll(ctx context.Context, area Area) error { return p.asks.Poll(ctx, string(area)) }

// Describe names the poller's readings of how the asking goes.
func (p *Poller) Describe(ch chan<- *prometheus.Desc) { p.asks.Describe(ch) }

// Collect writes how the asking goes: how often each area is asked about, how long its last poll took, how many have failed and when one last
// succeeded.
func (p *Poller) Collect(ch chan<- prometheus.Metric) { p.asks.Collect(ch) }

// State is what is known of the NAS now, without asking it anything. An answer older than three of its area's intervals is left out: the NAS has
// stopped answering, and what it last said is no longer worth showing as current. An area left out has no answer.
func (p *Poller) State() State {
	rounds := make(map[Area]poller.Round, len(p.areas))
	for _, area := range p.areas {
		rounds[area] = p.asks.Round("", string(area))
	}

	return State{
		System: p.system.Answer(), Pools: p.pools.Answer(), BootPool: p.bootPool.Answer(), Disks: p.disks.Answer(), Datasets: p.datasets.Answer(), Snapshots: p.snapshots.Answer(),
		Alerts: p.alerts.Answer(), Services: p.services.Answer(), Jobs: p.jobs.Answer(), Apps: p.apps.Answer(), VMs: p.vms.Answer(), Shares: p.shares.Answer(),
		Clients: p.clients.Answer(), Tasks: p.tasks.Answer(), Update: p.update.Answer(), Interfaces: p.interfaces.Answer(), Certificates: p.certificates.Answer(),
		Directory: p.directory.Answer(), Key: p.key.Answer(), Rounds: rounds,
	}
}
