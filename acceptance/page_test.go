//go:build acceptance

package acceptance_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/go-kt/pmetrics"
	"github.com/katbyte/truenas-mcp/sdk/truenasrpc"
	"github.com/prometheus/client_golang/prometheus/testutil/promlint"
)

// The first thing to know of a real NAS: asked every question once, it answers every one, as the user the program is meant to run as. A method this
// TrueNAS does not have, a question that user may not ask, or an answer that cannot be read all show up here.
func TestPrintAnswersEveryQuestion(t *testing.T) {
	live(t)
	printAnswersEveryQuestion(t)
}

func printAnswersEveryQuestion(t *testing.T) {
	t.Helper()

	page, err := printed(t)
	if strings.Contains(page, key) {
		t.Fatal("the page holds the key")
	}
	if err != nil {
		t.Errorf("print: %v; what went wrong is in the warnings above", err)
	}

	type named struct {
		name string
		with map[string]string
	}

	for _, r := range []named{{"truenas_up", nil}, {"truenas_system_state", map[string]string{"state": "READY"}}} {
		if got, ok := one(page, r.name, r.with); !ok || got != 1 {
			t.Errorf("%s%v = %v (one reading: %t), want 1", r.name, r.with, got, ok)
		}
	}

	// whether a scrub of the boot pool is running, or the update check failed, is the NAS's business: only that the reading is there
	for _, r := range []named{{"truenas_pool_scan_running", map[string]string{"pool": "boot-pool", "function": "SCRUB"}}, {"truenas_update_check_failed", nil}} {
		if _, ok := one(page, r.name, r.with); !ok {
			t.Errorf("the page has no one reading of %s%v", r.name, r.with)
		}
	}

	for _, round := range []string{"fast", "slow"} {
		if got, ok := one(page, "truenas_poll_failures_total", map[string]string{"poll": round}); !ok || got != 0 {
			t.Errorf("the %s round failed %v times (one reading: %t), want it answered in full the first time", round, got, ok)
		}
	}

	if info := readings(page, "truenas_system_info"); len(info) != 1 || info[0].labels["version"] == "" {
		t.Errorf("truenas_system_info = %+v, want the one reading with the NAS's version", info)
	}

	// what is written whether or not there is anything to count
	for name, want := range map[string]int{"truenas_alerts": 7, "truenas_jobs": 2, "truenas_shares": 9, "truenas_tasks": 6, "truenas_system_state": 3} {
		if got := len(readings(page, name)); got < want {
			t.Errorf("%d readings of %s, want the %d that are always written", got, name, want)
		}
	}

	problems, err := promlint.New(strings.NewReader(page)).Lint()
	if err != nil {
		t.Fatalf("checking the page: %v", err)
	}
	for _, p := range problems {
		t.Errorf("%s: %s", p.Metric, p.Text)
	}

	if t.Failed() {
		t.Logf("the page:\n%s", page)
	}
}

// said is what the NAS says when asked directly, for the things the page is checked against. It is asked for before the page is made and again
// after, and the two compared, since a NAS in use changes under the suite: an alert clears, a service is stopped.
type said struct {
	Pools []struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Healthy bool   `json:"healthy"`
		Size    *int64 `json:"size"`
	}
	Boot struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	Disks []struct {
		Name   string `json:"name"`
		Serial string `json:"serial"`
		Size   *int64 `json:"size"`
	}
	Datasets []struct {
		Name string `json:"name"`
		Pool string `json:"pool"`
	}
	Services []struct {
		Name    string `json:"service"`
		State   string `json:"state"`
		Enabled bool   `json:"enable"`
	}
	Alerts []struct {
		Dismissed bool `json:"dismissed"`
	}
	Tasks  map[string]int
	Shares map[string]int
}

