package readings_test

import (
	"flag"
	"os"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/katbyte/go-kt/pmetrics"
	"github.com/katbyte/go-kt/pointer"
	"github.com/katbyte/truenas-exporter/lib/poll"
	"github.com/katbyte/truenas-exporter/lib/readings"
	"github.com/prometheus/client_golang/prometheus/testutil/promlint"
)

// saved is where the page is kept as it was last agreed.
const saved = "testdata/page.txt"

const (
	gib = 1 << 30
	tib = 1 << 40
)

// update rewrites the saved page from what the code writes now: go test ./lib/readings -update. Read the difference before keeping it, since a
// reading that is renamed is one somebody's dashboard no longer finds.
var update = flag.Bool("update", false, "rewrite testdata/page.txt from the page as it is written now")

// fixed is a NAS whose state is whatever the test says.
type fixed poll.State

func (f *fixed) State() poll.State { return poll.State(*f) }

// answered is an answer given now.
func answered[T any](value T) poll.Answer[T] {
	return poll.Answer[T]{Value: value, At: time.Now()}
}

// online is a disk in a pool with nothing wrong with it.
func online(disk string) poll.Device {
	return poll.Device{Disk: disk, Status: "ONLINE"}
}

// tank is a NAS that has answered everything. Its one data pool is degraded: a raidz2 of four disks, one of them faulted with errors against it, a
// mirror for metadata, a cache disk and a spare, last scrubbed two days ago. Its boot pool is resilvering. It has alerts of two kinds and one
// dismissed, a cloud sync running, two apps, a virtual machine, four datasets of which one is two deep, and a task of every kind: one failed, one
// stopped by hand, one that has never run.
func tank() *fixed {
	now := time.Now()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }

	return &fixed{
		System: answered(poll.System{
			Version: "25.10.7", Product: "X11SSH-F", Manufacturer: "Supermicro", CPU: "Intel(R) Xeon(R) CPU E3-1230 v6",
			State: "READY", Booted: ago(240 * time.Hour), Uptime: 240 * time.Hour, Memory: 64 * gib, Cores: 8, PhysicalCores: 4,
			Load: [3]float64{0.5, 0.75, 1.25}, ECC: true, ClockOffset: -20 * time.Millisecond,
		}),
		Pools: answered([]poll.Pool{{
			Name: "tank", Status: "DEGRADED", Warning: true,
			Size: 40 * tib, Allocated: 30 * tib, Free: 10 * tib, Freeing: 4 * gib, Fragmentation: new(0.14),
			Scan: &poll.Scan{Function: poll.Scrub, State: "FINISHED", Started: ago(50 * time.Hour), Ended: ago(48 * time.Hour), Progress: 1, ToExamine: 30 * tib, Examined: 30 * tib, Issued: 30 * tib, Errors: 2},
			Vdevs: []poll.Vdev{
				{Class: "data", Name: "raidz2-0", Type: "RAIDZ2", Status: "DEGRADED", Size: 40 * tib, Allocated: 30 * tib, Fragmentation: 0.15, Errors: poll.Errors{Checksum: 7}, Devices: []poll.Device{
					online("sda"), online("sdb"), online("sdc"),
					{Disk: "sdd", Status: "FAULTED", Errors: poll.Errors{Read: 3, Write: 1, Checksum: 7}},
				}},
				{Class: "special", Name: "mirror-1", Type: "MIRROR", Status: "ONLINE", Size: tib, Allocated: 100 * gib, Fragmentation: 0.02, Devices: []poll.Device{online("nvme0n1"), online("nvme1n1")}},
				{Class: "cache", Name: "nvme2n1p1", Type: "DISK", Status: "ONLINE", Size: 500 * gib, Allocated: 400 * gib, Devices: []poll.Device{online("nvme2n1")}},
				{Class: "spare", Name: "sde1", Type: "DISK", Status: "ONLINE", Devices: []poll.Device{online("sde")}},
			},
		}}),
		BootPool: answered(poll.Pool{
			Name: "boot-pool", Status: "ONLINE", Healthy: true, Size: 100 * gib, Allocated: 20 * gib, Free: 80 * gib, Fragmentation: new(0.49),
			Scan:  &poll.Scan{Function: poll.Resilver, State: poll.ScanRunning, Started: ago(10 * time.Minute), Progress: 0.25, ToExamine: 20 * gib, Examined: 6 * gib, Issued: 5 * gib, Left: new(30 * time.Minute)},
			Vdevs: []poll.Vdev{{Class: "data", Name: "mirror-0", Type: "MIRROR", Status: "ONLINE", Size: 100 * gib, Allocated: 20 * gib, Fragmentation: 0.49, Devices: []poll.Device{online("sdf"), online("sdg")}}},
		}),
		Alerts: answered([]poll.Alert{
			{Level: "CRITICAL", Class: "VolumeStatus"},
			{Level: "CRITICAL", Class: "RsyncFailed"},
			{Level: "CRITICAL", Class: "RsyncFailed"},
			{Level: "INFO", Class: "RsyncSuccess"},
			{Level: "WARNING", Class: "ScrubPaused", Dismissed: true},
		}),
		Services: answered([]poll.Service{{Name: "cifs", Running: true, Enabled: true}, {Name: "nfs", Running: true, Enabled: true}, {Name: "ssh", Enabled: true}, {Name: "ftp"}}),
		Jobs:     answered([]poll.Job{{Method: "cloudsync.sync", State: "RUNNING"}, {Method: "pool.scrub.run", State: "RUNNING"}, {Method: "cloudsync.sync", State: "WAITING"}}),
		Apps:     answered(poll.Apps{Status: "RUNNING", List: []poll.App{{Name: "plex", State: "RUNNING", UpdateAvailable: true}, {Name: "syncthing", State: "STOPPED"}}}),
		VMs:      answered([]poll.VM{{Name: "homeassistant", State: "RUNNING"}, {Name: "scratch", State: "STOPPED"}}),
		Clients:  answered([]poll.Clients{{Protocol: "nfs", Count: 3}, {Protocol: "iscsi"}}),

		Disks: answered([]poll.Disk{
			{Name: "sda", Serial: "ZR5A0001", WWN: "5000c500e0000001", Model: "ST20000NM007D", Type: "HDD", Bus: "SCSI", Size: 20 * tib, Temperature: new(34.0)},
			{Name: "sdb", Serial: "ZR5A0002", WWN: "5000c500e0000002", Model: "ST20000NM007D", Type: "HDD", Bus: "SCSI", Size: 20 * tib, Temperature: new(35.0), Slot: &poll.Slot{Enclosure: 0, Slot: 2}},
			{Name: "sdc", Serial: "ZR5A0003", WWN: "5000c500e0000003", Model: "ST20000NM007D", Type: "HDD", Bus: "SCSI", Size: 20 * tib, Temperature: new(33.0)},
			{Name: "sdd", Serial: "ZR5A0004", WWN: "5000c500e0000004", Model: "ST20000NM007D", Type: "HDD", Bus: "SCSI", Size: 20 * tib},
			{Name: "sde", Serial: "ZR5A0005", WWN: "5000c500e0000005", Model: "ST20000NM007D", Type: "HDD", Bus: "SCSI", Size: 20 * tib, Temperature: new(29.0)},
			{Name: "nvme0n1", Serial: "S6B0NA0T000001", Model: "SAMSUNG MZQL21T9HCJR", Type: "SSD", Bus: "NVME", Size: tib, Temperature: new(41.0)},
			{Name: "sdh", Serial: "ZR5A0008", WWN: "5000c500e0000008", Model: "ST20000NM007D", Type: "HDD", Bus: "SCSI", Size: 20 * tib, Temperature: new(30.0)},
		}),
		Datasets: answered([]poll.Dataset{
			{Name: "tank", Pool: "tank", Type: "FILESYSTEM", Used: 20 * tib, Available: 6 * tib, UsedByDataset: 200 * gib, Compression: 1.04},
			{Name: "tank/media", Pool: "tank", Type: "FILESYSTEM", Used: 15 * tib, Available: 6 * tib, UsedByDataset: 100 * gib, UsedBySnapshots: 50 * gib, Compression: 1},
			{Name: "tank/media/films", Pool: "tank", Type: "FILESYSTEM", Used: 12 * tib, Available: 6 * tib, UsedByDataset: 12 * tib, Compression: 1},
			{Name: "tank/backups", Pool: "tank", Type: "FILESYSTEM", Used: 4 * tib, Available: tib, UsedByDataset: 3 * tib, UsedBySnapshots: tib, Quota: new(int64(5 * tib)), RefQuota: new(int64(4 * tib)), Compression: 1.8},
			{Name: "boot-pool", Pool: "boot-pool", Type: "FILESYSTEM", Used: 20 * gib, Available: 78 * gib, UsedByDataset: 96 << 10, Compression: 1.5},
		}),
		Snapshots: answered([]poll.SnapshotCount{{Dataset: "tank", Pool: "tank"}, {Dataset: "tank/media", Pool: "tank", Count: 12}, {Dataset: "tank/media/films", Pool: "tank", Count: 30}, {Dataset: "tank/backups", Pool: "tank", Count: 90}}),
		Tasks: answered([]poll.Task{
			{Kind: poll.SnapshotTask, ID: 1, Name: "tank/media", Enabled: true, LastRun: &poll.Run{OK: true, Ended: ago(time.Hour)}},
			{Kind: poll.SnapshotTask, ID: 2, Name: "tank/backups", LastRun: &poll.Run{Failed: true, Ended: ago(26 * time.Hour)}},
			{Kind: poll.ReplicationTask, ID: 1, Name: "tank to offsite", Enabled: true},
			{Kind: poll.CloudSyncTask, ID: 3, Name: "photos to the cloud", Enabled: true, Running: true, Progress: 0.87, LastRun: &poll.Run{OK: true, Ended: ago(24 * time.Hour), Took: 25 * time.Minute}},
			{Kind: poll.CloudSyncTask, ID: 4, Name: "documents to the cloud", Enabled: true, LastRun: &poll.Run{Ended: ago(3 * time.Hour), Took: 40 * time.Second}},
			{Kind: poll.RsyncTask, ID: 7, Name: "media to the old NAS", Enabled: true, LastRun: &poll.Run{Failed: true, Ended: ago(2 * time.Hour), Took: 3 * time.Second}},
			{Kind: poll.ScrubTask, ID: 1, Name: "tank", Enabled: true},
		}),
		Shares:       answered([]poll.Share{{Protocol: "smb", Enabled: true}, {Protocol: "smb", Enabled: true}, {Protocol: "smb"}, {Protocol: "nfs", Enabled: true, Locked: true}}),
		Certificates: answered([]poll.Certificate{{Name: "truenas_default", Expires: now.Add(72 * 24 * time.Hour)}}),
		Interfaces:   answered([]poll.Interface{{Name: "eno1", Type: "PHYSICAL", LinkUp: true, MTU: 9000}, {Name: "eno2", Type: "PHYSICAL", MTU: 1500}}),
		Update:       answered(poll.Update{Available: true}),
		Directory:    answered(poll.Directory{Type: "ACTIVEDIRECTORY", Status: "HEALTHY"}),
		Key:          answered(poll.Key{Expires: now.Add(30 * 24 * time.Hour)}),

		Rounds: rounds(now),
	}
}

