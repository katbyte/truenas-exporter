package nas

import (
	"context"

	"github.com/katbyte/go-kt/pointer"
	"github.com/katbyte/truenas-mcp/sdk/truenas"
	"github.com/katbyte/truenas-metrics/lib/poll"
)

// Alerts asks the NAS for the alerts it is showing. What each says is not kept, only how serious it is and of what kind.
func (n *NAS) Alerts(ctx context.Context) ([]poll.Alert, error) {
	alerts, err := n.api.Alert.List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]poll.Alert, 0, len(alerts.Answer))
	for _, a := range alerts.Answer {
		out = append(out, poll.Alert{Level: a.Level, Class: a.Klass, Dismissed: a.Dismissed})
	}

	return out, nil
}

// running is what the NAS calls a service, a job or a task that is under way.
const running = "RUNNING"

// Services asks the NAS which of its services are running and which start with it.
func (n *NAS) Services(ctx context.Context) ([]poll.Service, error) {
	services, err := n.api.Service.Query(ctx, nil, truenas.ServiceQueryOptions{Select: []any{"service", "state", "enable"}})
	if err != nil {
		return nil, err
	}

	out := make([]poll.Service, 0, len(services.Answer))
	for _, s := range services.Answer {
		out = append(out, poll.Service{Name: s.Service, Running: s.State == running, Enabled: s.Enable})
	}

	return out, nil
}

// Jobs asks the NAS for the jobs that are running or waiting to. The ones that are over are not asked for: the NAS keeps hundreds.
func (n *NAS) Jobs(ctx context.Context) ([]poll.Job, error) {
	jobs, err := n.api.Core.GetJobs(ctx, []any{[]any{"state", "in", poll.JobStates}}, truenas.CoreGetJobsOptions{Select: []any{"method", "state"}})
	if err != nil {
		return nil, err
	}

	out := make([]poll.Job, 0, len(jobs.Answer))
	for _, j := range jobs.Answer {
		out = append(out, poll.Job{Method: j.Method, State: j.State})
	}

	return out, nil
}

// Apps asks the NAS for the state of the service that runs apps, and for the apps installed.
func (n *NAS) Apps(ctx context.Context) (poll.Apps, error) {
	service, err := n.api.Docker.Status(ctx)
	if err != nil {
		return poll.Apps{}, err
	}

	apps, err := n.api.App.Query(ctx, nil, truenas.AppQueryOptions{Select: []any{"name", "state", "upgrade_available"}})
	if err != nil {
		return poll.Apps{}, err
	}

	out := poll.Apps{Status: string(service.Status), List: make([]poll.App, 0, len(apps.Answer))}
	for _, a := range apps.Answer {
		out.List = append(out.List, poll.App{Name: a.Name, State: string(a.State), UpdateAvailable: a.UpgradeAvailable})
	}

	return out, nil
}

// VMs asks the NAS for its virtual machines and whether each is running.
func (n *NAS) VMs(ctx context.Context) ([]poll.VM, error) {
	vms, err := n.api.Vm.Query(ctx, nil, truenas.VmQueryOptions{Select: []any{"name", "status.state"}})
	if err != nil {
		return nil, err
	}

	out := make([]poll.VM, 0, len(vms.Answer))
	for _, vm := range vms.Answer {
		out = append(out, poll.VM{Name: vm.Name, State: pointer.From(vm.Status).State})
	}

	return out, nil
}

// Clients asks the NAS how many clients are connected over NFS and over iSCSI. It has no such count for SMB.
func (n *NAS) Clients(ctx context.Context) ([]poll.Clients, error) {
	nfs, err := n.api.Nfs.ClientCount(ctx)
	if err != nil {
		return nil, err
	}

	iscsi, err := n.api.IscsiGlobal.ClientCount(ctx)
	if err != nil {
		return nil, err
	}

	return []poll.Clients{{Protocol: "nfs", Count: pointer.From(nfs.Answer)}, {Protocol: "iscsi", Count: pointer.From(iscsi.Answer)}}, nil
}
