// Package poll asks a TrueNAS for its state on a schedule and keeps the last good answers, so the metrics page is served from memory and a slow or
// stopped NAS never holds up a scrape.
//
// The types here are the exporter's own words for what a NAS says, not the API's: only what the page reports has a field. Nothing says who is
// connected or whose a share is: there is no field for a user's name, a client's address or a path, so none of them can reach a label.
package poll

import (
	"context"
	"strings"
	"time"
)

// NAS is the questions asked of one TrueNAS. Every one of them only reads.
type NAS interface {
	// what changes by the minute
	System(ctx context.Context) (System, error)
	Pools(ctx context.Context) ([]Pool, error)
	// BootPool is the pool the system itself runs from, which the NAS keeps apart from the rest.
	BootPool(ctx context.Context) (Pool, error)
	Alerts(ctx context.Context) ([]Alert, error)
	Services(ctx context.Context) ([]Service, error)
	// Jobs is the jobs running or waiting to run, not the ones that are over.
	Jobs(ctx context.Context) ([]Job, error)
	Apps(ctx context.Context) (Apps, error)
	VMs(ctx context.Context) ([]VM, error)
	Clients(ctx context.Context) ([]Clients, error)

	// what changes when somebody changes it, or a task runs
	Disks(ctx context.Context) ([]Disk, error)
	Datasets(ctx context.Context) ([]Dataset, error)
	// Snapshots is how many snapshots each dataset has, which the NAS has to go through every one of them to say.
	Snapshots(ctx context.Context) ([]SnapshotCount, error)
	Tasks(ctx context.Context) ([]Task, error)
	Shares(ctx context.Context) ([]Share, error)
	Certificates(ctx context.Context) ([]Certificate, error)
	Interfaces(ctx context.Context) ([]Interface, error)
	Update(ctx context.Context) (Update, error)
	Directory(ctx context.Context) (Directory, error)
	// Key is the API key the questions are asked with.
	Key(ctx context.Context) (Key, error)
}

// System is what a NAS says of itself.
type System struct {
	Version      string
	Product      string
	Manufacturer string
	CPU          string
	// State is READY once the NAS has started, BOOTING before and SHUTTING_DOWN after.
	State  string
	Booted time.Time
	Uptime time.Duration
	// Memory is in bytes.
	Memory        int64
	Cores         int
	PhysicalCores int
	// Load is the load average over the last 1, 5 and 15 minutes.
	Load [3]float64
	ECC  bool
	// ClockOffset is the NAS's clock less this machine's at the moment it was asked.
	ClockOffset time.Duration
}

// SystemStates is every state a NAS can be in, in the order the page lists them.
var SystemStates = []string{"BOOTING", "READY", "SHUTTING_DOWN"}

// PoolStatuses is every status a pool, a vdev or a disk in one can have, in the order the page lists them.
var PoolStatuses = []string{"ONLINE", "DEGRADED", "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED"}

// Pool is one pool, the boot pool included.
type Pool struct {
	Name   string
	Status string
	// Healthy is the NAS's own word on the pool, and Warning its word that something wants looking at though the pool is healthy.
	Healthy bool
	Warning bool
	// Size, Allocated, Free and Freeing are in bytes.
	Size      int64
	Allocated int64
	Free      int64
	Freeing   int64
	// Fragmentation is from 0 to 1, and nil when the pool does not say.
	Fragmentation *float64
	// Scan is the last scrub or resilver, running or over; nil for a pool never scanned.
	Scan  *Scan
	Vdevs []Vdev
}

// The work a scan does.
const (
	Scrub    = "SCRUB"
	Resilver = "RESILVER"
)

// ScanFunctions is both kinds of scan, in the order the page lists them.
var ScanFunctions = []string{Scrub, Resilver}

// ScanRunning is the state of a scan under way.
const ScanRunning = "SCANNING"

// Scan is a pool's last scrub or resilver.
type Scan struct {
	Function string
	// State is SCANNING while it runs, then FINISHED or CANCELED.
	State   string
	Started time.Time
	// Ended is the zero time while it runs.
	Ended time.Time
	// Progress is from 0 to 1.
	Progress float64
	// ToExamine, Examined and Issued are in bytes.
	ToExamine int64
	Examined  int64
	Issued    int64
	Errors    int64
	Paused    bool
	// Left is how long the NAS expects it to go on, and nil when it does not say.
	Left *time.Duration
}

