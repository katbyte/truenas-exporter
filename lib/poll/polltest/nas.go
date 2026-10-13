// Package polltest is a TrueNAS for tests: one that answers what it is told to, fails when told to, and counts what it was asked.
package polltest

import (
	"context"
	"sync"
	"time"

	"github.com/katbyte/truenas-metrics/lib/poll"
)

// The questions a NAS is asked, as Fail and Asked name them.
const (
	System       = "system"
	Pools        = "pools"
	BootPool     = "boot pool"
	Alerts       = "alerts"
	Services     = "services"
	Jobs         = "jobs"
	Apps         = "apps"
	VMs          = "vms"
	Clients      = "clients"
	Disks        = "disks"
	Datasets     = "datasets"
	Tasks        = "tasks"
	Shares       = "shares"
	Certificates = "certificates"
	Interfaces   = "interfaces"
	Update       = "update"
	Directory    = "directory"
	Key          = "key"
)

// Fast and Slow are the questions of each round.
var (
	Fast = []string{System, Pools, BootPool, Alerts, Services, Jobs, Apps, VMs, Clients}
	Slow = []string{Disks, Datasets, Tasks, Shares, Certificates, Interfaces, Update, Directory, Key}
)

// NAS answers each question with the value set for it. The zero value is a NAS with nothing on it; set the fields before it is polled.
type NAS struct {
	SystemAnswer       poll.System
	PoolsAnswer        []poll.Pool
	BootPoolAnswer     poll.Pool
	AlertsAnswer       []poll.Alert
	ServicesAnswer     []poll.Service
	JobsAnswer         []poll.Job
	AppsAnswer         poll.Apps
	VMsAnswer          []poll.VM
	ClientsAnswer      []poll.Clients
	DisksAnswer        []poll.Disk
	DatasetsAnswer     []poll.Dataset
	TasksAnswer        []poll.Task
	SharesAnswer       []poll.Share
	CertificatesAnswer []poll.Certificate
	InterfacesAnswer   []poll.Interface
	UpdateAnswer       poll.Update
	DirectoryAnswer    poll.Directory
	KeyAnswer          poll.Key

	// Slow is how long every question takes to answer, or until the context ends if that is sooner.
	Slow time.Duration

	mu      sync.Mutex
	failing map[string]error
	asked   map[string]int
}

// Fail makes one question fail with err from now on, and every question when question is "". A nil err puts it right again.
func (n *NAS) Fail(question string, err error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.failing == nil {
		n.failing = map[string]error{}
	}
	n.failing[question] = err
}

// Asked is how many times a question has been put.
func (n *NAS) Asked(question string) int {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.asked[question]
}

// answer counts the question, takes as long as Slow says, and gives the error it was told to fail with.
func (n *NAS) answer(ctx context.Context, question string) error {
	n.mu.Lock()
	if n.asked == nil {
		n.asked = map[string]int{}
	}
	n.asked[question]++
	err := n.failing[question]
	if err == nil {
		err = n.failing[""]
	}
	n.mu.Unlock()

	if n.Slow > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(n.Slow):
		}
	}

	return err
}

func (n *NAS) System(ctx context.Context) (poll.System, error) {
	return n.SystemAnswer, n.answer(ctx, System)
}

func (n *NAS) Pools(ctx context.Context) ([]poll.Pool, error) {
	return n.PoolsAnswer, n.answer(ctx, Pools)
}

func (n *NAS) BootPool(ctx context.Context) (poll.Pool, error) {
	return n.BootPoolAnswer, n.answer(ctx, BootPool)
}

func (n *NAS) Alerts(ctx context.Context) ([]poll.Alert, error) {
	return n.AlertsAnswer, n.answer(ctx, Alerts)
}

func (n *NAS) Services(ctx context.Context) ([]poll.Service, error) {
	return n.ServicesAnswer, n.answer(ctx, Services)
}

func (n *NAS) Jobs(ctx context.Context) ([]poll.Job, error) {
	return n.JobsAnswer, n.answer(ctx, Jobs)
}

func (n *NAS) Apps(ctx context.Context) (poll.Apps, error) {
	return n.AppsAnswer, n.answer(ctx, Apps)
}

func (n *NAS) VMs(ctx context.Context) ([]poll.VM, error) {
	return n.VMsAnswer, n.answer(ctx, VMs)
}

func (n *NAS) Clients(ctx context.Context) ([]poll.Clients, error) {
	return n.ClientsAnswer, n.answer(ctx, Clients)
}

func (n *NAS) Disks(ctx context.Context) ([]poll.Disk, error) {
	return n.DisksAnswer, n.answer(ctx, Disks)
}

func (n *NAS) Datasets(ctx context.Context) ([]poll.Dataset, error) {
	return n.DatasetsAnswer, n.answer(ctx, Datasets)
}

func (n *NAS) Tasks(ctx context.Context) ([]poll.Task, error) {
	return n.TasksAnswer, n.answer(ctx, Tasks)
}

func (n *NAS) Shares(ctx context.Context) ([]poll.Share, error) {
	return n.SharesAnswer, n.answer(ctx, Shares)
}

func (n *NAS) Certificates(ctx context.Context) ([]poll.Certificate, error) {
	return n.CertificatesAnswer, n.answer(ctx, Certificates)
}

func (n *NAS) Interfaces(ctx context.Context) ([]poll.Interface, error) {
	return n.InterfacesAnswer, n.answer(ctx, Interfaces)
}

func (n *NAS) Update(ctx context.Context) (poll.Update, error) {
	return n.UpdateAnswer, n.answer(ctx, Update)
}

func (n *NAS) Directory(ctx context.Context) (poll.Directory, error) {
	return n.DirectoryAnswer, n.answer(ctx, Directory)
}

func (n *NAS) Key(ctx context.Context) (poll.Key, error) {
	return n.KeyAnswer, n.answer(ctx, Key)
}
