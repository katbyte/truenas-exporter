package nas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/katbyte/truenas-mcp/sdk/truenas"
	"github.com/katbyte/truenas-metrics/lib/nas"
	"github.com/katbyte/truenas-metrics/lib/poll"
)

// canned is a NAS that answers each method with the file of its name in testdata, or with what a test says in its place, and remembers what it was
// asked.
type canned struct {
	// instead is the answer to give for a method in place of its file, as JSON.
	instead map[string]string
	// failing is the methods that fail, and with what.
	failing map[string]error

	mu    sync.Mutex
	asked map[string][]any
}

func (c *canned) Call(_ context.Context, method string, params []any, into any) error {
	c.mu.Lock()
	if c.asked == nil {
		c.asked = map[string][]any{}
	}
	c.asked[method] = params
	c.mu.Unlock()

	if err := c.failing[method]; err != nil {
		return err
	}

	answer := []byte(c.instead[method])
	if _, ok := c.instead[method]; !ok {
		var err error
		if answer, err = os.ReadFile(filepath.Join("testdata", method+".json")); err != nil { //nolint:gosec // G304: the file is one of the test's own, named for the method
			return err
		}
	}

	// a field that is read but was not asked for is missing here, as it would be from the NAS
	answer, err := picked(answer, fields(params))
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}

	return json.Unmarshal(answer, into)
}

// fields is the fields a query names in its options, or none for a call that is not a query or names none.
func fields(params []any) []string {
	if len(params) < 2 {
		return nil
	}

	options, err := json.Marshal(params[1])
	if err != nil {
		return nil
	}

	var named struct {
		Select []string `json:"select"`
	}
	if json.Unmarshal(options, &named) != nil {
		return nil
	}

	return named.Select
}

// picked is a query's answer cut down to the fields named, as the NAS cuts it: a dotted name is a field inside a record, a field a row does not
// have is left out, and a record that is nothing stays nothing. With no fields named it is the whole answer.
func picked(answer []byte, names []string) ([]byte, error) {
	if len(names) == 0 {
		return answer, nil
	}

	var rows []map[string]any
	d := json.NewDecoder(bytes.NewReader(answer))
	d.UseNumber()
	if err := d.Decode(&rows); err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		entry := map[string]any{}
		for _, name := range names {
			if err := pick(row, entry, strings.Split(name, ".")); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
		}

		out = append(out, entry)
	}

	return json.Marshal(out)
}

// pick copies the field at path from row into entry. The walk stops at a value that is not a record, and that value is what is copied; one
// that is a list cannot be walked into, and the NAS turns the query down.
func pick(row, entry map[string]any, path []string) error {
	var keys []string
	var at any = row
	for _, key := range path {
		if _, ok := at.([]any); ok {
			return errors.New("a list cannot be picked into")
		}

		record, ok := at.(map[string]any)
		if !ok {
			break
		}

		if at, ok = record[key]; !ok {
			return nil
		}
		keys = append(keys, key)
	}

	for _, key := range keys[:len(keys)-1] {
		next, ok := entry[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			entry[key] = next
		}
		entry = next
	}
	entry[keys[len(keys)-1]] = at

	return nil
}

// methods is every method the NAS was asked, in order of name.
func (c *canned) methods() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]string, 0, len(c.asked))
	for method := range c.asked {
		out = append(out, method)
	}
	slices.Sort(out)

	return out
}

// of is the NAS behind these canned answers, with a missing method taken as a failure like any other.
func of(c *canned) *nas.NAS {
	return nas.New(c, nas.Options{})
}

// at is a moment the NAS gives as milliseconds since 1970.
func at(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}

// same fails the test when what was got is not what was wanted.
func same[T any](t *testing.T, what string, got, want T) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n got %+v\nwant %+v", what, got, want)
	}
}

// The NAS says who it is and how long it has been up. Its clock is compared with this machine's, which under the test stands at the start of 2000.
func TestSystem(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		got, err := of(&canned{}).System(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		same(t, "system", got, poll.System{
			Version: "25.10.7", Product: "Standard PC (Q35 + ICH9, 2009)", Manufacturer: "QEMU", CPU: "AMD EPYC 7543 32-Core Processor", State: "READY",
			Booted: at(1790938328000), Uptime: time.Duration(911259.272104045 * float64(time.Second)), Memory: 270262870016, Cores: 32, PhysicalCores: 32,
			Load: [3]float64{0.35546875, 0.36083984375, 0.48193359375}, ECC: true, ClockOffset: time.Until(at(1791849587000)),
		})
	})
}

