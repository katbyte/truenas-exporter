// Package readings is what the poller knows of the NAS, as Prometheus readings for a metrics page to hold.
package readings

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/katbyte/truenas-metrics/lib/poll"
	"github.com/prometheus/client_golang/prometheus"
)

// Namespace starts every reading's name.
const Namespace = "truenas"

// DefaultDatasetDepth reports each pool's own dataset and the datasets directly under it.
const DefaultDatasetDepth = 1

// all is every reading the page can hold, for Describe.
var all []*prometheus.Desc

// reading describes one reading of the NAS.
func reading(name, help string, labels ...string) *prometheus.Desc {
	d := prometheus.NewDesc(Namespace+"_"+name, help, labels, nil)
	all = append(all, d)

	return d
}

var (
	up              = reading("up", "Whether the NAS answered the last time it was asked about its own state (1) or not (0).")
	pollInterval    = reading("poll_interval_seconds", "How often each part of the NAS is asked about.", "poll")
	pollDuration    = reading("poll_duration_seconds", "How long the last poll of each part of the NAS took.", "poll")
	pollLastSuccess = reading("poll_last_success_timestamp_seconds", "When each part of the NAS was last answered in full, in seconds since 1970.", "poll")
	pollFailures    = reading("poll_failures_total", "How many polls of each part of the NAS have failed since truenas-metrics started.", "poll")

	systemInfo        = reading("system_info", "Always 1, with the version of TrueNAS and the machine it runs on.", "version", "product", "manufacturer", "cpu")
	systemState       = reading("system_state", "Whether the NAS is in each state (1) or not (0): BOOTING, READY or SHUTTING_DOWN.", "state")
	systemBoot        = reading("system_boot_timestamp_seconds", "When the NAS last started, in seconds since 1970.")
	systemUptime      = reading("system_uptime_seconds", "How long the NAS had been running when it was last asked.")
	systemMemory      = reading("system_memory_bytes", "How much memory the NAS has.")
	systemCores       = reading("system_cpu_cores", "How many processor cores the NAS has, counting each thread of a core as one.")
	systemPhysical    = reading("system_cpu_physical_cores", "How many processor cores the NAS has, threads not counted.")
	systemLoad        = reading("system_load_average", "The load average of the NAS over the window: 1m, 5m or 15m.", "window")
	systemECC         = reading("system_ecc_memory", "Whether the NAS's memory corrects its own errors (1) or not (0).")
	systemClockOffset = reading("system_clock_offset_seconds", "The NAS's clock less the clock of the machine running truenas-metrics, when it was last asked.")

	updateAvailable   = reading("update_available", "Whether the NAS says a newer version of TrueNAS is available (1) or not (0).")
	updateCheckFailed = reading("update_check_failed", "Whether the NAS could not find out if there is a newer version (1) or could (0).")

	poolHealthy       = reading("pool_healthy", "Whether the NAS calls the pool healthy (1) or not (0). The boot pool is one of them.", "pool")
	poolWarning       = reading("pool_warning", "Whether the NAS has a warning about the pool (1) or not (0).", "pool")
	poolStatus        = reading("pool_status", "Always 1, with the pool's status: ONLINE, DEGRADED, FAULTED, OFFLINE, UNAVAIL or REMOVED.", "pool", "status")
	poolSize          = reading("pool_size_bytes", "The size of the pool, before what parity and mirrors take.", "pool")
	poolAllocated     = reading("pool_allocated_bytes", "How much of the pool is written to, parity and mirrors included.", "pool")
	poolFree          = reading("pool_free_bytes", "How much of the pool is not written to.", "pool")
	poolFreeing       = reading("pool_freeing_bytes", "How much of the pool is still to be given back after something was deleted.", "pool")
	poolFragmentation = reading("pool_fragmentation_ratio", "How broken up the pool's free space is, from 0 to 1.", "pool")

	scanRunning   = reading("pool_scan_running", "Whether a scan of the pool is under way (1) or not (0), by what it does: SCRUB or RESILVER.", "pool", "function")
	scanPaused    = reading("pool_scan_paused", "Whether the pool's last scan is paused (1) or not (0).", "pool", "function")
	scanProgress  = reading("pool_scan_progress_ratio", "How far the pool's last scan got, from 0 to 1.", "pool", "function")
	scanErrors    = reading("pool_scan_errors", "How many errors the pool's last scan found.", "pool", "function")
	scanStart     = reading("pool_scan_start_timestamp_seconds", "When the pool's last scan started, in seconds since 1970.", "pool", "function")
	scanEnd       = reading("pool_scan_end_timestamp_seconds", "When the pool's last scan ended, in seconds since 1970. Left off while one runs.", "pool", "function")
	scanToExamine = reading("pool_scan_to_examine_bytes", "How much the pool's last scan had to look at.", "pool", "function")
	scanExamined  = reading("pool_scan_examined_bytes", "How much the pool's last scan has looked at.", "pool", "function")
	scanIssued    = reading("pool_scan_issued_bytes", "How much the pool's last scan has read and checked.", "pool", "function")
	scanRemaining = reading("pool_scan_remaining_seconds", "How long the NAS expects the pool's scan to go on. Only while one runs.", "pool", "function")

	vdevInfo          = reading("pool_vdev_info", "Always 1, with the kind of each top-level vdev: RAIDZ2, MIRROR, DISK and so on.", "pool", "class", "vdev", "type")
	vdevStatus        = reading("pool_vdev_status", "Always 1, with the status of each top-level vdev.", "pool", "class", "vdev", "status")
	vdevSize          = reading("pool_vdev_size_bytes", "The size of each top-level vdev that holds data.", "pool", "class", "vdev")
	vdevAllocated     = reading("pool_vdev_allocated_bytes", "How much of each top-level vdev that holds data is written to.", "pool", "class", "vdev")
	vdevFragmentation = reading("pool_vdev_fragmentation_ratio", "How broken up the free space of each top-level vdev that holds data is, from 0 to 1.", "pool", "class", "vdev")
	vdevRead          = reading("pool_vdev_read_errors", "Reads of each top-level vdev that have gone wrong since the count was last cleared.", "pool", "class", "vdev")
	vdevWrite         = reading("pool_vdev_write_errors", "Writes to each top-level vdev that have gone wrong since the count was last cleared.", "pool", "class", "vdev")
	vdevChecksum      = reading("pool_vdev_checksum_errors", "Times each top-level vdev has given back data that was not what was written, since the count was last cleared.", "pool", "class", "vdev")

	deviceStatus   = reading("pool_device_status", "Always 1, with the status of each disk in a pool.", "pool", "class", "vdev", "disk", "serial", "status")
	deviceRead     = reading("pool_device_read_errors", "Reads of each disk in a pool that have gone wrong since the count was last cleared.", "pool", "class", "vdev", "disk", "serial")
	deviceWrite    = reading("pool_device_write_errors", "Writes to each disk in a pool that have gone wrong since the count was last cleared.", "pool", "class", "vdev", "disk", "serial")
	deviceChecksum = reading("pool_device_checksum_errors", "Times each disk in a pool has given back data that was not what was written, since the count was last cleared.", "pool", "class", "vdev", "disk", "serial")

	diskInfo        = reading("disk_info", "Always 1, with what is known of each disk. The serial number is what stays the same; the name can change when the NAS restarts.", "disk", "serial", "wwn", "model", "type", "bus", "pool", "enclosure", "slot")
	diskSize        = reading("disk_size_bytes", "The size of each disk.", "disk", "serial")
	diskTemperature = reading("disk_temperature_celsius", "The temperature of each disk that gives one.", "disk", "serial")

	datasets              = reading("datasets", "How many datasets each pool has, however deep.", "pool")
	snapshots             = reading("snapshots", "How many snapshots each pool has, of every dataset however deep.", "pool")
	datasetUsed           = reading("dataset_used_bytes", "How much each dataset takes up, the datasets under it and its snapshots included.", "dataset", "pool")
	datasetAvailable      = reading("dataset_available_bytes", "How much each dataset may still grow by.", "dataset", "pool")
	datasetUsedByDataset  = reading("dataset_used_by_dataset_bytes", "How much each dataset's own data takes up, with nothing under it counted.", "dataset", "pool")
	datasetUsedBySnapshot = reading("dataset_used_by_snapshots_bytes", "How much each dataset's snapshots alone hold on to.", "dataset", "pool")
	datasetQuota          = reading("dataset_quota_bytes", "The most each dataset may take up with everything under it. Only where a quota is set.", "dataset", "pool")
	datasetRefQuota       = reading("dataset_refquota_bytes", "The most each dataset's own data may take up. Only where a quota is set.", "dataset", "pool")
	datasetCompression    = reading("dataset_compression_ratio", "How many times smaller each dataset is stored than it is: 1.5 is a third saved.", "dataset", "pool")
	datasetSnapshots      = reading("dataset_snapshots", "How many snapshots each dataset has of itself.", "dataset", "pool")

	alerts          = reading("alerts", "Alerts the NAS is showing that nobody has dismissed, by level.", "level")
	alertsByClass   = reading("alerts_by_class", "Alerts the NAS is showing that nobody has dismissed, by level and kind. A kind is here only while there is such an alert.", "level", "class")
	alertsDismissed = reading("alerts_dismissed", "Alerts somebody has dismissed that the NAS still holds.")

	serviceRunning = reading("service_running", "Whether each of the NAS's services is running (1) or not (0).", "service")
	serviceEnabled = reading("service_enabled", "Whether each of the NAS's services starts with it (1) or not (0).", "service")

	jobs        = reading("jobs", "Jobs on the NAS that are not over, by state: RUNNING or WAITING.", "state")
	jobsRunning = reading("jobs_running", "Jobs running on the NAS, by what they do. A kind is here only while one runs.", "method")

	appsService        = reading("apps_service_status", "Always 1, with the state of the service that runs apps: RUNNING, STOPPED, FAILED, or UNCONFIGURED where apps were never set up.", "status")
	apps               = reading("apps", "Installed apps, by state.", "state")
	appState           = reading("app_state", "Always 1, with the state of each installed app.", "app", "state")
	appUpdateAvailable = reading("app_update_available", "Whether there is a newer version of each installed app (1) or not (0).", "app")

	vms     = reading("vms", "Virtual machines, by state.", "state")
	vmState = reading("vm_state", "Always 1, with the state of each virtual machine.", "vm", "state")

	shareClients = reading("share_clients", "Clients connected to the NAS's shares, by protocol: nfs or iscsi. The NAS gives no count for SMB.", "protocol")
	shares       = reading("shares", "Shares, by protocol and by whether each is enabled, disabled or locked with its dataset.", "protocol", "state")

	tasks           = reading("tasks", "Scheduled tasks, by kind.", "kind")
	tasksEnabled    = reading("tasks_enabled", "Scheduled tasks that are switched on, by kind.", "kind")
	tasksRunning    = reading("tasks_running", "Scheduled tasks running now, by kind.", "kind")
	tasksFailed     = reading("tasks_failed", "Scheduled tasks whose last run failed, by kind.", "kind")
	taskEnabled     = reading("task_enabled", "Whether each scheduled task is switched on (1) or not (0).", "kind", "id", "name")
	taskRunning     = reading("task_running", "Whether each scheduled task is running now (1) or not (0).", "kind", "id", "name")
	taskProgress    = reading("task_progress_ratio", "How far each running task says it has got, from 0 to 1. Only while it runs.", "kind", "id", "name")
	taskLastFailed  = reading("task_last_run_failed", "Whether the last run of each scheduled task that has run failed (1) or not (0).", "kind", "id", "name")
	taskLastEnd     = reading("task_last_run_timestamp_seconds", "When the last run of each scheduled task ended, in seconds since 1970.", "kind", "id", "name")
	taskLastTook    = reading("task_last_run_duration_seconds", "How long the last run of each scheduled task took, for the kinds the NAS times.", "kind", "id", "name")
	certificateEnd  = reading("certificate_expiry_timestamp_seconds", "When each certificate the NAS holds stops being good, in seconds since 1970.", "certificate")
	interfaceLinkUp = reading("interface_link_up", "Whether each network interface has a link (1) or not (0).", "interface", "type")
	interfaceMTU    = reading("interface_mtu_bytes", "The largest packet each network interface sends.", "interface")

	directoryHealthy = reading("directory_service_healthy", "Whether the directory service the NAS is joined to is healthy (1) or not (0). Only on a NAS joined to one.", "type", "status")
	keyExpiry        = reading("api_key_expiry_timestamp_seconds", "When the API key truenas-metrics uses stops working, in seconds since 1970. Only for a key that expires.")
)