// rounds is every area asked about as often as it is by default and answered just now, but for the tasks: answered a minute ago, after two polls
// that failed.
func rounds(now time.Time) map[poll.Area]poll.RoundState {
	out := map[poll.Area]poll.RoundState{}
	for area, every := range poll.DefaultIntervals() {
		out[area] = poll.RoundState{Interval: every, Ran: true, OK: true, Took: 250 * time.Millisecond, LastSuccess: now}
	}
	out[poll.AreaTasks] = poll.RoundState{Interval: 5 * time.Minute, Ran: true, OK: true, Took: 3 * time.Second, LastSuccess: now.Add(-time.Minute), Failures: 2}

	return out
}

// down is a NAS that has been asked once about its own state and did not answer.
func down() *fixed {
	return &fixed{Rounds: map[poll.Area]poll.RoundState{poll.AreaSystem: {Interval: 30 * time.Second, Ran: true, Took: 30 * time.Second, Failures: 1}}}
}

// page is the page of this NAS's readings, with the datasets named to the depth given and the labels on every one.
func page(t *testing.T, source readings.Source, depth int, labels map[string]string) string {
	t.Helper()

	p, err := pmetrics.New(pmetrics.Options{Name: "truenas-exporter", Namespace: readings.Namespace, Labels: labels}, readings.New(source, readings.Options{DatasetDepth: depth}))
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := p.Write(&out); err != nil {
		t.Fatal(err)
	}

	return out.String()
}