// A pool's health, size and last scrub, and under it each vdev with its disks. A vdev of one disk goes by the disk's name, not the long number
// zpool lists it under.
func TestPools(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{}).Pools(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("%d pools, want the one", len(got))
	}

	p := got[0]
	vdevs := p.Vdevs
	p.Vdevs = nil

	online := func(disks ...string) []poll.Device {
		out := make([]poll.Device, 0, len(disks))
		for _, d := range disks {
			out = append(out, poll.Device{Disk: d, Status: "ONLINE"})
		}

		return out
	}

	same(t, "pool", p, poll.Pool{
		Name: "tank", Status: "ONLINE", Healthy: true, Size: 1119972851974144, Allocated: 733514178957312, Free: 386458673016832, Fragmentation: new(0.14),
		// the NAS said the scrub was 100.44% done, which is done
		Scan: &poll.Scan{Function: "SCRUB", State: "FINISHED", Started: at(1790845202000), Ended: at(1790961133000), Progress: 1, ToExamine: 790761222291456, Examined: 771385897451520, Issued: 774781332185088},
	})
	same(t, "vdevs", vdevs, []poll.Vdev{
		{Class: "data", Name: "raidz2-0", Type: "RAIDZ2", Status: "ONLINE", Size: 159996121710592, Allocated: 104953124855808, Fragmentation: 0.15, Devices: online("sdf", "sdg", "sdl", "sdm")},
		{Class: "data", Name: "raidz2-1", Type: "RAIDZ2", Status: "ONLINE", Size: 159996121710592, Allocated: 105047392346112, Fragmentation: 0.15, Devices: online("sdd", "sde", "sdj", "sdk")},
		{Class: "cache", Name: "nvme1n1", Type: "DISK", Status: "ONLINE", Size: 7681495003136, Allocated: 7459674374144, Devices: online("nvme1n1")},
		{Class: "spare", Name: "sdae", Type: "DISK", Status: "ONLINE", Devices: online("sdae")},
	})
}

// The boot pool is asked for by itself and comes back as a pool like any other.
func TestBootPool(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{}).BootPool(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if got.Name != "boot-pool" || got.Status != "ONLINE" || !got.Healthy || got.Size != 50465865728 || got.Fragmentation == nil || *got.Fragmentation != 0.49 {
		t.Errorf("boot pool = %+v", got)
	}
	if got.Scan == nil || got.Scan.Function != "SCRUB" || got.Scan.State != "FINISHED" || got.Scan.Left != nil {
		t.Errorf("boot pool's scan = %+v, want a scrub that is over", got.Scan)
	}
	same(t, "boot pool's vdevs", got.Vdevs, []poll.Vdev{{Class: "data", Name: "sda", Type: "DISK", Status: "ONLINE", Size: 50465865728, Allocated: 21929115648, Fragmentation: 0.49, Devices: []poll.Device{{Disk: "sda", Status: "ONLINE"}}}})
}

// A pool that is not imported has a name and a status and nothing else, and one never scrubbed has a scan with nothing in it. Neither may lose the
// answer: they are the pools most worth hearing about.
func TestAPoolWithLittleToSay(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{instead: map[string]string{"pool.query": `[
		{"name": "gone", "status": "OFFLINE", "healthy": false, "warning": false, "size": null, "allocated": null, "free": null, "freeing": null, "fragmentation": null, "scan": null, "topology": null},
		{"name": "new", "status": "ONLINE", "healthy": true, "fragmentation": "-", "scan": {"function": null, "state": null, "start_time": null, "end_time": null, "percentage": null, "errors": null, "total_secs_left": null}, "topology": {"data": [], "log": [], "cache": [], "spare": [], "special": [], "dedup": []}}
	]`}}).Pools(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	same(t, "pools", got, []poll.Pool{{Name: "gone", Status: "OFFLINE"}, {Name: "new", Status: "ONLINE", Healthy: true}})
}