// Share states, as the page words them.
const (
	shareEnabled  = "enabled"
	shareDisabled = "disabled"
	shareLocked   = "locked"
)

// loadWindows is what each of the three load averages is over.
var loadWindows = []string{"1m", "5m", "15m"}

// Source is where the NAS's state comes from: a poller, or a test's own.
type Source interface {
	State() poll.State
}

// Options says what of the NAS goes on the page.
type Options struct {
	// DatasetDepth is how far below its pool a dataset may be and still have readings of its own: 0 for each pool's own dataset alone, 1 for those
	// directly under it too. A dataset deeper than that is still counted, and its space is part of the ones above it.
	DatasetDepth int
}

// Readings writes what the source knows as readings. It asks the NAS nothing, so a scrape is answered from memory however slow the NAS is.
type Readings struct {
	source Source
	o      Options
}

// New is the readings of this NAS.
func New(source Source, o Options) *Readings {
	return &Readings{source: source, o: o}
}

// Describe names every reading there can be.
func (*Readings) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range all {
		ch <- d
	}
}

// Collect writes every reading there is an answer for. A question the NAS has never answered has no reading, which is not the same as a reading of
// zero.
func (r *Readings) Collect(ch chan<- prometheus.Metric) {
	s := r.source.State()
	w := &writer{ch: ch, written: map[string]bool{}}

	w.rounds(&s)
	w.system(&s)
	w.pools(&s)
	w.disks(&s)
	w.datasets(&s, r.o.DatasetDepth)
	w.snapshots(&s, r.o.DatasetDepth)
	w.alerts(&s)
	w.services(&s)
	w.jobs(&s)
	w.apps(&s)
	w.vms(&s)
	w.shares(&s)
	w.tasks(&s)
	w.rest(&s)
}