// write is the page as a scrape reads it with the settings left alone: the readings, and the one line the page adds to say which build wrote it.
func write(t *testing.T, source readings.Source) string {
	t.Helper()

	return page(t, source, readings.DefaultDatasetDepth, nil)
}

// lines is the page's readings without the comments above each, and without the one line that changes with the Go it was built by.
func lines(text string) []string {
	var out []string
	for line := range strings.Lines(text) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "truenas_metrics_build_info") {
			out = append(out, line)
		}
	}

	return out
}

// has says whether the page holds a reading that starts as given: its name, or its name and labels.
func has(text, start string) bool {
	for _, line := range lines(text) {
		if strings.HasPrefix(line, start) {
			return true
		}
	}

	return false
}

// wants fails the test for each of these readings the page lacks.
func wants(t *testing.T, got string, want ...string) {
	t.Helper()

	for _, w := range want {
		if !has(got, w) {
			t.Errorf("the page lacks %s", w)
		}
	}

	if t.Failed() {
		t.Logf("the page:\n%s", strings.Join(lines(got), "\n"))
	}
}

// absent fails the test for each of these readings the page holds.
func absent(t *testing.T, got string, not ...string) {
	t.Helper()

	for _, n := range not {
		if has(got, n) {
			t.Errorf("the page has %s, which there is nothing to say of:\n%s", n, strings.Join(lines(got), "\n"))
		}
	}
}

// The page is what dashboards and alerts are written against, so the whole of it is kept in testdata/page.txt and any change to a name, a label
// or a help text shows up as a difference there. It is written at the start of the year 2000, where the test's clock begins.
func TestThePageIsWhatWasSaved(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		got := write(t, tank())
		got = strings.Replace(got, runtime.Version(), "go", 1)

		if pointer.From(update) {
			if err := os.WriteFile(saved, []byte(got), 0o600); err != nil {
				t.Fatal(err)
			}
		}

		want, err := os.ReadFile(saved)
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Errorf("the page is not the one saved in %s; if the change is meant, run go test ./lib/readings -update and read the difference\n%s", saved, got)
		}
	})
}

// The names follow the conventions Prometheus checks for: a unit at the end, _total on what only goes up, help on everything.
func TestThePageFollowsTheNamingConventions(t *testing.T) {
	t.Parallel()

	problems, err := promlint.New(strings.NewReader(write(t, tank()))).Lint()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Errorf("%s: %s", p.Metric, p.Text)
	}
}

// No label whoever runs the program is likely to want is one a reading already has, so the page can still be made with them on it.
func TestTheLabelsAnyoneWouldSetCanBeSet(t *testing.T) {
	t.Parallel()

	got := page(t, tank(), readings.DefaultDatasetDepth, map[string]string{"hostname": "nas", "host": "nas", "job": "truenas", "instance": "nas", "site": "home", "nas": "tank"})

	for _, line := range lines(got) {
		if !strings.Contains(line, `hostname="nas"`) {
			t.Errorf("a reading without the labels: %s", line)
		}
	}
}