// A resilver under way: how far it has got and how long is left. And the disks of a vdev however deep: one being replaced sits beside its
// replacement a level down, and one the NAS can no longer find has no name, so it goes by the number the pool knows it by.
func TestAResilverAndTheDisksUnderIt(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{instead: map[string]string{"pool.query": `[{"name": "tank", "status": "DEGRADED", "healthy": false, "warning": true, "size": 1000, "allocated": 600, "free": 400, "freeing": 0, "fragmentation": "5",
		"scan": {"function": "RESILVER", "state": "SCANNING", "start_time": {"$date": 1791849587000}, "end_time": null, "percentage": 25.5, "bytes_to_process": 600, "bytes_processed": 200, "bytes_issued": 153, "pause": {"$date": 1791849600000}, "errors": 3, "total_secs_left": 5400},
		"topology": {"data": [{"name": "mirror-0", "type": "MIRROR", "guid": "1", "status": "DEGRADED", "disk": null, "stats": {"read_errors": 0, "write_errors": 0, "checksum_errors": 9, "size": 1000, "allocated": 600, "fragmentation": 5}, "children": [
			{"name": "replacing-0", "type": "REPLACING", "guid": "2", "status": "DEGRADED", "disk": null, "stats": {"read_errors": 0, "write_errors": 0, "checksum_errors": 0}, "children": [
				{"name": "3333", "type": "DISK", "guid": "3333", "status": "UNAVAIL", "disk": null, "stats": {"read_errors": 4, "write_errors": 2, "checksum_errors": 0}, "children": []},
				{"name": "sdc1", "type": "DISK", "guid": "4", "status": "ONLINE", "disk": "sdc", "stats": {"read_errors": 0, "write_errors": 0, "checksum_errors": 0}, "children": []}]},
			{"name": "sdb1", "type": "DISK", "guid": "5", "status": "ONLINE", "disk": "sdb", "stats": {"read_errors": 0, "write_errors": 0, "checksum_errors": 9}, "children": []}]}],
		"log": [], "cache": [], "spare": [], "special": [], "dedup": []}}]`}}).Pools(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	same(t, "scan", got[0].Scan, &poll.Scan{Function: "RESILVER", State: "SCANNING", Started: at(1791849587000), Progress: 0.255, ToExamine: 600, Examined: 200, Issued: 153, Errors: 3, Paused: true, Left: new(90 * time.Minute)})
	same(t, "vdevs", got[0].Vdevs, []poll.Vdev{{Class: "data", Name: "mirror-0", Type: "MIRROR", Status: "DEGRADED", Size: 1000, Allocated: 600, Fragmentation: 0.05, Errors: poll.Errors{Checksum: 9}, Devices: []poll.Device{
		{Disk: "3333", Status: "UNAVAIL", Errors: poll.Errors{Read: 4, Write: 2}},
		{Disk: "sdc", Status: "ONLINE"},
		{Disk: "sdb", Status: "ONLINE", Errors: poll.Errors{Checksum: 9}},
	}}})
}

// Alerts keep their level and kind and nothing of what they say, which names paths and hosts.
func TestAlerts(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{}).Alerts(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	same(t, "alerts", got, []poll.Alert{{Level: "CRITICAL", Class: "RsyncFailed"}, {Level: "CRITICAL", Class: "RsyncFailed"}, {Level: "INFO", Class: "RsyncSuccess"}, {Level: "INFO", Class: "RsyncSuccess"}})
}

func TestServices(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{}).Services(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	same(t, "services", got, []poll.Service{
		{Name: "cifs", Running: true, Enabled: true},
		{Name: "ftp"},
		{Name: "iscsitarget"},
		{Name: "nfs", Running: true, Enabled: true},
		{Name: "snmp"},
		{Name: "ssh", Running: true, Enabled: true},
		{Name: "ups"},
		{Name: "nvmet"},
	})
}

// Only the jobs that are not over are asked for: the NAS keeps every job it has run, and they are not what the page counts.
func TestJobs(t *testing.T) {
	t.Parallel()

	c := &canned{}
	got, err := nas.New(c, nas.Options{}).Jobs(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	same(t, "jobs", got, []poll.Job{{Method: "cloudsync.sync", State: "RUNNING"}})

	asked, _ := json.Marshal(c.asked["core.get_jobs"])
	if string(asked) != `[[["state","in",["RUNNING","WAITING"]]],{"select":["method","state"]}]` {
		t.Errorf("core.get_jobs was asked with %s, want only the jobs running or waiting", asked)
	}
}

// The apps and the state of what runs them. A NAS never set up for apps says so and has none.
func TestApps(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{instead: map[string]string{"docker.status": `{"status": "RUNNING", "description": "Docker is running"}`}}).Apps(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "apps", got, poll.Apps{Status: "RUNNING", List: []poll.App{{Name: "plex", State: "RUNNING", UpdateAvailable: true}, {Name: "syncthing", State: "STOPPED"}}})

	got, err = of(&canned{instead: map[string]string{"app.query": `[]`}}).Apps(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "apps on a NAS without them", got, poll.Apps{Status: "UNCONFIGURED", List: []poll.App{}})
}

