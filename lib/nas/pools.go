package nas

import (
	"context"
	"time"

	"github.com/katbyte/go-kt/pointer"
	"github.com/katbyte/truenas-metrics/lib/poll"
)

// pool is one pool as pool.query and boot.get_state give it, less what the page has no use for. A pool that is not imported has little but its
// name and status: the rest is nothing at all.
type pool struct {
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	Healthy       bool      `json:"healthy"`
	Warning       bool      `json:"warning"`
	Size          *int64    `json:"size"`
	Allocated     *int64    `json:"allocated"`
	Free          *int64    `json:"free"`
	Freeing       *int64    `json:"freeing"`
	Fragmentation number    `json:"fragmentation"`
	Scan          *scan     `json:"scan"`
	Topology      *topology `json:"topology"`
}

// scan is a pool's last scrub or resilver. A pool never scanned has one with nothing in it.
type scan struct {
	Function  *string  `json:"function"`
	State     *string  `json:"state"`
	Started   moment   `json:"start_time"`
	Ended     moment   `json:"end_time"`
	Percent   *float64 `json:"percentage"`
	ToExamine *int64   `json:"bytes_to_process"`
	Examined  *int64   `json:"bytes_processed"`
	Issued    *int64   `json:"bytes_issued"`
	Paused    moment   `json:"pause"`
	Errors    *int64   `json:"errors"`
	Left      *float64 `json:"total_secs_left"`
}

// topology is a pool's vdevs, sorted into the parts of the pool they belong to.
type topology struct {
	Data    []vdev `json:"data"`
	Log     []vdev `json:"log"`
	Cache   []vdev `json:"cache"`
	Spare   []vdev `json:"spare"`
	Special []vdev `json:"special"`
	Dedup   []vdev `json:"dedup"`
}

// vdev is one node of a pool's tree: a raidz or a mirror with its disks under it, or a disk.
type vdev struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	GUID     string  `json:"guid"`
	Status   string  `json:"status"`
	Disk     *string `json:"disk"`
	Children []vdev  `json:"children"`
	Stats    struct {
		Read          int64   `json:"read_errors"`
		Write         int64   `json:"write_errors"`
		Checksum      int64   `json:"checksum_errors"`
		Size          int64   `json:"size"`
		Allocated     int64   `json:"allocated"`
		Fragmentation float64 `json:"fragmentation"`
	} `json:"stats"`
}

// Pools asks the NAS for its pools, the boot pool not among them.
func (n *NAS) Pools(ctx context.Context) ([]poll.Pool, error) {
	pools, err := ask[[]pool](ctx, n, "pool.query")
	if err != nil {
		return nil, err
	}

	out := make([]poll.Pool, 0, len(pools))
	for i := range pools {
		out = append(out, pools[i].poll())
	}

	return out, nil
}

// BootPool asks the NAS for the pool it runs from.
func (n *NAS) BootPool(ctx context.Context) (poll.Pool, error) {
	boot, err := ask[pool](ctx, n, "boot.get_state")
	if err != nil {
		return poll.Pool{}, err
	}

	return boot.poll(), nil
}

// poll is the pool in the poller's words.
func (p *pool) poll() poll.Pool {
	out := poll.Pool{
		Name:      p.Name,
		Status:    p.Status,
		Healthy:   p.Healthy,
		Warning:   p.Warning,
		Size:      pointer.From(p.Size),
		Allocated: pointer.From(p.Allocated),
		Free:      pointer.From(p.Free),
		Freeing:   pointer.From(p.Freeing),
		Scan:      p.Scan.poll(),
	}

	if p.Fragmentation.Known {
		out.Fragmentation = new(ratio(p.Fragmentation.Value))
	}

	if t := p.Topology; t != nil {
		for _, part := range []struct {
			class string
			vdevs []vdev
		}{{"data", t.Data}, {"log", t.Log}, {"cache", t.Cache}, {"spare", t.Spare}, {"special", t.Special}, {"dedup", t.Dedup}} {
			for i := range part.vdevs {
				out.Vdevs = append(out.Vdevs, part.vdevs[i].poll(part.class))
			}
		}
	}

	return out
}

// poll is the scan in the poller's words, and nil for a pool that has never had one.
func (s *scan) poll() *poll.Scan {
	if s == nil || s.Function == nil || s.State == nil {
		return nil
	}

	out := &poll.Scan{
		Function:  *s.Function,
		State:     *s.State,
		Started:   s.Started.Time,
		Ended:     s.Ended.Time,
		Progress:  ratio(pointer.From(s.Percent)),
		ToExamine: pointer.From(s.ToExamine),
		Examined:  pointer.From(s.Examined),
		Issued:    pointer.From(s.Issued),
		Errors:    pointer.From(s.Errors),
		Paused:    !s.Paused.IsZero(),
	}

	if s.Left != nil {
		out.Left = new(time.Duration(*s.Left * float64(time.Second)))
	}

	return out
}

// poll is a top-level vdev in the poller's words, with every disk under it however deep: a disk being replaced sits a level further down than the
// rest, beside the one replacing it.
func (v *vdev) poll(class string) poll.Vdev {
	// zpool lists a vdev of one disk under a long number where the NAS gives one; the disk's own name says more
	name := v.Name
	if disk := pointer.From(v.Disk); len(v.Children) == 0 && disk != "" {
		name = disk
	}

	out := poll.Vdev{
		Class:         class,
		Name:          name,
		Type:          v.Type,
		Status:        v.Status,
		Size:          v.Stats.Size,
		Allocated:     v.Stats.Allocated,
		Fragmentation: ratio(v.Stats.Fragmentation),
		Errors:        poll.Errors{Read: v.Stats.Read, Write: v.Stats.Write, Checksum: v.Stats.Checksum},
	}
	v.disks(&out.Devices)

	return out
}

// disks adds the disks at and under this node. A node with nothing under it is a disk, whatever it is called.
func (v *vdev) disks(into *[]poll.Device) {
	if len(v.Children) > 0 {
		for i := range v.Children {
			v.Children[i].disks(into)
		}

		return
	}

	// a disk the NAS can no longer find has no name; the pool still knows it by its number, which tells two missing disks apart
	disk := pointer.From(v.Disk)
	if disk == "" {
		disk = v.GUID
	}

	*into = append(*into, poll.Device{Disk: disk, Status: v.Status, Errors: poll.Errors{Read: v.Stats.Read, Write: v.Stats.Write, Checksum: v.Stats.Checksum}})
}