// ask is what the NAS says now.
func ask(t *testing.T, conn *truenasrpc.Conn) said {
	t.Helper()

	var s said
	s.Pools = asked[[]struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Healthy bool   `json:"healthy"`
		Size    *int64 `json:"size"`
	}](t, conn, "pool.query")
	s.Boot = asked[struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}](t, conn, "boot.get_state")
	s.Disks = asked[[]struct {
		Name   string `json:"name"`
		Serial string `json:"serial"`
		Size   *int64 `json:"size"`
	}](t, conn, "disk.query")
	s.Datasets = asked[[]struct {
		Name string `json:"name"`
		Pool string `json:"pool"`
	}](t, conn, "pool.dataset.query", []any{}, map[string]any{"extra": map[string]any{"flat": true, "retrieve_children": true, "properties": []string{}}})
	s.Services = asked[[]struct {
		Name    string `json:"service"`
		State   string `json:"state"`
		Enabled bool   `json:"enable"`
	}](t, conn, "service.query")
	s.Alerts = asked[[]struct {
		Dismissed bool `json:"dismissed"`
	}](t, conn, "alert.list")

	s.Tasks, s.Shares = map[string]int{}, map[string]int{}
	for kind, method := range map[string]string{"snapshot": "pool.snapshottask.query", "replication": "replication.query", "cloud_sync": "cloudsync.query", "cloud_backup": "cloud_backup.query", "rsync": "rsynctask.query", "scrub": "pool.scrub.query"} {
		s.Tasks[kind] = len(asked[[]json.RawMessage](t, conn, method))
	}
	for protocol, method := range map[string]string{"smb": "sharing.smb.query", "nfs": "sharing.nfs.query", "iscsi": "iscsi.target.query"} {
		s.Shares[protocol] = len(asked[[]json.RawMessage](t, conn, method))
	}

	return s
}

// The page is compared with the NAS's own answers: every pool with the status the NAS gives it, every disk by name and serial number, every
// dataset, every service, and as many alerts, tasks and shares as the NAS lists. Nothing is known of the NAS beforehand, so this holds for any.
func TestThePageSaysWhatTheNASSays(t *testing.T) {
	live(t)
	pageSaysWhatTheNASSays(t)
}

func pageSaysWhatTheNASSays(t *testing.T) {
	t.Helper()

	conn := asking(t)

	// the NAS is asked before and after the page is made, and the page is only held to an answer that did not change in between
	var nas said
	var page string
	for try := range 3 {
		before := ask(t, conn)

		var err error
		if page, err = printed(t); err != nil {
			t.Fatalf("print: %v", err)
		}

		nas = ask(t, conn)
		was, _ := json.Marshal(before)
		is, _ := json.Marshal(nas)
		if bytes.Equal(was, is) {
			break
		}
		if try == 2 {
			t.Skip("the NAS changed under the suite three times running, so there is nothing steady to hold the page to")
		}
	}

	if got, want := len(readings(page, "truenas_pool_healthy")), len(nas.Pools)+1; got != want {
		t.Errorf("%d pools on the page, want the NAS's %d and its boot pool", got, len(nas.Pools))
	}
	for _, pool := range nas.Pools {
		if got, ok := one(page, "truenas_pool_status", map[string]string{"pool": pool.Name, "status": pool.Status}); !ok || got != 1 {
			t.Errorf("pool %s is not on the page as %s", pool.Name, pool.Status)
		}

		healthy := 0.0
		if pool.Healthy {
			healthy = 1
		}
		if got, ok := one(page, "truenas_pool_healthy", map[string]string{"pool": pool.Name}); !ok || got != healthy {
			t.Errorf("truenas_pool_healthy of %s = %v, want %v", pool.Name, got, healthy)
		}

		if pool.Size != nil {
			if got, ok := one(page, "truenas_pool_size_bytes", map[string]string{"pool": pool.Name}); !ok || got != float64(*pool.Size) {
				t.Errorf("truenas_pool_size_bytes of %s = %v, want %d", pool.Name, got, *pool.Size)
			}
		}
	}
	if got, ok := one(page, "truenas_pool_status", map[string]string{"pool": nas.Boot.Name, "status": nas.Boot.Status}); !ok || got != 1 {
		t.Errorf("the boot pool %s is not on the page as %s", nas.Boot.Name, nas.Boot.Status)
	}

	if got := len(readings(page, "truenas_disk_info")); got != len(nas.Disks) {
		t.Errorf("%d disks on the page, want the NAS's %d", got, len(nas.Disks))
	}
	for _, disk := range nas.Disks {
		if _, ok := one(page, "truenas_disk_info", map[string]string{"disk": disk.Name, "serial": disk.Serial}); !ok {
			t.Errorf("disk %s is not on the page with its serial number", disk.Name)
		}

		if disk.Size != nil {
			if got, ok := one(page, "truenas_disk_size_bytes", map[string]string{"disk": disk.Name, "serial": disk.Serial}); !ok || got != float64(*disk.Size) {
				t.Errorf("truenas_disk_size_bytes of %s = %v, want %d", disk.Name, got, *disk.Size)
			}
		}
	}

	// every disk a pool has is one the NAS lists, so each has a serial number to be joined on. A disk the pool has lost is not listed any more,
	// and has none
	for _, device := range readings(page, "truenas_pool_device_status") {
		if device.labels["status"] == "ONLINE" && device.labels["serial"] == "" {
			t.Errorf("the disk %s of pool %s has no serial number on the page", device.labels["disk"], device.labels["pool"])
		}
	}

	perPool := map[string]int{}
	for _, dataset := range nas.Datasets {
		perPool[dataset.Pool]++
		if _, ok := one(page, "truenas_dataset_used_bytes", map[string]string{"dataset": dataset.Name}); !ok {
			t.Errorf("dataset %s is not on the page", dataset.Name)
		}
	}
	for pool, want := range perPool {
		if got, ok := one(page, "truenas_datasets", map[string]string{"pool": pool}); !ok || int(got) != want {
			t.Errorf("truenas_datasets of %s = %v, want %d", pool, got, want)
		}
	}

	for _, service := range nas.Services {
		running, enabled := 0.0, 0.0
		if service.State == "RUNNING" {
			running = 1
		}
		if service.Enabled {
			enabled = 1
		}

		if got, ok := one(page, "truenas_service_running", map[string]string{"service": service.Name}); !ok || got != running {
			t.Errorf("truenas_service_running of %s = %v, want %v", service.Name, got, running)
		}
		if got, ok := one(page, "truenas_service_enabled", map[string]string{"service": service.Name}); !ok || got != enabled {
			t.Errorf("truenas_service_enabled of %s = %v, want %v", service.Name, got, enabled)
		}
	}

	var showing, dismissed float64
	for _, alert := range nas.Alerts {
		if alert.Dismissed {
			dismissed++
		} else {
			showing++
		}
	}
	var onPage float64
	for _, r := range readings(page, "truenas_alerts") {
		onPage += r.value
	}
	if got, _ := one(page, "truenas_alerts_dismissed", nil); onPage != showing || got != dismissed {
		t.Errorf("%v alerts showing and %v dismissed on the page, want the NAS's %v and %v", onPage, got, showing, dismissed)
	}

	for kind, want := range nas.Tasks {
		if got, ok := one(page, "truenas_tasks", map[string]string{"kind": kind}); !ok || int(got) != want {
			t.Errorf("truenas_tasks of kind %s = %v, want %d", kind, got, want)
		}
	}
	for protocol, want := range nas.Shares {
		var got float64
		for _, r := range readings(page, "truenas_shares") {
			if r.labels["protocol"] == protocol {
				got += r.value
			}
		}
		if int(got) != want {
			t.Errorf("%v shares over %s on the page, want the NAS's %d", got, protocol, want)
		}
	}

	if t.Failed() {
		t.Logf("the page:\n%s", page)
	}
}