func TestVMs(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{}).VMs(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	same(t, "virtual machines", got, []poll.VM{{Name: "homeassistant", State: "RUNNING"}, {Name: "scratch", State: "STOPPED"}})
}

func TestClients(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{instead: map[string]string{"nfs.client_count": `4`}}).Clients(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	same(t, "clients", got, []poll.Clients{{Protocol: "nfs", Count: 4}, {Protocol: "iscsi"}})
}

// Each disk with its serial number, which is what its readings are joined on, and its temperature. A disk that gives no temperature has none,
// and one the NAS knows nothing more about than its name is still a disk.
func TestDisks(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{}).Disks(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 12 {
		t.Fatalf("%d disks, want 12", len(got))
	}

	same(t, "the first disk", got[0], poll.Disk{Name: "sdj", Serial: "ZR5A0001", WWN: "5000c500e0000001", Model: "WUH722020BL5204", Type: "HDD", Bus: "SCSI", Size: 20000588955648, Temperature: new(39.0)})
	same(t, "the cache disk", got[8], poll.Disk{Name: "nvme1n1", Serial: "ZR5A0009", WWN: "5000c500e0000009", Model: "SAMSUNG MZQL27T6HBLA-00A07", Type: "SSD", Bus: "UNKNOWN", Size: 7681501126656, Temperature: new(46.85)})
	// the disk the NAS boots from is a virtual one: it has no world wide name and gives no temperature
	same(t, "the boot disk", got[11], poll.Disk{Name: "sda", Serial: "ZR5A0012", Model: "QEMU_HARDDISK", Type: "SSD", Bus: "SCSI", Size: 68719476736})

	got, err = of(&canned{instead: map[string]string{
		"disk.query":        `[{"name": "sdz", "serial": "", "lunid": null, "model": null, "type": null, "bus": "UNKNOWN", "size": null, "enclosure": {"number": 1, "slot": 12}}]`,
		"disk.temperatures": `{}`,
	}}).Disks(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "a disk with little to say", got, []poll.Disk{{Name: "sdz", Bus: "UNKNOWN", Slot: &poll.Slot{Enclosure: 1, Slot: 12}}})
}

// Every dataset as one flat list, with only the properties the page reports asked for and a count of each one's snapshots. No quota is no
// quota, not one of 0.
func TestDatasets(t *testing.T) {
	t.Parallel()

	c := &canned{}
	got, err := nas.New(c, nas.Options{}).Datasets(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("%d datasets, want 6", len(got))
	}

	same(t, "the pool's own dataset", got[0], poll.Dataset{Name: "tank", Pool: "tank", Type: "FILESYSTEM", Used: 566789391307776, Available: 274593752252000, UsedByDataset: 141697347826944, UsedBySnapshots: 35424336956736, Compression: 1.04, Snapshots: new(0)})
	if d := got[1]; d.Name != "tank/dataset-5" || d.Depth() != 1 || d.Quota != nil || d.RefQuota != nil || d.Snapshots == nil || *d.Snapshots != 12 {
		t.Errorf("a dataset one deep = %+v", d)
	}
	if d := got[5]; d.Depth() != 2 || d.Quota == nil || *d.Quota != 21990232555520 || d.RefQuota != nil {
		t.Errorf("a dataset two deep with a quota = %+v", d)
	}

	// without the datasets under each, with a filter every dataset passes so the NAS goes below the pools, and of each only what is read
	asked, _ := json.Marshal(c.asked["pool.dataset.query"])
	if want := `[[["type","in",["FILESYSTEM","VOLUME"]]],{"extra":{"properties":["used","available","usedbydataset","usedbysnapshots","quota","refquota","compressratio"],"retrieve_children":false,"snapshots_count":true},"select":["name","pool","type","used.parsed","available.parsed","usedbydataset.parsed","usedbysnapshots.parsed","quota.parsed","refquota.parsed","compressratio.parsed","snapshot_count"]}]`; string(asked) != want {
		t.Errorf("pool.dataset.query was asked with\n%s\nwant\n%s", asked, want)
	}
}

