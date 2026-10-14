package nas

import (
	"context"
	"time"

	"github.com/katbyte/go-kt/pointer"
	"github.com/katbyte/truenas-mcp/sdk/truenas"
	"github.com/katbyte/truenas-metrics/lib/poll"
)

// Pools asks the NAS for its pools, the boot pool not among them.
func (n *NAS) Pools(ctx context.Context) ([]poll.Pool, error) {
	pools, err := n.api.Pool.Query(ctx, nil, truenas.PoolQueryOptions{})
	if err != nil {
		return nil, err
	}

	out := make([]poll.Pool, 0, len(pools.Answer))
	for i := range pools.Answer {
		out = append(out, pool(&pools.Answer[i]))
	}

	return out, nil
}

// BootPool asks the NAS for the pool it runs from, which it answers with a type of its own holding the same fields as any other pool's.
func (n *NAS) BootPool(ctx context.Context) (poll.Pool, error) {
	boot, err := n.api.Boot.GetState(ctx)
	if err != nil {
		return poll.Pool{}, err
	}

	return pool(&truenas.PoolQueryResultItem{Name: boot.Name, Status: boot.Status, Healthy: boot.Healthy, Warning: boot.Warning, Size: boot.Size, Allocated: boot.Allocated, Free: boot.Free, Freeing: boot.Freeing, Fragmentation: boot.Fragmentation, Scan: boot.Scan, Topology: boot.Topology}), nil
}

// pool is a pool in the poller's words. A pool that is not imported has little but its name and status: the rest is nothing at all.
func pool(p *truenas.PoolQueryResultItem) poll.Pool {
	out := poll.Pool{
		Name:      p.Name,
		Status:    p.Status,
		Healthy:   p.Healthy,
		Warning:   p.Warning,
		Size:      int64(p.Size),
		Allocated: int64(p.Allocated),
		Free:      int64(p.Free),
		Freeing:   int64(p.Freeing),
		Scan:      scan(p.Scan),
	}

	if fragmentation, ok := number(p.Fragmentation); ok {
		out.Fragmentation = new(ratio(fragmentation))
	}

	if t := p.Topology; t != nil {
		for _, part := range []struct {
			class string
			vdevs []truenas.PoolVdev
		}{{"data", t.Data}, {"log", t.Log}, {"cache", t.Cache}, {"spare", t.Spare}, {"special", t.Special}, {"dedup", t.Dedup}} {
			for i := range part.vdevs {
				out.Vdevs = append(out.Vdevs, vdev(&part.vdevs[i], part.class))
			}
		}
	}

	return out
}

// scan is a pool's last scrub or resilver in the poller's words, and nil for a pool that has never had one: the NAS gives that pool a scan with
// nothing in it.
func scan(s *truenas.PoolScan) *poll.Scan {
	if s == nil || s.Function == "" || s.State == "" {
		return nil
	}

	out := &poll.Scan{
		Function:  s.Function,
		State:     s.State,
		Started:   s.StartTime.Time,
		Ended:     s.EndTime.Time,
		Progress:  ratio(s.Percentage),
		ToExamine: int64(s.BytesToProcess),
		Examined:  int64(s.BytesProcessed),
		Issued:    int64(s.BytesIssued),
		Errors:    int64(s.Errors),
		Paused:    s.Pause != nil,
	}

	if s.TotalSecsLeft != nil {
		out.Left = new(time.Duration(*s.TotalSecsLeft) * time.Second)
	}

	return out
}

// vdev is a top-level vdev in the poller's words, with every disk under it however deep: a disk being replaced sits a level further down than the
// rest, beside the one replacing it.
func vdev(v *truenas.PoolVdev, class string) poll.Vdev {
	// zpool lists a vdev of one disk under a long number where the NAS gives one; the disk's own name says more
	name := v.Name
	if len(v.Children) == 0 && v.Disk != "" {
		name = v.Disk
	}

	stats := pointer.From(v.Stats)
	out := poll.Vdev{
		Class:         class,
		Name:          name,
		Type:          v.Type,
		Status:        v.Status,
		Size:          int64(stats.Size),
		Allocated:     int64(stats.Allocated),
		Fragmentation: ratio(float64(stats.Fragmentation)),
		Errors:        errorsOf(stats),
	}
	disks(v, &out.Devices)

	return out
}

// disks adds the disks at and under v. A node with nothing under it is a disk, whatever it is called.
func disks(v *truenas.PoolVdev, into *[]poll.Device) {
	if len(v.Children) > 0 {
		for i := range v.Children {
			disks(&v.Children[i], into)
		}

		return
	}

	// a disk the NAS can no longer find has no name; the pool still knows it by its number, which tells two missing disks apart
	disk := v.Disk
	if disk == "" {
		disk = v.Guid
	}

	*into = append(*into, poll.Device{Disk: disk, Status: v.Status, Errors: errorsOf(pointer.From(v.Stats))})
}

// errorsOf is the errors a vdev or a disk has counted.
func errorsOf(s truenas.PoolVdevStats) poll.Errors {
	return poll.Errors{Read: int64(s.ReadErrors), Write: int64(s.WriteErrors), Checksum: int64(s.ChecksumErrors)}
}
