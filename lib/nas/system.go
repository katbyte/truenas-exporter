package nas

import (
	"context"
	"time"

	"github.com/katbyte/go-kt/pointer"
	"github.com/katbyte/truenas-metrics/lib/poll"
)

// systemInfo is what system.info answers, less what the page has no use for.
type systemInfo struct {
	Version       string    `json:"version"`
	Product       string    `json:"system_product"`
	Manufacturer  string    `json:"system_manufacturer"`
	Model         string    `json:"model"`
	Memory        int64     `json:"physmem"`
	Cores         int       `json:"cores"`
	PhysicalCores int       `json:"physical_cores"`
	Load          []float64 `json:"loadavg"`
	Uptime        float64   `json:"uptime_seconds"`
	Booted        moment    `json:"boottime"`
	Clock         moment    `json:"datetime"`
	ECC           bool      `json:"ecc_memory"`
}

// System asks the NAS about itself, and whether it has finished starting.
func (n *NAS) System(ctx context.Context) (poll.System, error) {
	asked := time.Now()
	info, err := ask[systemInfo](ctx, n, "system.info")
	if err != nil {
		return poll.System{}, err
	}
	answered := time.Now()

	state, err := ask[string](ctx, n, "system.state")
	if err != nil {
		return poll.System{}, err
	}

	s := poll.System{
		Version:       info.Version,
		Product:       info.Product,
		Manufacturer:  info.Manufacturer,
		CPU:           info.Model,
		State:         state,
		Booted:        info.Booted.Time,
		Uptime:        time.Duration(info.Uptime * float64(time.Second)),
		Memory:        info.Memory,
		Cores:         info.Cores,
		PhysicalCores: info.PhysicalCores,
		ECC:           info.ECC,
	}
	copy(s.Load[:], info.Load)

	// the NAS read its clock somewhere between the question leaving and the answer arriving: halfway is the best guess there is
	if !info.Clock.IsZero() {
		s.ClockOffset = info.Clock.Sub(asked.Add(answered.Sub(asked) / 2))
	}

	return s, nil
}

// updateStatus is what update.status answers, less what the page has no use for.
type updateStatus struct {
	Code   string `json:"code"`
	Status *struct {
		NewVersion *struct{} `json:"new_version"`
	} `json:"status"`
}

// Update asks the NAS whether it knows of a newer version of itself.
func (n *NAS) Update(ctx context.Context) (poll.Update, error) {
	status, err := ask[updateStatus](ctx, n, "update.status")
	if err != nil {
		return poll.Update{}, err
	}

	return poll.Update{
		Available:   status.Status != nil && status.Status.NewVersion != nil,
		CheckFailed: status.Code == "ERROR",
	}, nil
}

// Directory asks the NAS which directory service it is joined to, if any, and how that is going.
func (n *NAS) Directory(ctx context.Context) (poll.Directory, error) {
	status, err := ask[struct {
		Type   *string `json:"type"`
		Status *string `json:"status"`
	}](ctx, n, "directoryservices.status")
	if err != nil {
		return poll.Directory{}, err
	}

	return poll.Directory{Type: pointer.From(status.Type), Status: pointer.From(status.Status)}, nil
}

// Key asks the NAS when the key the questions are asked with runs out. The NAS lists every key of the user logged in and does not say which one
// this is, so the answer is the soonest any of them that still works runs out.
func (n *NAS) Key(ctx context.Context) (poll.Key, error) {
	keys, err := ask[[]struct {
		Expires moment `json:"expires_at"`
		Revoked bool   `json:"revoked"`
	}](ctx, n, "api_key.my_keys")
	if err != nil {
		return poll.Key{}, err
	}

	var k poll.Key
	for _, key := range keys {
		if key.Revoked || key.Expires.IsZero() {
			continue
		}

		if k.Expires.IsZero() || key.Expires.Before(k.Expires) {
			k.Expires = key.Expires.Time
		}
	}

	return k, nil
}