// A task of every kind, and how each last went: the three the NAS runs as jobs by their job, snapshots and replication by the state the NAS keeps
// for them, and a scrub's schedule with no run of its own.
func TestTasks(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{}).Tasks(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	same(t, "tasks", got, []poll.Task{
		{Kind: poll.SnapshotTask, ID: 1, Name: "tank/dataset-2", Enabled: true, LastRun: &poll.Run{OK: true, Ended: at(1790838000000)}},
		{Kind: poll.SnapshotTask, ID: 2, Name: "tank/dataset-4", Enabled: true, LastRun: &poll.Run{OK: true, Ended: at(1790838000000)}},
		{Kind: poll.SnapshotTask, ID: 3, Name: "tank/dataset-5", Enabled: true, LastRun: &poll.Run{OK: true, Ended: at(1790838000000)}},
		{Kind: poll.ReplicationTask, ID: 1, Name: "tank to offsite", Enabled: true, LastRun: &poll.Run{OK: true, Ended: at(1791702004000)}},
		// on hold until somebody unlocks its dataset: it cannot run, which is a failure
		{Kind: poll.ReplicationTask, ID: 2, Name: "tank to the shelf", Enabled: true, LastRun: &poll.Run{Failed: true, Ended: at(1791705600000)}},
		{Kind: poll.ReplicationTask, ID: 3, Name: "never run"},
		{Kind: poll.CloudSyncTask, ID: 5, Name: "description-3", Enabled: true, Running: true, Progress: 0.87},
		{Kind: poll.CloudSyncTask, ID: 10, Name: "description-5", LastRun: &poll.Run{OK: true, Ended: at(1790067633000), Took: 31167 * time.Second}},
		{Kind: poll.CloudSyncTask, ID: 1, Name: "description-7", Enabled: true, LastRun: &poll.Run{OK: true, Ended: at(1791786287000), Took: 1486 * time.Second}},
		{Kind: poll.CloudSyncTask, ID: 2, Name: "description-9", Enabled: true, LastRun: &poll.Run{OK: true, Ended: at(1791788569000), Took: 1968 * time.Second}},
		// stopped by hand: over, and neither a success nor a failure
		{Kind: poll.CloudBackupTask, ID: 1, Name: "tank to the bucket", Enabled: true, LastRun: &poll.Run{Ended: at(1791784861000), Took: time.Minute}},
		{Kind: poll.RsyncTask, ID: 13, Name: "desc-1", LastRun: &poll.Run{Failed: true, Ended: at(1768363442000), Took: 19 * time.Second}},
		{Kind: poll.RsyncTask, ID: 15, Name: "desc-2", LastRun: &poll.Run{Failed: true, Ended: at(1790654464000), Took: 162062 * time.Second}},
		{Kind: poll.RsyncTask, ID: 1, Name: "desc-3", Enabled: true, LastRun: &poll.Run{OK: true, Ended: at(1791702004000), Took: 3 * time.Second}},
		{Kind: poll.RsyncTask, ID: 2, Name: "description-7", Enabled: true, LastRun: &poll.Run{OK: true, Ended: at(1791702936000), Took: 35 * time.Second}},
		{Kind: poll.ScrubTask, ID: 1, Name: "tank", Enabled: true},
	})
}