// writer writes the NAS's readings.
type writer struct {
	ch chan<- prometheus.Metric
	// written is every reading written so far, by name and labels. The NAS's names are its own and nothing promises two tasks or two disks will not
	// share one, and two readings with the same labels would spoil the whole page: the second is left off.
	written map[string]bool
}

// write writes one reading, unless one of that name and those labels is already on the page.
func (w *writer) write(d *prometheus.Desc, kind prometheus.ValueType, value float64, labels ...string) {
	key := d.String() + "\x00" + strings.Join(labels, "\x00")
	if w.written[key] {
		return
	}
	w.written[key] = true

	w.ch <- prometheus.MustNewConstMetric(d, kind, value, labels...)
}

// gauge writes one reading that can go up and down.
func (w *writer) gauge(d *prometheus.Desc, value float64, labels ...string) {
	w.write(d, prometheus.GaugeValue, value, labels...)
}

// count is gauge for a whole number.
func (w *writer) count(d *prometheus.Desc, n int, labels ...string) {
	w.gauge(d, float64(n), labels...)
}

// bytes is gauge for a size.
func (w *writer) bytes(d *prometheus.Desc, n int64, labels ...string) {
	w.gauge(d, float64(n), labels...)
}

// flag is gauge for a yes or no: 1 or 0.
func (w *writer) flag(d *prometheus.Desc, yes bool, labels ...string) {
	if yes {
		w.gauge(d, 1, labels...)
		return
	}

	w.gauge(d, 0, labels...)
}