// A NAS that has never been asked has no readings at all, and one that was asked and did not answer has only how the asking went: a pool reported
// healthy, or with no alerts, would say something nobody knows.
func TestANASThatNeverAnsweredHasNoReadingsOfItsState(t *testing.T) {
	t.Parallel()

	if got := lines(write(t, &fixed{})); len(got) != 0 {
		t.Errorf("a NAS never asked has readings: %v", got)
	}

	got := write(t, down())
	want := []string{
		`truenas_poll_duration_seconds{poll="system"} 30`,
		`truenas_poll_failures_total{poll="system"} 1`,
		`truenas_poll_interval_seconds{poll="system"} 30`,
		`truenas_up 0`,
	}
	if strings.Join(lines(got), "\n") != strings.Join(want, "\n") {
		t.Errorf("a NAS that did not answer reads\n%s\nwant\n%s", strings.Join(lines(got), "\n"), strings.Join(want, "\n"))
	}
}

// Every level, state and kind is on the page at 0 when there is nothing of it, so a graph of critical alerts shows none rather than nothing known,
// and an alert on a failed task has a reading to watch before the first failure.
func TestEveryKindIsWrittenWhenThereIsNoneOfIt(t *testing.T) {
	t.Parallel()

	got := write(t, &fixed{
		System: answered(poll.System{State: "READY"}), Alerts: answered([]poll.Alert{}), Jobs: answered([]poll.Job{}), Apps: answered(poll.Apps{Status: "UNCONFIGURED"}),
		VMs: answered([]poll.VM{}), Shares: answered([]poll.Share{}), Tasks: answered([]poll.Task{}), Pools: answered([]poll.Pool{{Name: "tank", Healthy: true}}),
	})

	wants(t, got,
		`truenas_system_state{state="BOOTING"} 0`, `truenas_system_state{state="READY"} 1`, `truenas_system_state{state="SHUTTING_DOWN"} 0`,
		`truenas_alerts{level="INFO"} 0`, `truenas_alerts{level="NOTICE"} 0`, `truenas_alerts{level="WARNING"} 0`, `truenas_alerts{level="ERROR"} 0`,
		`truenas_alerts{level="CRITICAL"} 0`, `truenas_alerts{level="ALERT"} 0`, `truenas_alerts{level="EMERGENCY"} 0`, `truenas_alerts_dismissed 0`,
		`truenas_jobs{state="RUNNING"} 0`, `truenas_jobs{state="WAITING"} 0`,
		`truenas_apps_service_status{status="UNCONFIGURED"} 1`,
		`truenas_apps{state="RUNNING"} 0`, `truenas_apps{state="DEPLOYING"} 0`, `truenas_apps{state="STOPPING"} 0`, `truenas_apps{state="STOPPED"} 0`, `truenas_apps{state="CRASHED"} 0`,
		`truenas_vms{state="RUNNING"} 0`, `truenas_vms{state="STOPPED"} 0`, `truenas_vms{state="SUSPENDED"} 0`,
		`truenas_shares{protocol="smb",state="enabled"} 0`, `truenas_shares{protocol="smb",state="disabled"} 0`, `truenas_shares{protocol="smb",state="locked"} 0`,
		`truenas_shares{protocol="nfs",state="enabled"} 0`, `truenas_shares{protocol="iscsi",state="locked"} 0`,
		`truenas_tasks{kind="snapshot"} 0`, `truenas_tasks{kind="replication"} 0`, `truenas_tasks{kind="cloud_sync"} 0`, `truenas_tasks{kind="cloud_backup"} 0`, `truenas_tasks{kind="rsync"} 0`, `truenas_tasks{kind="scrub"} 0`,
		`truenas_tasks_failed{kind="rsync"} 0`, `truenas_tasks_running{kind="cloud_sync"} 0`, `truenas_tasks_enabled{kind="scrub"} 0`,
		`truenas_pool_scan_running{function="SCRUB",pool="tank"} 0`, `truenas_pool_scan_running{function="RESILVER",pool="tank"} 0`,
	)

	// a kind is named only while there is one of it, and a scrub's runs are the pool's scans
	absent(t, got, "truenas_alerts_by_class", "truenas_jobs_running", "truenas_app_state", "truenas_vm_state", "truenas_task_", `truenas_tasks_failed{kind="scrub"}`, `truenas_tasks_running{kind="scrub"}`)
}