// A task that has never run has no job at all, one waiting its turn has not run yet, and a state this has not heard of says nothing either way.
func TestATaskWithNoRunToReport(t *testing.T) {
	t.Parallel()

	none := `[]`
	got, err := of(&canned{instead: map[string]string{
		"pool.snapshottask.query": `[{"id": 1, "dataset": "tank", "enabled": true, "state": {"state": "PENDING"}}, {"id": 2, "dataset": "tank", "enabled": true, "state": {"state": "RUNNING"}}, {"id": 3, "dataset": "tank", "enabled": true, "state": {"state": "ERROR", "datetime": {"$date": 1000}, "error": "no such dataset"}}]`,
		"replication.query":       none,
		"cloudsync.query":         `[{"id": 1, "description": "never", "enabled": true, "job": null}, {"id": 2, "description": "queued", "enabled": true, "job": {"state": "WAITING", "progress": {"percent": null}, "time_started": null, "time_finished": null}}, {"id": 3, "description": "odd", "enabled": true, "job": {"state": "PAUSED"}}]`,
		"cloud_backup.query":      none,
		"rsynctask.query":         none,
		"pool.scrub.query":        none,
	}}).Tasks(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	same(t, "tasks", got, []poll.Task{
		{Kind: poll.SnapshotTask, ID: 1, Name: "tank", Enabled: true},
		{Kind: poll.SnapshotTask, ID: 2, Name: "tank", Enabled: true, Running: true},
		{Kind: poll.SnapshotTask, ID: 3, Name: "tank", Enabled: true, LastRun: &poll.Run{Failed: true, Ended: at(1000)}},
		{Kind: poll.CloudSyncTask, ID: 1, Name: "never", Enabled: true},
		{Kind: poll.CloudSyncTask, ID: 2, Name: "queued", Enabled: true},
		{Kind: poll.CloudSyncTask, ID: 3, Name: "odd", Enabled: true},
	})
}

// Shares keep how they are offered and whether they are on, and nothing of their names or paths. An iSCSI target has no switch of its own.
func TestShares(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{instead: map[string]string{"sharing.smb.query": `[{"id": 1, "name": "films", "path": "/mnt/tank/films", "enabled": true, "locked": false}, {"id": 2, "name": "old", "path": "/mnt/tank/old", "enabled": false, "locked": false}]`}}).Shares(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	same(t, "shares", got, []poll.Share{{Protocol: "smb", Enabled: true}, {Protocol: "smb"}, {Protocol: "nfs", Enabled: true, Locked: true}, {Protocol: "iscsi", Enabled: true}})

	saved, err := of(&canned{}).Shares(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 5 || saved[0] != (poll.Share{Protocol: "smb", Enabled: true}) {
		t.Errorf("the shares of the saved answers = %+v", saved)
	}
}

// When a certificate runs out, which the NAS writes as a line of text in its own time zone without naming it, and not as it writes every other
// time. Read as UTC, a NAS in Los Angeles would have its certificates run out eight hours early.
func TestCertificates(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{}).Certificates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "certificates", got, []poll.Certificate{{Name: "truenas_default", Expires: time.Date(2027, time.March, 6, 16, 30, 0, 0, time.UTC)}})

	// a time that names its zone is where it says it is whatever the NAS's zone, and a zone this machine does not know is UTC
	for _, c := range []struct{ zone, until string }{
		{"America/Los_Angeles", `"2027-03-06T08:30:00Z"`},
		{"America/Los_Angeles", `"2027-03-06 00:30:00"`},
		{"Nowhere/Atlantis", `"Sat Mar 06 08:30:00 2027"`},
	} {
		got, err = of(&canned{instead: map[string]string{"certificate.query": `[{"name": "c", "until": ` + c.until + `}]`, "system.info": `{"timezone": "` + c.zone + `"}`}}).Certificates(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		same(t, c.until+" on a NAS in "+c.zone, got, []poll.Certificate{{Name: "c", Expires: time.Date(2027, time.March, 6, 8, 30, 0, 0, time.UTC)}})
	}

	// a date that cannot be read is no date, and the certificate is still listed
	got, err = of(&canned{instead: map[string]string{"certificate.query": `[{"name": "odd", "until": "next spring"}, {"name": "unparsed", "until": null}]`}}).Certificates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "certificates with no date to read", got, []poll.Certificate{{Name: "odd"}, {Name: "unparsed"}})
}

func TestInterfaces(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{}).Interfaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "interfaces", got, []poll.Interface{{Name: "enp1s0", Type: "PHYSICAL", LinkUp: true, MTU: 9000}})

	got, err = of(&canned{instead: map[string]string{"interface.query": `[{"name": "eno2", "type": "PHYSICAL", "state": {"link_state": "LINK_STATE_DOWN", "mtu": 1500}}]`}}).Interfaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "an interface with no link", got, []poll.Interface{{Name: "eno2", Type: "PHYSICAL", MTU: 1500}})
}

// Whether there is a newer TrueNAS. Not being able to find out is said apart: it is not the same as there being none.
func TestUpdate(t *testing.T) {
	t.Parallel()

	for answer, want := range map[string]poll.Update{
		"": {},
		`{"code": "NORMAL", "status": {"current_version": {"train": "TrueNAS-SCALE-Goldeye"}, "new_version": {"version": "25.10.8", "manifest": {}}}, "error": null}`: {Available: true},
		`{"code": "ERROR", "status": null, "error": {"errname": "ENETUNREACH", "reason": "Network is unreachable"}}`:                                                  {CheckFailed: true},
	} {
		c := &canned{}
		if answer != "" {
			c.instead = map[string]string{"update.status": answer}
		}

		got, err := nas.New(c, nas.Options{}).Update(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		same(t, "update for "+answer, got, want)
	}
}

// A NAS joined to no directory says so with nothing at all, which is not a fault.
func TestDirectory(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{}).Directory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "no directory", got, poll.Directory{})

	got, err = of(&canned{instead: map[string]string{"directoryservices.status": `{"type": "ACTIVEDIRECTORY", "status": "FAULTED", "status_msg": "winbind is not running"}`}}).Directory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "a directory at fault", got, poll.Directory{Type: "ACTIVEDIRECTORY", Status: "FAULTED"})
}