// at is gauge for a moment, in seconds since 1970.
func (w *writer) at(d *prometheus.Desc, t time.Time, labels ...string) {
	w.gauge(d, float64(t.UnixMilli())/1000, labels...)
}

// rounds is how the polling itself is going, which is what says how far to trust the rest. An area left out has nothing here either.
func (w *writer) rounds(s *poll.State) {
	if s.Rounds[poll.AreaSystem].Ran {
		w.flag(up, s.Up())
	}

	for _, area := range poll.Every {
		r, asked := s.Rounds[area]
		if !asked {
			continue
		}

		w.gauge(pollInterval, r.Interval.Seconds(), string(area))
		if !r.Ran {
			continue
		}

		w.gauge(pollDuration, r.Took.Seconds(), string(area))
		w.write(pollFailures, prometheus.CounterValue, float64(r.Failures), string(area))
		if !r.LastSuccess.IsZero() {
			w.at(pollLastSuccess, r.LastSuccess, string(area))
		}
	}
}

// system is the NAS itself, and what it knows of newer versions of itself.
func (w *writer) system(s *poll.State) {
	if s.System.Known() {
		i := s.System.Value
		w.gauge(systemInfo, 1, i.Version, i.Product, i.Manufacturer, i.CPU)
		w.states(systemState, poll.SystemStates, i.State)
		w.gauge(systemUptime, i.Uptime.Seconds())
		w.bytes(systemMemory, i.Memory)
		w.count(systemCores, i.Cores)
		w.count(systemPhysical, i.PhysicalCores)
		w.flag(systemECC, i.ECC)
		w.gauge(systemClockOffset, i.ClockOffset.Seconds())

		if !i.Booted.IsZero() {
			w.at(systemBoot, i.Booted)
		}

		for n, window := range loadWindows {
			w.gauge(systemLoad, i.Load[n], window)
		}
	}

	if s.Update.Known() {
		w.flag(updateAvailable, s.Update.Value.Available)
		w.flag(updateCheckFailed, s.Update.Value.CheckFailed)
	}
}