// A NAS newer than this may have a state, a level or a kind of scan this has not heard of. It is on the page under its own name rather than left
// out, so the counts still add up to what there is.
func TestWhatThisHasNotHeardOfIsStillOnThePage(t *testing.T) {
	t.Parallel()

	got := write(t, &fixed{
		System: answered(poll.System{State: "UPGRADING"}),
		Alerts: answered([]poll.Alert{{Level: "FATAL", Class: "Meltdown"}}),
		Jobs:   answered([]poll.Job{{Method: "pool.expand", State: "PAUSED"}}),
		Apps:   answered(poll.Apps{List: []poll.App{{Name: "plex", State: "UPGRADING"}}}),
		VMs:    answered([]poll.VM{{Name: "scratch", State: "ERROR"}}),
		Shares: answered([]poll.Share{{Protocol: "nvmet", Enabled: true}}),
		Pools:  answered([]poll.Pool{{Name: "tank", Scan: &poll.Scan{Function: "REBUILD", State: poll.ScanRunning}}}),
	})

	wants(t, got,
		`truenas_system_state{state="UPGRADING"} 1`, `truenas_system_state{state="READY"} 0`,
		`truenas_alerts{level="FATAL"} 1`, `truenas_alerts_by_class{class="Meltdown",level="FATAL"} 1`, `truenas_alerts{level="CRITICAL"} 0`,
		`truenas_jobs{state="PAUSED"} 1`, `truenas_jobs{state="RUNNING"} 0`,
		`truenas_apps{state="UPGRADING"} 1`, `truenas_app_state{app="plex",state="UPGRADING"} 1`,
		`truenas_vms{state="ERROR"} 1`, `truenas_vm_state{state="ERROR",vm="scratch"} 1`,
		`truenas_shares{protocol="nvmet",state="enabled"} 1`,
		`truenas_pool_scan_running{function="REBUILD",pool="tank"} 1`, `truenas_pool_scan_running{function="SCRUB",pool="tank"} 0`,
	)

	// with no service that runs apps named, there is no status to give for it
	absent(t, got, "truenas_apps_service_status", "truenas_jobs_running")
}

// The pool is what the page is for: whether it is healthy, how full, and which disk the errors are against.
func TestAPoolItsVdevsAndItsDisks(t *testing.T) {
	t.Parallel()

	got := write(t, tank())

	wants(t, got,
		`truenas_pool_healthy{pool="tank"} 0`, `truenas_pool_warning{pool="tank"} 1`, `truenas_pool_status{pool="tank",status="DEGRADED"} 1`,
		`truenas_pool_healthy{pool="boot-pool"} 1`, `truenas_pool_status{pool="boot-pool",status="ONLINE"} 1`,
		`truenas_pool_fragmentation_ratio{pool="tank"} 0.14`, `truenas_pool_freeing_bytes{pool="tank"} 4.294967296e+09`,
		`truenas_pool_vdev_info{class="data",pool="tank",type="RAIDZ2",vdev="raidz2-0"} 1`,
		`truenas_pool_vdev_status{class="data",pool="tank",status="DEGRADED",vdev="raidz2-0"} 1`,
		`truenas_pool_vdev_checksum_errors{class="data",pool="tank",vdev="raidz2-0"} 7`,
		`truenas_pool_vdev_fragmentation_ratio{class="special",pool="tank",vdev="mirror-1"} 0.02`,
		`truenas_pool_device_status{class="data",disk="sdd",pool="tank",serial="ZR5A0004",status="FAULTED",vdev="raidz2-0"} 1`,
		`truenas_pool_device_read_errors{class="data",disk="sdd",pool="tank",serial="ZR5A0004",vdev="raidz2-0"} 3`,
		`truenas_pool_device_write_errors{class="data",disk="sdd",pool="tank",serial="ZR5A0004",vdev="raidz2-0"} 1`,
		`truenas_pool_device_checksum_errors{class="data",disk="sdd",pool="tank",serial="ZR5A0004",vdev="raidz2-0"} 7`,
		`truenas_pool_device_checksum_errors{class="data",disk="sda",pool="tank",serial="ZR5A0001",vdev="raidz2-0"} 0`,
		`truenas_pool_device_status{class="spare",disk="sde",pool="tank",serial="ZR5A0005",status="ONLINE",vdev="sde1"} 1`,
		// a spare holds no data, so its errors are counted and it has no size
		`truenas_pool_vdev_read_errors{class="spare",pool="tank",vdev="sde1"} 0`,
	)

	absent(t, got, `truenas_pool_vdev_size_bytes{class="spare"`, `truenas_pool_vdev_allocated_bytes{class="spare"`, `truenas_pool_vdev_fragmentation_ratio{class="spare"`)
}

// A scan that is over has an end and nothing left to do; one under way has time left and no end. A pool never scanned says only that none is
// running, which is what an alert on a scrub that is overdue needs to be able to tell apart from a scrub that is fine.
func TestAScanRunningOverAndNeverRun(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		got := write(t, tank())

		wants(t, got,
			`truenas_pool_scan_running{function="SCRUB",pool="tank"} 0`, `truenas_pool_scan_running{function="RESILVER",pool="tank"} 0`,
			`truenas_pool_scan_progress_ratio{function="SCRUB",pool="tank"} 1`, `truenas_pool_scan_errors{function="SCRUB",pool="tank"} 2`,
			// two days before the start of 2000, where the test's clock begins
			`truenas_pool_scan_end_timestamp_seconds{function="SCRUB",pool="tank"} 9.46512e+08`,
			`truenas_pool_scan_running{function="RESILVER",pool="boot-pool"} 1`, `truenas_pool_scan_running{function="SCRUB",pool="boot-pool"} 0`,
			`truenas_pool_scan_progress_ratio{function="RESILVER",pool="boot-pool"} 0.25`, `truenas_pool_scan_remaining_seconds{function="RESILVER",pool="boot-pool"} 1800`,
			`truenas_pool_scan_paused{function="RESILVER",pool="boot-pool"} 0`,
		)
		absent(t, got, `truenas_pool_scan_end_timestamp_seconds{function="RESILVER"`, `truenas_pool_scan_remaining_seconds{function="SCRUB"`)

		never := write(t, &fixed{Pools: answered([]poll.Pool{{Name: "new", Healthy: true}})})
		wants(t, never, `truenas_pool_scan_running{function="SCRUB",pool="new"} 0`, `truenas_pool_scan_running{function="RESILVER",pool="new"} 0`)
		absent(t, never, "truenas_pool_scan_progress_ratio", "truenas_pool_scan_errors", "truenas_pool_scan_start_timestamp_seconds", "truenas_pool_scan_end_timestamp_seconds", "truenas_pool_scan_paused")
	})
}