// The NAS lists every key of the user and not which one is in use, so the answer is the soonest a key that still works runs out: a revoked one is
// already gone and one with no end never goes.
func TestKey(t *testing.T) {
	t.Parallel()

	got, err := of(&canned{}).Key(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "key", got, poll.Key{Expires: at(1822536000000)})

	got, err = of(&canned{instead: map[string]string{"api_key.my_keys": `[{"expires_at": null, "revoked": false}]`}}).Key(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	same(t, "a key that never expires", got, poll.Key{})
}

// errRefused is the NAS saying no.
var errRefused = errors.New("not authorized")

// A question is one or several methods, and when one of them fails the question does, saying which: the page then keeps the last whole answer
// rather than half of a new one.
func TestAFailedMethodFailsItsQuestionAndIsNamed(t *testing.T) {
	t.Parallel()

	n := func(method string) *nas.NAS {
		return of(&canned{failing: map[string]error{method: errRefused}})
	}

	for method, question := range map[string]func(*nas.NAS) error{
		"system.info":               func(n *nas.NAS) error { _, err := n.System(t.Context()); return err },
		"system.state":              func(n *nas.NAS) error { _, err := n.System(t.Context()); return err },
		"pool.query":                func(n *nas.NAS) error { _, err := n.Pools(t.Context()); return err },
		"boot.get_state":            func(n *nas.NAS) error { _, err := n.BootPool(t.Context()); return err },
		"alert.list":                func(n *nas.NAS) error { _, err := n.Alerts(t.Context()); return err },
		"service.query":             func(n *nas.NAS) error { _, err := n.Services(t.Context()); return err },
		"core.get_jobs":             func(n *nas.NAS) error { _, err := n.Jobs(t.Context()); return err },
		"docker.status":             func(n *nas.NAS) error { _, err := n.Apps(t.Context()); return err },
		"app.query":                 func(n *nas.NAS) error { _, err := n.Apps(t.Context()); return err },
		"vm.query":                  func(n *nas.NAS) error { _, err := n.VMs(t.Context()); return err },
		"nfs.client_count":          func(n *nas.NAS) error { _, err := n.Clients(t.Context()); return err },
		"iscsi.global.client_count": func(n *nas.NAS) error { _, err := n.Clients(t.Context()); return err },
		"disk.query":                func(n *nas.NAS) error { _, err := n.Disks(t.Context()); return err },
		"disk.temperatures":         func(n *nas.NAS) error { _, err := n.Disks(t.Context()); return err },
		"pool.dataset.query":        func(n *nas.NAS) error { _, err := n.Datasets(t.Context()); return err },
		"pool.snapshottask.query":   func(n *nas.NAS) error { _, err := n.Tasks(t.Context()); return err },
		"replication.query":         func(n *nas.NAS) error { _, err := n.Tasks(t.Context()); return err },
		"cloudsync.query":           func(n *nas.NAS) error { _, err := n.Tasks(t.Context()); return err },
		"cloud_backup.query":        func(n *nas.NAS) error { _, err := n.Tasks(t.Context()); return err },
		"rsynctask.query":           func(n *nas.NAS) error { _, err := n.Tasks(t.Context()); return err },
		"pool.scrub.query":          func(n *nas.NAS) error { _, err := n.Tasks(t.Context()); return err },
		"sharing.smb.query":         func(n *nas.NAS) error { _, err := n.Shares(t.Context()); return err },
		"sharing.nfs.query":         func(n *nas.NAS) error { _, err := n.Shares(t.Context()); return err },
		"iscsi.target.query":        func(n *nas.NAS) error { _, err := n.Shares(t.Context()); return err },
		"certificate.query":         func(n *nas.NAS) error { _, err := n.Certificates(t.Context()); return err },
		"interface.query":           func(n *nas.NAS) error { _, err := n.Interfaces(t.Context()); return err },
		"update.status":             func(n *nas.NAS) error { _, err := n.Update(t.Context()); return err },
		"directoryservices.status":  func(n *nas.NAS) error { _, err := n.Directory(t.Context()); return err },
		"api_key.my_keys":           func(n *nas.NAS) error { _, err := n.Key(t.Context()); return err },
	} {
		err := question(n(method))
		if !errors.Is(err, errRefused) || !strings.HasPrefix(err.Error(), method+": ") {
			t.Errorf("with %s failing the question gave %v, want its failure with the method's name in front", method, err)
		}
	}
}

// reads is every method the exporter may call. It is the whole of what it does to a NAS, so a method that is not on this list fails the test
// until somebody has looked at what it does: nothing here may change anything.
var reads = []string{
	"alert.list", "api_key.my_keys", "app.query", "boot.get_state", "certificate.query", "cloud_backup.query", "cloudsync.query", "core.get_jobs",
	"directoryservices.status", "disk.query", "disk.temperatures", "docker.status", "interface.query", "iscsi.global.client_count", "iscsi.target.query",
	"nfs.client_count", "pool.dataset.query", "pool.query", "pool.scrub.query", "pool.snapshottask.query", "replication.query", "rsynctask.query",
	"service.query", "sharing.nfs.query", "sharing.smb.query", "system.info", "system.state", "update.status", "vm.query",
}

// Asked every question there is, the NAS hears only the methods on the list of reads, and every one of them.
func TestEveryQuestionOnlyReads(t *testing.T) {
	t.Parallel()

	c := &canned{}
	n := nas.New(c, nas.Options{})

	// the poller is what asks every question, so it is what is run: one round of each
	p, err := poll.New(n, poll.Intervals{Fast: time.Minute, Slow: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	for _, round := range poll.Rounds {
		p.Poll(t.Context(), round)
	}

	if s := p.State(); !s.FastRound.OK || !s.SlowRound.OK {
		t.Fatalf("a round against the saved answers failed: fast %+v, slow %+v", s.FastRound, s.SlowRound)
	}

	same(t, "the methods called", c.methods(), reads)

	// every query names the fields it reads, so a field the page has no use for cannot fail it
	c.mu.Lock()
	defer c.mu.Unlock()
	for method, params := range c.asked {
		query := slices.ContainsFunc(truenas.Operations, func(o truenas.OperationInfo) bool { return o.Method == method && o.Form == "count" })
		if query && len(fields(params)) == 0 {
			t.Errorf("%s does not name the fields it reads", method)
		}
	}
}

// Every method on the list of reads is one that changes nothing on the NAS, as truenas-mcp's table of methods has it from the NAS's own
// description. Where the two disagree, the table is looked at in truenas-mcp, not overruled here.
func TestEveryReadIsAReadToTheNAS(t *testing.T) {
	t.Parallel()

	for _, method := range reads {
		i := slices.IndexFunc(truenas.Operations, func(o truenas.OperationInfo) bool { return o.Method == method && o.Form == "" })
		switch {
		case i < 0:
			t.Errorf("%s is not a method the NAS describes", method)
		case !truenas.Operations[i].Read:
			t.Errorf("%s is not a read to the NAS's own description", method)
		}
	}
}

// errNoSuchMethod is the NAS having no method of the name called.
var errNoSuchMethod = errors.New("method does not exist")

// A NAS older or newer than this was written for may lack a method. Told how to recognise that, the question is answered with what the NAS does
// have and nothing for the method it lacks, so a missing method costs its own readings and not the round's; any other failure still fails.
func TestAMethodTheNASDoesNotHaveIsAnsweredWithNothing(t *testing.T) {
	t.Parallel()

	c := &canned{failing: map[string]error{"vm.query": errNoSuchMethod, "cloud_backup.query": errNoSuchMethod, "update.status": errNoSuchMethod, "docker.status": errRefused}}
	n := nas.New(c, nas.Options{NoSuchMethod: func(err error) bool { return errors.Is(err, errNoSuchMethod) }})

	vms, err := n.VMs(t.Context())
	if err != nil || len(vms) != 0 {
		t.Errorf("virtual machines on a NAS without the method = %+v, %v; want none and no failure", vms, err)
	}

	tasks, err := n.Tasks(t.Context())
	if err != nil || len(tasks) != 15 {
		t.Errorf("tasks on a NAS without cloud backup = %d, %v; want the 15 of the other kinds and no failure", len(tasks), err)
	}

	if update, err := n.Update(t.Context()); err != nil || update != (poll.Update{}) {
		t.Errorf("update on a NAS without the method = %+v, %v", update, err)
	}

	if _, err := n.Apps(t.Context()); !errors.Is(err, errRefused) {
		t.Errorf("a method refused for another reason gave %v, want its failure", err)
	}

	// not told how to recognise it, a missing method is a failure like any other
	if _, err := nas.New(c, nas.Options{}).VMs(t.Context()); !errors.Is(err, errNoSuchMethod) {
		t.Errorf("a missing method nobody said how to recognise gave %v, want its failure", err)
	}
}