// serve against a real NAS: the page is up within moments, says the NAS is, and is refreshed on the schedule it was given without being asked.
func TestServeKeepsThePageFresh(t *testing.T) {
	live(t)
	serveKeepsThePageFresh(t)
}

func serveKeepsThePageFresh(t *testing.T) {
	t.Helper()

	at := serving(t, 2*time.Second, time.Minute)

	if got := strings.TrimSpace(get(t, at+pmetrics.HealthPath)); got != "ok" {
		t.Errorf("%s = %q, want ok", pmetrics.HealthPath, got)
	}

	answered := func(page, round string) (float64, bool) {
		return one(page, "truenas_poll_last_success_timestamp_seconds", map[string]string{"poll": round})
	}

	page := until(t, at, func(page string) bool {
		up, _ := one(page, "truenas_up", nil)
		_, fast := answered(page, "fast")
		_, slow := answered(page, "slow")

		return up == 1 && fast && slow
	})
	if strings.Contains(page, key) {
		t.Fatal("the page holds the key")
	}

	first, ok := answered(page, "fast")
	if up, _ := one(page, "truenas_up", nil); up != 1 || !ok {
		t.Fatalf("the page never said the NAS was up with both rounds answered:\n%s", page)
	}

	page = until(t, at, func(page string) bool {
		again, _ := answered(page, "fast")
		return again > first
	})
	if again, _ := answered(page, "fast"); again <= first {
		t.Errorf("the fast round was not asked again within %s of being answered", patience)
	}
	if failures, _ := one(page, "truenas_poll_failures_total", map[string]string{"poll": "fast"}); failures != 0 {
		t.Errorf("the fast round failed %v times while serving", failures)
	}
}