// states writes 1 for the state something is in and 0 for each of the others it could be in, so none vanishes when it is left. A state that is not
// one of those known is written too, at 1: a NAS newer than this may have states this has not heard of.
func (w *writer) states(d *prometheus.Desc, known []string, is string) {
	for _, state := range known {
		w.flag(d, state == is, state)
	}

	if is != "" && !slices.Contains(known, is) {
		w.gauge(d, 1, is)
	}
}

// pools is every pool, the boot pool among them, with its last scan, its vdevs and the disks they are made of.
func (w *writer) pools(s *poll.State) {
	serials := serialsOf(s)

	var every []poll.Pool
	if s.Pools.Known() {
		every = append(every, s.Pools.Value...)
	}
	if s.BootPool.Known() {
		every = append(every, s.BootPool.Value)
	}

	for i := range every {
		p := &every[i]

		w.flag(poolHealthy, p.Healthy, p.Name)
		w.flag(poolWarning, p.Warning, p.Name)
		w.bytes(poolSize, p.Size, p.Name)
		w.bytes(poolAllocated, p.Allocated, p.Name)
		w.bytes(poolFree, p.Free, p.Name)
		w.bytes(poolFreeing, p.Freeing, p.Name)

		if p.Status != "" {
			w.gauge(poolStatus, 1, p.Name, p.Status)
		}

		if p.Fragmentation != nil {
			w.gauge(poolFragmentation, *p.Fragmentation, p.Name)
		}

		w.scan(p)

		for v := range p.Vdevs {
			w.vdev(p.Name, &p.Vdevs[v], serials)
		}
	}
}

// scan is a pool's last scrub or resilver. Whether one is running is written for both kinds, at 0 when it is not, so the reading is there before the
// first scan and after the last; the rest is of the one scan the pool remembers.
func (w *writer) scan(p *poll.Pool) {
	scan := p.Scan
	running := scan != nil && scan.State == poll.ScanRunning

	for _, function := range poll.ScanFunctions {
		w.flag(scanRunning, running && scan.Function == function, p.Name, function)
	}

	if scan == nil {
		return
	}

	// a function this has not heard of is still running, and is said to be under its own name
	if running && !slices.Contains(poll.ScanFunctions, scan.Function) {
		w.gauge(scanRunning, 1, p.Name, scan.Function)
	}

	w.flag(scanPaused, scan.Paused, p.Name, scan.Function)
	w.gauge(scanProgress, scan.Progress, p.Name, scan.Function)
	w.gauge(scanErrors, float64(scan.Errors), p.Name, scan.Function)
	w.bytes(scanToExamine, scan.ToExamine, p.Name, scan.Function)
	w.bytes(scanExamined, scan.Examined, p.Name, scan.Function)
	w.bytes(scanIssued, scan.Issued, p.Name, scan.Function)

	if !scan.Started.IsZero() {
		w.at(scanStart, scan.Started, p.Name, scan.Function)
	}

	if !running && !scan.Ended.IsZero() {
		w.at(scanEnd, scan.Ended, p.Name, scan.Function)
	}

	if running && scan.Left != nil {
		w.gauge(scanRemaining, scan.Left.Seconds(), p.Name, scan.Function)
	}
}