// VdevClasses is every part a pool's vdevs are sorted into, in the order the page lists them.
var VdevClasses = []string{"data", "log", "cache", "spare", "special", "dedup"}

// Errors is how many times reading, writing and checking the data of a vdev or a disk has gone wrong since the counts were last cleared.
type Errors struct {
	Read     int64
	Write    int64
	Checksum int64
}

// Vdev is one of a pool's top-level vdevs: a raidz or mirror of several disks, or a disk by itself.
type Vdev struct {
	// Class is which part of the pool it belongs to: data, log, cache, spare, special or dedup.
	Class string
	// Name is as zpool status lists it, raidz2-0 say; a vdev of one disk has that disk's name.
	Name   string
	Type   string
	Status string
	// Size and Allocated are in bytes, and 0 for a vdev that holds no data of its own: a spare.
	Size      int64
	Allocated int64
	// Fragmentation is from 0 to 1.
	Fragmentation float64
	Errors        Errors
	// Devices is the disks it is made of; a vdev of one disk holds that disk.
	Devices []Device
}

// Device is one disk as a pool sees it.
type Device struct {
	// Disk is the disk's name, sda say. For a disk the NAS can no longer find it is the number the pool knows the disk by, so that two missing
	// disks are still told apart.
	Disk   string
	Status string
	Errors Errors
}

// AlertLevels is every level an alert can have, least serious first.
var AlertLevels = []string{"INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL", "ALERT", "EMERGENCY"}

// Alert is one alert the NAS is showing.
type Alert struct {
	Level string
	// Class is the kind of alert, RsyncFailed say. It is never the alert's text, which names paths and hosts.
	Class     string
	Dismissed bool
}

// Service is one of the NAS's services: SMB, NFS, SSH and the rest.
type Service struct {
	Name    string
	Running bool
	// Enabled says it starts with the NAS.
	Enabled bool
}

// JobRunning is the state of a job under way.
const JobRunning = "RUNNING"

// JobStates is the states of a job that is not over, in the order the page lists them.
var JobStates = []string{JobRunning, "WAITING"}

// Job is one job running or waiting to run.
type Job struct {
	// Method is what the job does, cloudsync.sync say.
	Method string
	State  string
}

// AppStates is every state an app can be in, in the order the page lists them.
var AppStates = []string{"RUNNING", "DEPLOYING", "STOPPING", "STOPPED", "CRASHED"}

// Apps is the apps installed on the NAS and the service that runs them.
type Apps struct {
	// Status is the state of the service that runs apps: RUNNING, STOPPED, FAILED, or UNCONFIGURED on a NAS never set up for them.
	Status string
	List   []App
}

// App is one installed app.
type App struct {
	Name            string
	State           string
	UpdateAvailable bool
}

// VMStates is every state a virtual machine can be in, in the order the page lists them.
var VMStates = []string{"RUNNING", "STOPPED", "SUSPENDED"}

// VM is one virtual machine.
type VM struct {
	Name  string
	State string
}

// Clients is how many clients are connected over one protocol. It is never which.
type Clients struct {
	// Protocol is nfs or iscsi; the NAS gives no count for SMB.
	Protocol string
	Count    int
}

// Disk is one disk the NAS knows of, in a pool or not.
type Disk struct {
	// Name is the kernel's, sda say, which can change from one start to the next; Serial is what stays.
	Name   string
	Serial string
	// WWN is the disk's world wide name as the NAS writes it: hex with nothing in front.
	WWN   string
	Model string
	// Type is HDD or SSD.
	Type string
	Bus  string
	// Size is in bytes.
	Size int64
	// Temperature is in degrees Celsius, and nil for a disk that does not say.
	Temperature *float64
	// Slot is where the disk sits, and nil where the NAS does not know its enclosure: it knows its maker's own and little else.
	Slot *Slot
}

// Slot is a disk's place in an enclosure, by the NAS's own numbering.
type Slot struct {
	Enclosure int
	Slot      int
}