// A pool names its disks by the kernel's name alone, which moves when the NAS restarts; the serial number from the list of disks is put beside it so
// that another exporter's readings of the same disk can be joined on. And a disk says which pool it is in, which the list of disks does not.
func TestADiskIsJoinedToItsSerialAndItsPool(t *testing.T) {
	t.Parallel()

	got := write(t, tank())

	wants(t, got,
		`truenas_disk_info{bus="SCSI",disk="sda",enclosure="",model="ST20000NM007D",pool="tank",serial="ZR5A0001",slot="",type="HDD",wwn="5000c500e0000001"} 1`,
		`truenas_disk_info{bus="SCSI",disk="sdb",enclosure="0",model="ST20000NM007D",pool="tank",serial="ZR5A0002",slot="2",type="HDD",wwn="5000c500e0000002"} 1`,
		`truenas_disk_info{bus="NVME",disk="nvme0n1",enclosure="",model="SAMSUNG MZQL21T9HCJR",pool="tank",serial="S6B0NA0T000001",slot="",type="SSD",wwn=""} 1`,
		// a disk in no pool
		`truenas_disk_info{bus="SCSI",disk="sdh",enclosure="",model="ST20000NM007D",pool="",serial="ZR5A0008",slot="",type="HDD",wwn="5000c500e0000008"} 1`,
		`truenas_disk_temperature_celsius{disk="sda",serial="ZR5A0001"} 34`,
		`truenas_disk_size_bytes{disk="sdd",serial="ZR5A0004"} 2.199023255552e+13`,
		// a disk the pool names and the list of disks does not has no serial to give
		`truenas_pool_device_status{class="data",disk="sdf",pool="boot-pool",serial="",status="ONLINE",vdev="mirror-0"} 1`,
	)

	// a disk that gives no temperature has none, which is not one of 0
	absent(t, got, `truenas_disk_temperature_celsius{disk="sdd"`)

	// until the disks have been asked for, the pool's readings are there without a serial rather than not there
	early := tank()
	early.Disks = poll.Answer[[]poll.Disk]{}
	wants(t, write(t, early), `truenas_pool_device_read_errors{class="data",disk="sdd",pool="tank",serial="",vdev="raidz2-0"} 3`)
	absent(t, write(t, early), "truenas_disk_")
}

// Only the datasets no deeper than asked for are named, since the names further down are often people's. Every dataset and snapshot is still in
// its pool's count, and a deeper one's space is in the dataset above it.
func TestOnlyDatasetsNoDeeperThanAskedAreNamed(t *testing.T) {
	t.Parallel()

	shallow := page(t, tank(), 0, nil)
	wants(t, shallow, `truenas_dataset_used_bytes{dataset="tank",pool="tank"}`, `truenas_dataset_used_bytes{dataset="boot-pool",pool="boot-pool"}`, `truenas_datasets{pool="tank"} 4`, `truenas_snapshots{pool="tank"} 132`)
	absent(t, shallow, `truenas_dataset_used_bytes{dataset="tank/media"`, `truenas_dataset_snapshots{dataset="tank/backups"`)

	one := write(t, tank())
	wants(t, one,
		`truenas_dataset_used_bytes{dataset="tank/media",pool="tank"} 1.649267441664e+13`,
		`truenas_dataset_available_bytes{dataset="tank/backups",pool="tank"} 1.099511627776e+12`,
		`truenas_dataset_quota_bytes{dataset="tank/backups",pool="tank"} 5.49755813888e+12`,
		`truenas_dataset_refquota_bytes{dataset="tank/backups",pool="tank"} 4.398046511104e+12`,
		`truenas_dataset_compression_ratio{dataset="tank/backups",pool="tank"} 1.8`,
		`truenas_dataset_snapshots{dataset="tank/media",pool="tank"} 12`,
		`truenas_dataset_used_by_snapshots_bytes{dataset="tank/media",pool="tank"} 5.36870912e+10`,
		`truenas_datasets{pool="tank"} 4`, `truenas_snapshots{pool="tank"} 132`, `truenas_datasets{pool="boot-pool"} 1`,
	)
	// no quota is not a quota of 0, and a pool whose datasets gave no count of snapshots has no count
	absent(t, one, `truenas_dataset_used_bytes{dataset="tank/media/films"`, `truenas_dataset_quota_bytes{dataset="tank/media"`, `truenas_snapshots{pool="boot-pool"}`, `truenas_dataset_snapshots{dataset="boot-pool"`)

	wants(t, page(t, tank(), 2, nil), `truenas_dataset_used_bytes{dataset="tank/media/films",pool="tank"}`, `truenas_dataset_snapshots{dataset="tank/media/films",pool="tank"} 30`)
}