// vdev is one top-level vdev and its disks. Size and what is written to it are left off a vdev that holds no data of its own, a spare say, since a
// size of 0 would read as a disk of no size.
func (w *writer) vdev(pool string, v *poll.Vdev, serials map[string]string) {
	w.gauge(vdevInfo, 1, pool, v.Class, v.Name, v.Type)
	w.gauge(vdevRead, float64(v.Errors.Read), pool, v.Class, v.Name)
	w.gauge(vdevWrite, float64(v.Errors.Write), pool, v.Class, v.Name)
	w.gauge(vdevChecksum, float64(v.Errors.Checksum), pool, v.Class, v.Name)

	if v.Status != "" {
		w.gauge(vdevStatus, 1, pool, v.Class, v.Name, v.Status)
	}

	if v.Size > 0 {
		w.bytes(vdevSize, v.Size, pool, v.Class, v.Name)
		w.bytes(vdevAllocated, v.Allocated, pool, v.Class, v.Name)
		w.gauge(vdevFragmentation, v.Fragmentation, pool, v.Class, v.Name)
	}

	for i := range v.Devices {
		d := &v.Devices[i]
		serial := serials[d.Disk]

		w.gauge(deviceRead, float64(d.Errors.Read), pool, v.Class, v.Name, d.Disk, serial)
		w.gauge(deviceWrite, float64(d.Errors.Write), pool, v.Class, v.Name, d.Disk, serial)
		w.gauge(deviceChecksum, float64(d.Errors.Checksum), pool, v.Class, v.Name, d.Disk, serial)

		if d.Status != "" {
			w.gauge(deviceStatus, 1, pool, v.Class, v.Name, d.Disk, serial, d.Status)
		}
	}
}

// serialsOf is the serial number of each disk the NAS knows of, by the disk's name. The pools say which disks they are made of by name alone, and
// the serial number is what another exporter's readings of the same disk are joined on.
func serialsOf(s *poll.State) map[string]string {
	serials := map[string]string{}
	if !s.Disks.Known() {
		return serials
	}

	for _, d := range s.Disks.Value {
		serials[d.Name] = d.Serial
	}

	return serials
}

// poolsOf is which pool each disk is in, by the disk's name.
func poolsOf(s *poll.State) map[string]string {
	pools := map[string]string{}

	var every []poll.Pool
	if s.Pools.Known() {
		every = append(every, s.Pools.Value...)
	}
	if s.BootPool.Known() {
		every = append(every, s.BootPool.Value)
	}

	for _, p := range every {
		for _, v := range p.Vdevs {
			for _, d := range v.Devices {
				if d.Disk != "" {
					pools[d.Disk] = p.Name
				}
			}
		}
	}

	return pools
}

// disks is every disk the NAS knows of, in a pool or not.
func (w *writer) disks(s *poll.State) {
	if !s.Disks.Known() {
		return
	}

	pools := poolsOf(s)
	for _, d := range s.Disks.Value {
		var enclosure, slot string
		if d.Slot != nil {
			enclosure, slot = strconv.Itoa(d.Slot.Enclosure), strconv.Itoa(d.Slot.Slot)
		}

		w.gauge(diskInfo, 1, d.Name, d.Serial, d.WWN, d.Model, d.Type, d.Bus, pools[d.Name], enclosure, slot)
		w.bytes(diskSize, d.Size, d.Name, d.Serial)

		if d.Temperature != nil {
			w.gauge(diskTemperature, *d.Temperature, d.Name, d.Serial)
		}
	}
}

