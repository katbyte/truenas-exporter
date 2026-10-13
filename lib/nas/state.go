package nas

import (
	"context"

	"github.com/katbyte/truenas-metrics/lib/poll"
)

// Alerts asks the NAS for the alerts it is showing. What each says is not kept, only how serious it is and of what kind.
func (n *NAS) Alerts(ctx context.Context) ([]poll.Alert, error) {
	alerts, err := ask[[]struct {
		Level     string `json:"level"`
		Class     string `json:"klass"`
		Dismissed bool   `json:"dismissed"`
	}](ctx, n, "alert.list")
	if err != nil {
		return nil, err
	}

	out := make([]poll.Alert, 0, len(alerts))
	for _, a := range alerts {
		out = append(out, poll.Alert{Level: a.Level, Class: a.Class, Dismissed: a.Dismissed})
	}

	return out, nil
}

// running is what the NAS calls a service, a job or a task that is under way.
const running = "RUNNING"

// Services asks the NAS which of its services are running and which start with it.
func (n *NAS) Services(ctx context.Context) ([]poll.Service, error) {
	services, err := ask[[]struct {
		Name    string `json:"service"`
		State   string `json:"state"`
		Enabled bool   `json:"enable"`
	}](ctx, n, "service.query")
	if err != nil {
		return nil, err
	}

	out := make([]poll.Service, 0, len(services))
	for _, s := range services {
		out = append(out, poll.Service{Name: s.Name, Running: s.State == running, Enabled: s.Enabled})
	}

	return out, nil
}

// Jobs asks the NAS for the jobs that are running or waiting to. The ones that are over are not asked for: the NAS keeps hundreds.
func (n *NAS) Jobs(ctx context.Context) ([]poll.Job, error) {
	jobs, err := ask[[]struct {
		Method string `json:"method"`
		State  string `json:"state"`
	}](ctx, n, "core.get_jobs", []any{[]any{"state", "in", poll.JobStates}})
	if err != nil {
		return nil, err
	}

	out := make([]poll.Job, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, poll.Job{Method: j.Method, State: j.State})
	}

	return out, nil
}

// Apps asks the NAS for the state of the service that runs apps, and for the apps installed.
func (n *NAS) Apps(ctx context.Context) (poll.Apps, error) {
	service, err := ask[struct {
		Status string `json:"status"`
	}](ctx, n, "docker.status")
	if err != nil {
		return poll.Apps{}, err
	}

	apps, err := ask[[]struct {
		Name            string `json:"name"`
		State           string `json:"state"`
		UpdateAvailable bool   `json:"upgrade_available"`
	}](ctx, n, "app.query")
	if err != nil {
		return poll.Apps{}, err
	}

	out := poll.Apps{Status: service.Status, List: make([]poll.App, 0, len(apps))}
	for _, a := range apps {
		out.List = append(out.List, poll.App{Name: a.Name, State: a.State, UpdateAvailable: a.UpdateAvailable})
	}

	return out, nil
}

// VMs asks the NAS for its virtual machines and whether each is running.
func (n *NAS) VMs(ctx context.Context) ([]poll.VM, error) {
	vms, err := ask[[]struct {
		Name   string `json:"name"`
		Status struct {
			State string `json:"state"`
		} `json:"status"`
	}](ctx, n, "vm.query")
	if err != nil {
		return nil, err
	}

	out := make([]poll.VM, 0, len(vms))
	for _, vm := range vms {
		out = append(out, poll.VM{Name: vm.Name, State: vm.Status.State})
	}

	return out, nil
}

// Clients asks the NAS how many clients are connected over NFS and over iSCSI. It has no such count for SMB.
func (n *NAS) Clients(ctx context.Context) ([]poll.Clients, error) {
	nfs, err := ask[int](ctx, n, "nfs.client_count")
	if err != nil {
		return nil, err
	}

	iscsi, err := ask[int](ctx, n, "iscsi.global.client_count")
	if err != nil {
		return nil, err
	}

	return []poll.Clients{{Protocol: "nfs", Count: nfs}, {Protocol: "iscsi", Count: iscsi}}, nil
}