// Alerts are counted by level and by kind, and one somebody has dismissed is in neither: it is counted apart.
func TestAlertsAreCountedByLevelAndKind(t *testing.T) {
	t.Parallel()

	got := write(t, tank())

	wants(t, got,
		`truenas_alerts{level="CRITICAL"} 3`, `truenas_alerts{level="INFO"} 1`, `truenas_alerts{level="WARNING"} 0`,
		`truenas_alerts_by_class{class="RsyncFailed",level="CRITICAL"} 2`, `truenas_alerts_by_class{class="VolumeStatus",level="CRITICAL"} 1`, `truenas_alerts_by_class{class="RsyncSuccess",level="INFO"} 1`,
		`truenas_alerts_dismissed 1`,
	)
	absent(t, got, `truenas_alerts_by_class{class="ScrubPaused"`)
}

// Services, jobs, apps, virtual machines and clients: what is running and what is not.
func TestWhatIsRunning(t *testing.T) {
	t.Parallel()

	wants(t, write(t, tank()),
		`truenas_service_running{service="cifs"} 1`, `truenas_service_enabled{service="cifs"} 1`,
		// a service meant to start with the NAS that is not running is the one to alert on
		`truenas_service_running{service="ssh"} 0`, `truenas_service_enabled{service="ssh"} 1`, `truenas_service_enabled{service="ftp"} 0`,
		`truenas_jobs{state="RUNNING"} 2`, `truenas_jobs{state="WAITING"} 1`, `truenas_jobs_running{method="cloudsync.sync"} 1`, `truenas_jobs_running{method="pool.scrub.run"} 1`,
		`truenas_apps_service_status{status="RUNNING"} 1`, `truenas_apps{state="RUNNING"} 1`, `truenas_apps{state="STOPPED"} 1`, `truenas_apps{state="CRASHED"} 0`,
		`truenas_app_state{app="plex",state="RUNNING"} 1`, `truenas_app_update_available{app="plex"} 1`, `truenas_app_update_available{app="syncthing"} 0`,
		`truenas_vms{state="RUNNING"} 1`, `truenas_vm_state{state="STOPPED",vm="scratch"} 1`,
		`truenas_share_clients{protocol="nfs"} 3`, `truenas_share_clients{protocol="iscsi"} 0`,
	)
}

// A share whose dataset is locked serves nothing whether it is switched on or not, so it is counted as locked and not as enabled.
func TestSharesAreCountedByProtocolAndState(t *testing.T) {
	t.Parallel()

	wants(t, write(t, tank()),
		`truenas_shares{protocol="smb",state="enabled"} 2`, `truenas_shares{protocol="smb",state="disabled"} 1`, `truenas_shares{protocol="smb",state="locked"} 0`,
		`truenas_shares{protocol="nfs",state="locked"} 1`, `truenas_shares{protocol="nfs",state="enabled"} 0`, `truenas_shares{protocol="iscsi",state="enabled"} 0`,
	)
}

// Each task is on the page by kind, number and name with whether it is on, whether it is running and how its last run went, so a count of failed
// tasks can be traced to the one. A task that has never run has no last run, a run stopped by hand is not one that failed, and a scrub's runs are
// the pool's scans.
func TestEachTaskAndItsLastRun(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		got := write(t, tank())

		wants(t, got,
			`truenas_tasks{kind="snapshot"} 2`, `truenas_tasks_enabled{kind="snapshot"} 1`, `truenas_tasks_failed{kind="snapshot"} 1`,
			`truenas_tasks{kind="cloud_sync"} 2`, `truenas_tasks_running{kind="cloud_sync"} 1`, `truenas_tasks_failed{kind="cloud_sync"} 0`,
			`truenas_tasks{kind="cloud_backup"} 0`, `truenas_tasks_failed{kind="rsync"} 1`, `truenas_tasks{kind="scrub"} 1`, `truenas_tasks_enabled{kind="scrub"} 1`,
			`truenas_task_enabled{id="2",kind="snapshot",name="tank/backups"} 0`,
			`truenas_task_last_run_failed{id="2",kind="snapshot",name="tank/backups"} 1`,
			`truenas_task_last_run_failed{id="1",kind="snapshot",name="tank/media"} 0`,
			// an hour before the start of 2000, where the test's clock begins
			`truenas_task_last_run_timestamp_seconds{id="1",kind="snapshot",name="tank/media"} 9.466812e+08`,
			`truenas_task_running{id="3",kind="cloud_sync",name="photos to the cloud"} 1`,
			`truenas_task_progress_ratio{id="3",kind="cloud_sync",name="photos to the cloud"} 0.87`,
			`truenas_task_last_run_duration_seconds{id="3",kind="cloud_sync",name="photos to the cloud"} 1500`,
			// stopped by hand: over, and not failed
			`truenas_task_last_run_failed{id="4",kind="cloud_sync",name="documents to the cloud"} 0`,
			`truenas_task_running{id="4",kind="cloud_sync",name="documents to the cloud"} 0`,
			`truenas_task_last_run_failed{id="7",kind="rsync",name="media to the old NAS"} 1`,
			`truenas_task_enabled{id="1",kind="replication",name="tank to offsite"} 1`,
			`truenas_task_running{id="1",kind="replication",name="tank to offsite"} 0`,
			`truenas_task_enabled{id="1",kind="scrub",name="tank"} 1`,
		)

		absent(t, got,
			`truenas_task_last_run_failed{id="1",kind="replication"`, `truenas_task_last_run_timestamp_seconds{id="1",kind="replication"`,
			`truenas_task_progress_ratio{id="4"`, `truenas_task_last_run_duration_seconds{id="1",kind="snapshot"`,
			`truenas_task_running{id="1",kind="scrub"`, `truenas_task_last_run_failed{id="1",kind="scrub"`,
		)
	})
}