// datasets counts every dataset of each pool, and gives the space of the datasets no deeper than depth. The count takes in the deeper ones, so that
// nothing is lost by not naming them.
func (w *writer) datasets(s *poll.State, depth int) {
	if !s.Datasets.Known() {
		return
	}

	total := map[string]int{}
	for i := range s.Datasets.Value {
		d := &s.Datasets.Value[i]

		total[d.Pool]++
		if d.Depth() > depth {
			continue
		}

		w.bytes(datasetUsed, d.Used, d.Name, d.Pool)
		w.bytes(datasetAvailable, d.Available, d.Name, d.Pool)
		w.bytes(datasetUsedByDataset, d.UsedByDataset, d.Name, d.Pool)
		w.bytes(datasetUsedBySnapshot, d.UsedBySnapshots, d.Name, d.Pool)

		if d.Quota != nil {
			w.bytes(datasetQuota, *d.Quota, d.Name, d.Pool)
		}

		if d.RefQuota != nil {
			w.bytes(datasetRefQuota, *d.RefQuota, d.Name, d.Pool)
		}

		if d.Compression > 0 {
			w.gauge(datasetCompression, d.Compression, d.Name, d.Pool)
		}
	}

	for pool, n := range total {
		w.count(datasets, n, pool)
	}
}

// snapshots counts every snapshot of each pool, and of each dataset no deeper than depth. The pool's count takes in the deeper ones.
func (w *writer) snapshots(s *poll.State, depth int) {
	if !s.Snapshots.Known() {
		return
	}

	total := map[string]int{}
	for i := range s.Snapshots.Value {
		c := &s.Snapshots.Value[i]

		total[c.Pool] += c.Count
		if c.Depth() <= depth {
			w.count(datasetSnapshots, c.Count, c.Dataset, c.Pool)
		}
	}

	for pool, n := range total {
		w.count(snapshots, n, pool)
	}
}

// alerts counts the alerts nobody has dismissed by level, every level written at 0 when it has none so that a reading does not vanish when the last
// alert clears, and by kind as well, which is on the page only while there is such an alert.
func (w *writer) alerts(s *poll.State) {
	if !s.Alerts.Known() {
		return
	}

	type kind struct{ level, class string }

	levels, kinds := map[string]int{}, map[kind]int{}
	var dismissed int
	for _, a := range s.Alerts.Value {
		if a.Dismissed {
			dismissed++
			continue
		}

		levels[a.Level]++
		kinds[kind{a.Level, a.Class}]++
	}

	for _, level := range poll.AlertLevels {
		w.count(alerts, levels[level], level)
	}
	for level, n := range levels {
		w.count(alerts, n, level)
	}
	for k, n := range kinds {
		w.count(alertsByClass, n, k.level, k.class)
	}
	w.count(alertsDismissed, dismissed)
}

func (w *writer) services(s *poll.State) {
	if !s.Services.Known() {
		return
	}

	for _, service := range s.Services.Value {
		w.flag(serviceRunning, service.Running, service.Name)
		w.flag(serviceEnabled, service.Enabled, service.Name)
	}
}

// jobs counts the jobs that are not over by state, and the running ones by what they do.
func (w *writer) jobs(s *poll.State) {
	if !s.Jobs.Known() {
		return
	}

	states, methods := map[string]int{}, map[string]int{}
	for _, job := range s.Jobs.Value {
		states[job.State]++
		if job.State == poll.JobRunning {
			methods[job.Method]++
		}
	}

	w.counts(jobs, poll.JobStates, states)
	for method, n := range methods {
		w.count(jobsRunning, n, method)
	}
}

// counts writes how many there are of each kind: every kind known, at 0 when there is none, and any other kind there turns out to be.
func (w *writer) counts(d *prometheus.Desc, known []string, n map[string]int) {
	for _, kind := range known {
		w.count(d, n[kind], kind)
	}

	for kind, count := range n {
		w.count(d, count, kind)
	}
}

func (w *writer) apps(s *poll.State) {
	if !s.Apps.Known() {
		return
	}

	if status := s.Apps.Value.Status; status != "" {
		w.gauge(appsService, 1, status)
	}

	states := map[string]int{}
	for _, app := range s.Apps.Value.List {
		states[app.State]++
		w.gauge(appState, 1, app.Name, app.State)
		w.flag(appUpdateAvailable, app.UpdateAvailable, app.Name)
	}
	w.counts(apps, poll.AppStates, states)
}

