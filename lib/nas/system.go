package nas

import (
	"context"
	"time"

	"github.com/katbyte/go-kt/pointer"
	"github.com/katbyte/truenas-mcp/sdk/truenas"
	"github.com/katbyte/truenas-metrics/lib/poll"
)

// System asks the NAS about itself, and whether it has finished starting.
func (n *NAS) System(ctx context.Context) (poll.System, error) {
	asked := time.Now()
	info, err := n.api.System.Info(ctx)
	if err != nil {
		return poll.System{}, err
	}
	answered := time.Now()

	state, err := n.api.System.State(ctx)
	if err != nil {
		return poll.System{}, err
	}

	s := poll.System{
		Version:       info.Version,
		Product:       info.SystemProduct,
		Manufacturer:  info.SystemManufacturer,
		CPU:           info.Model,
		State:         pointer.From(state.Answer),
		Booted:        info.Boottime.Time,
		Uptime:        time.Duration(info.UptimeSeconds * float64(time.Second)),
		Memory:        int64(info.Physmem),
		Cores:         info.Cores,
		PhysicalCores: info.PhysicalCores,
		ECC:           info.EccMemory,
	}
	for i := range min(len(info.Loadavg), len(s.Load)) {
		s.Load[i], _ = number(info.Loadavg[i])
	}

	// the NAS read its clock somewhere between the question leaving and the answer arriving: halfway is the best guess there is
	if !info.Datetime.IsZero() {
		s.ClockOffset = info.Datetime.Add(n.o.ClockCorrection).Sub(asked.Add(answered.Sub(asked) / 2))
	}

	return s, nil
}

// Update asks the NAS whether it knows of a newer version of itself.
func (n *NAS) Update(ctx context.Context) (poll.Update, error) {
	status, err := n.api.Update.Status(ctx)
	if err != nil {
		return poll.Update{}, err
	}

	return poll.Update{
		Available:   status.Status != nil && status.Status.NewVersion != nil,
		CheckFailed: status.Code == truenas.UpdateStatusAnswerCodeERROR,
	}, nil
}

// Directory asks the NAS which directory service it is joined to, if any, and how that is going.
func (n *NAS) Directory(ctx context.Context) (poll.Directory, error) {
	status, err := n.api.Directoryservices.Status(ctx)
	if err != nil {
		return poll.Directory{}, err
	}

	return poll.Directory{Type: string(status.Type), Status: string(status.Status)}, nil
}

// Key asks the NAS when the key the questions are asked with runs out. The NAS lists every key of the user logged in and does not say which one
// this is, so the answer is the soonest any of them that still works runs out.
func (n *NAS) Key(ctx context.Context) (poll.Key, error) {
	keys, err := n.api.ApiKey.MyKeys(ctx)
	if err != nil {
		return poll.Key{}, err
	}

	var k poll.Key
	for _, key := range keys.Answer {
		if key.Revoked || key.ExpiresAt.IsZero() {
			continue
		}

		if k.Expires.IsZero() || key.ExpiresAt.Before(k.Expires) {
			k.Expires = key.ExpiresAt.Time
		}
	}

	return k, nil
}