// Two readings with the same labels would lose the whole page, and the names are the NAS's own: two tasks can share a number and a name after a
// restore, and two disks can go missing from one vdev. The first of each is kept and the page is still written.
func TestTwoOfOneNameDoNotSpoilThePage(t *testing.T) {
	t.Parallel()

	got := write(t, &fixed{
		Tasks:    answered([]poll.Task{{Kind: poll.RsyncTask, ID: 1, Name: "copy", Enabled: true}, {Kind: poll.RsyncTask, ID: 1, Name: "copy"}}),
		Pools:    answered([]poll.Pool{{Name: "tank", Vdevs: []poll.Vdev{{Class: "data", Name: "mirror-0", Devices: []poll.Device{{Disk: "sda", Errors: poll.Errors{Read: 1}}, {Disk: "sda", Errors: poll.Errors{Read: 5}}}}}}, {Name: "tank"}}),
		Disks:    answered([]poll.Disk{{Name: "sda", Serial: "A"}, {Name: "sda", Serial: "A"}}),
		Datasets: answered([]poll.Dataset{{Name: "tank", Pool: "tank"}, {Name: "tank", Pool: "tank"}}),
		Services: answered([]poll.Service{{Name: "ssh", Running: true}, {Name: "ssh"}}),
	})

	wants(t, got,
		`truenas_tasks{kind="rsync"} 2`, `truenas_task_enabled{id="1",kind="rsync",name="copy"} 1`,
		`truenas_pool_device_read_errors{class="data",disk="sda",pool="tank",serial="A",vdev="mirror-0"} 1`,
		`truenas_service_running{service="ssh"} 1`, `truenas_datasets{pool="tank"} 2`,
	)
}

// The small things: when a certificate and the key this is asked with run out, which interfaces have a link, and whether there is an update.
func TestTheRest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		wants(t, write(t, tank()),
			// 72 days and 30 days after the start of 2000, where the test's clock begins
			`truenas_certificate_expiry_timestamp_seconds{certificate="truenas_default"} 9.529056e+08`,
			`truenas_api_key_expiry_timestamp_seconds 9.492768e+08`,
			`truenas_interface_link_up{interface="eno1",type="PHYSICAL"} 1`, `truenas_interface_link_up{interface="eno2",type="PHYSICAL"} 0`, `truenas_interface_mtu_bytes{interface="eno1"} 9000`,
			`truenas_update_available 1`, `truenas_update_check_failed 0`,
			`truenas_directory_service_healthy{status="HEALTHY",type="ACTIVEDIRECTORY"} 1`,
			`truenas_system_info{cpu="Intel(R) Xeon(R) CPU E3-1230 v6",manufacturer="Supermicro",product="X11SSH-F",version="25.10.7"} 1`,
			`truenas_system_uptime_seconds 864000`, `truenas_system_boot_timestamp_seconds 9.458208e+08`, `truenas_system_memory_bytes 6.8719476736e+10`,
			`truenas_system_cpu_cores 8`, `truenas_system_cpu_physical_cores 4`, `truenas_system_ecc_memory 1`, `truenas_system_clock_offset_seconds -0.02`,
			`truenas_system_load_average{window="1m"} 0.5`, `truenas_system_load_average{window="5m"} 0.75`, `truenas_system_load_average{window="15m"} 1.25`,
			`truenas_up 1`, `truenas_poll_duration_seconds{poll="tasks"} 3`, `truenas_poll_failures_total{poll="tasks"} 2`, `truenas_poll_last_success_timestamp_seconds{poll="system"} 9.466848e+08`, `truenas_poll_interval_seconds{poll="snapshots"} 600`,
		)

		// a NAS joined to no directory has no directory to be healthy, a key that never expires has no end, and a faulted directory is not healthy
		absent(t, write(t, &fixed{Directory: answered(poll.Directory{}), Key: answered(poll.Key{}), Certificates: answered([]poll.Certificate{{Name: "unread"}})}), "truenas_directory_service_healthy", "truenas_api_key_expiry_timestamp_seconds", "truenas_certificate_expiry_timestamp_seconds")
		wants(t, write(t, &fixed{Directory: answered(poll.Directory{Type: "LDAP", Status: "FAULTED"})}), `truenas_directory_service_healthy{status="FAULTED",type="LDAP"} 0`)
	})
}