func (w *writer) vms(s *poll.State) {
	if !s.VMs.Known() {
		return
	}

	states := map[string]int{}
	for _, vm := range s.VMs.Value {
		states[vm.State]++
		w.gauge(vmState, 1, vm.Name, vm.State)
	}
	w.counts(vms, poll.VMStates, states)
}

// shares counts the shares by protocol and state, and the clients connected by protocol. Every protocol and state is written, at 0 when there is
// none.
func (w *writer) shares(s *poll.State) {
	if s.Clients.Known() {
		for _, c := range s.Clients.Value {
			w.count(shareClients, c.Count, c.Protocol)
		}
	}

	if !s.Shares.Known() {
		return
	}

	type kind struct{ protocol, state string }

	n := map[kind]int{}
	for _, share := range s.Shares.Value {
		switch {
		case share.Locked:
			n[kind{share.Protocol, shareLocked}]++
		case share.Enabled:
			n[kind{share.Protocol, shareEnabled}]++
		default:
			n[kind{share.Protocol, shareDisabled}]++
		}
	}

	for _, protocol := range poll.ShareProtocols {
		for _, state := range []string{shareEnabled, shareDisabled, shareLocked} {
			w.count(shares, n[kind{protocol, state}], protocol, state)
		}
	}
	for k, count := range n {
		w.count(shares, count, k.protocol, k.state)
	}
}

// tasks counts the scheduled tasks by kind, every kind written at 0 when it has none, and says of each task whether it is on, running, and how its
// last run went.
func (w *writer) tasks(s *poll.State) {
	if !s.Tasks.Known() {
		return
	}

	total, enabled, running, failed := map[poll.TaskKind]int{}, map[poll.TaskKind]int{}, map[poll.TaskKind]int{}, map[poll.TaskKind]int{}
	for i := range s.Tasks.Value {
		task := &s.Tasks.Value[i]
		kind, id := string(task.Kind), strconv.Itoa(task.ID)

		total[task.Kind]++
		w.flag(taskEnabled, task.Enabled, kind, id, task.Name)
		if task.Enabled {
			enabled[task.Kind]++
		}

		// a scrub's runs are the pool's scans, which have readings of their own
		if task.Kind == poll.ScrubTask {
			continue
		}

		w.flag(taskRunning, task.Running, kind, id, task.Name)
		if task.Running {
			running[task.Kind]++
			w.gauge(taskProgress, task.Progress, kind, id, task.Name)
		}

		run := task.LastRun
		if run == nil {
			continue
		}

		w.flag(taskLastFailed, run.Failed, kind, id, task.Name)
		if run.Failed {
			failed[task.Kind]++
		}

		if !run.Ended.IsZero() {
			w.at(taskLastEnd, run.Ended, kind, id, task.Name)
		}

		if run.Took > 0 {
			w.gauge(taskLastTook, run.Took.Seconds(), kind, id, task.Name)
		}
	}

	for _, kind := range poll.TaskKinds {
		w.count(tasks, total[kind], string(kind))
		w.count(tasksEnabled, enabled[kind], string(kind))

		if kind != poll.ScrubTask {
			w.count(tasksRunning, running[kind], string(kind))
			w.count(tasksFailed, failed[kind], string(kind))
		}
	}
}

// rest is the small things: certificates, network interfaces, the directory service and the key this is asked with.
func (w *writer) rest(s *poll.State) {
	if s.Certificates.Known() {
		for _, c := range s.Certificates.Value {
			if !c.Expires.IsZero() {
				w.at(certificateEnd, c.Expires, c.Name)
			}
		}
	}

	if s.Interfaces.Known() {
		for _, i := range s.Interfaces.Value {
			w.flag(interfaceLinkUp, i.LinkUp, i.Name, i.Type)
			if i.MTU > 0 {
				w.count(interfaceMTU, i.MTU, i.Name)
			}
		}
	}

	if d := s.Directory; d.Known() && d.Value.Type != "" {
		w.flag(directoryHealthy, d.Value.Status == "HEALTHY", d.Value.Type, d.Value.Status)
	}

	if k := s.Key; k.Known() && !k.Value.Expires.IsZero() {
		w.at(keyExpiry, k.Value.Expires)
	}
}