// Dataset is one filesystem or volume.
type Dataset struct {
	// Name is the whole name, pool included: tank/media.
	Name string
	Pool string
	// Type is FILESYSTEM or VOLUME.
	Type string
	// Used is everything the dataset takes up, children and snapshots included, and Available what it may still grow by, both in bytes.
	Used      int64
	Available int64
	// UsedByDataset is its own data alone and UsedBySnapshots what its snapshots alone hold on to, both in bytes.
	UsedByDataset   int64
	UsedBySnapshots int64
	// Quota and RefQuota are in bytes, and nil where none is set.
	Quota    *int64
	RefQuota *int64
	// Compression is how many times smaller the data is stored than it is: 1.5 for a third saved.
	Compression float64
}

// Depth is how far below its pool a dataset is: 0 for the pool's own, 1 for tank/media.
func (d *Dataset) Depth() int { return depth(d.Name) }

// SnapshotCount is how many snapshots one dataset has of itself, its children's not counted.
type SnapshotCount struct {
	// Dataset is the dataset's whole name, pool included: tank/media.
	Dataset string
	Pool    string
	Count   int
}

// Depth is how far below its pool the dataset is, as for Dataset.
func (c *SnapshotCount) Depth() int { return depth(c.Dataset) }

// depth is how far below its pool the dataset of that name is.
func depth(name string) int {
	return strings.Count(name, "/")
}

// TaskKind is which kind of scheduled task a task is.
type TaskKind string

const (
	SnapshotTask    TaskKind = "snapshot"
	ReplicationTask TaskKind = "replication"
	CloudSyncTask   TaskKind = "cloud_sync"
	CloudBackupTask TaskKind = "cloud_backup"
	RsyncTask       TaskKind = "rsync"
	// ScrubTask is a pool's scrub schedule. How its last run went is the pool's own scan, so it has no run of its own.
	ScrubTask TaskKind = "scrub"
)

// TaskKinds is every kind, in the order the page lists them.
var TaskKinds = []TaskKind{SnapshotTask, ReplicationTask, CloudSyncTask, CloudBackupTask, RsyncTask, ScrubTask}

// Task is one scheduled task.
type Task struct {
	Kind TaskKind
	// ID is the task's number, which with its kind says which task it is.
	ID int
	// Name is what whoever made the task called it, or the dataset or pool it works on where a task has no name of its own.
	Name    string
	Enabled bool
	Running bool
	// Progress is how far a running task says it has got, from 0 to 1.
	Progress float64
	// LastRun is nil for a task that has never run, or whose kind keeps no record of its runs.
	LastRun *Run
}

// Run is how a task's last run went.
type Run struct {
	// OK says it finished as it should, and Failed that it went wrong. A run that was stopped by hand is neither.
	OK     bool
	Failed bool
	// Ended is the zero time for a run the NAS gives no end for.
	Ended time.Time
	// Took is 0 for a kind of task the NAS does not time.
	Took time.Duration
}

// ShareProtocols is every way a share is offered, in the order the page lists them.
var ShareProtocols = []string{"smb", "nfs", "iscsi"}

// Share is one share, less everything that says what it is called or whose it is.
type Share struct {
	Protocol string
	Enabled  bool
	// Locked says the share's dataset is encrypted and its key not loaded, so nothing can be read through it.
	Locked bool
}

// Certificate is one certificate the NAS holds.
type Certificate struct {
	Name    string
	Expires time.Time
}

// Interface is one network interface.
type Interface struct {
	Name string
	// Type is PHYSICAL, BRIDGE, LINK_AGGREGATION or VLAN.
	Type   string
	LinkUp bool
	// MTU is in bytes.
	MTU int
}

// Update is what the NAS knows of newer versions of itself.
type Update struct {
	Available bool
	// CheckFailed says the NAS could not find out, which is not the same as there being none.
	CheckFailed bool
}

// Directory is the directory service the NAS is joined to, Active Directory say.
type Directory struct {
	// Type is empty for a NAS joined to none.
	Type string
	// Status is HEALTHY, FAULTED, JOINING, LEAVING or DISABLED.
	Status string
}

// DirectoryStatuses is every status a directory service can have, in the order the page lists them.
var DirectoryStatuses = []string{"HEALTHY", "FAULTED", "JOINING", "LEAVING", "DISABLED"}

// Key is the API key the questions are asked with.
type Key struct {
	// Expires is when the NAS will stop taking it, and the zero time for a key that never expires.
	Expires time.Time
}
