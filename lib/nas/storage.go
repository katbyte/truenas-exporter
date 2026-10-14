package nas

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katbyte/go-kt/pointer"
	"github.com/katbyte/truenas-mcp/sdk/truenas"
	"github.com/katbyte/truenas-metrics/lib/poll"
)

// diskFields is what is read of a disk.
var diskFields = []any{"name", "serial", "lunid", "model", "type", "bus", "size", "enclosure"}

// Disks asks the NAS for every disk it knows of, and for the temperature of each.
func (n *NAS) Disks(ctx context.Context) ([]poll.Disk, error) {
	disks, err := n.api.Disk.Query(ctx, nil, truenas.DiskQueryOptions{Select: diskFields})
	if err != nil {
		return nil, err
	}

	answer, err := n.api.Disk.Temperatures(ctx, nil, false)
	if err != nil {
		return nil, err
	}

	// the NAS's description gives this answer no shape, so it is read here: a disk that gives no temperature has nothing against its name
	var temperatures map[string]*float64
	if len(answer.Answer) > 0 {
		if err := json.Unmarshal(answer.Answer, &temperatures); err != nil {
			return nil, fmt.Errorf("disk.temperatures: %w", err)
		}
	}

	out := make([]poll.Disk, 0, len(disks.Answer))
	for _, d := range disks.Answer {
		disk := poll.Disk{Name: d.Name, Serial: d.Serial, WWN: d.Lunid, Model: d.Model, Type: d.Type, Bus: d.Bus, Size: int64(d.Size), Temperature: temperatures[d.Name]}
		if d.Enclosure != nil {
			disk.Slot = &poll.Slot{Enclosure: d.Enclosure.Number, Slot: d.Enclosure.Slot}
		}

		out = append(out, disk)
	}

	return out, nil
}

// datasetProperties is the properties of a dataset the NAS is asked for: named, so that it does not read out the dozens there are for each.
var datasetProperties = []string{"used", "available", "usedbydataset", "usedbysnapshots", "quota", "refquota", "compressratio"}

// datasetFields is what is read of a dataset: of each property, only the value the NAS has made a number of.
var datasetFields = []any{"name", "pool", "type", "used.parsed", "available.parsed", "usedbydataset.parsed", "usedbysnapshots.parsed", "quota.parsed", "refquota.parsed", "compressratio.parsed"}

// everyDataset is a filter every dataset passes. Asked for datasets without the ones under each, the NAS goes below the pools only when it is
// given a filter.
var everyDataset = []any{[]any{"type", "in", []string{"FILESYSTEM", "VOLUME"}}}

// Datasets asks the NAS for every dataset, however deep, as one flat list. Each is asked for without the ones under it: with them, every row holds
// a copy of the whole tree below, and the answer grows with how deep the tree goes.
func (n *NAS) Datasets(ctx context.Context) ([]poll.Dataset, error) {
	datasets, err := n.api.PoolDataset.Query(ctx, everyDataset, truenas.PoolDatasetQueryOptions{Select: datasetFields, Extra: map[string]any{"retrieve_children": false, "properties": datasetProperties}})
	if err != nil {
		return nil, err
	}

	out := make([]poll.Dataset, 0, len(datasets.Answer))
	for _, d := range datasets.Answer {
		compression, _ := number(pointer.From(d.Compressratio).Parsed)

		out = append(out, poll.Dataset{
			Name:            d.Name,
			Pool:            d.Pool,
			Type:            d.Type,
			Used:            size(pointer.From(d.Used).Parsed),
			Available:       size(pointer.From(d.Available).Parsed),
			UsedByDataset:   size(pointer.From(d.Usedbydataset).Parsed),
			UsedBySnapshots: size(pointer.From(d.Usedbysnapshots).Parsed),
			Quota:           limit(pointer.From(d.Quota).Parsed),
			RefQuota:        limit(pointer.From(d.Refquota).Parsed),
			Compression:     compression,
		})
	}

	return out, nil
}

// Snapshots asks the NAS how many snapshots every dataset has, asked for as datasets are but with none of their properties. A dataset the NAS
// gives no count for is left out.
func (n *NAS) Snapshots(ctx context.Context) ([]poll.SnapshotCount, error) {
	datasets, err := n.api.PoolDataset.Query(ctx, everyDataset, truenas.PoolDatasetQueryOptions{Select: []any{"name", "pool", "snapshot_count"}, Extra: map[string]any{"retrieve_children": false, "properties": []string{}, "snapshots_count": true}})
	if err != nil {
		return nil, err
	}

	out := make([]poll.SnapshotCount, 0, len(datasets.Answer))
	for _, d := range datasets.Answer {
		if d.SnapshotCount != nil {
			out = append(out, poll.SnapshotCount{Dataset: d.Name, Pool: d.Pool, Count: *d.SnapshotCount})
		}
	}

	return out, nil
}

// shareFields is what is read of an SMB or an NFS share.
var shareFields = []any{"enabled", "locked"}

// Shares asks the NAS for its SMB and NFS shares and its iSCSI targets. Nothing of a share is kept but how it is offered and whether it is on.
func (n *NAS) Shares(ctx context.Context) ([]poll.Share, error) {
	smb, err := n.api.SharingSmb.Query(ctx, nil, truenas.SharingSmbQueryOptions{Select: shareFields})
	if err != nil {
		return nil, err
	}

	nfs, err := n.api.SharingNfs.Query(ctx, nil, truenas.SharingNfsQueryOptions{Select: shareFields})
	if err != nil {
		return nil, err
	}

	// a target is only counted, so nothing of it is read but its number
	iscsi, err := n.api.IscsiTarget.Query(ctx, nil, truenas.IscsiTargetQueryOptions{Select: []any{"id"}})
	if err != nil {
		return nil, err
	}

	var out []poll.Share
	for _, s := range smb.Answer {
		out = append(out, poll.Share{Protocol: "smb", Enabled: s.Enabled, Locked: pointer.From(s.Locked)})
	}
	for _, s := range nfs.Answer {
		out = append(out, poll.Share{Protocol: "nfs", Enabled: s.Enabled, Locked: pointer.From(s.Locked)})
	}

	// an iSCSI target has no switch of its own: it is on while the service is
	for range iscsi.Answer {
		out = append(out, poll.Share{Protocol: "iscsi", Enabled: true})
	}

	return out, nil
}

// Certificates asks the NAS for the certificates it holds and when each runs out. The NAS writes those times in its own time zone without naming
// it, so it is asked for its zone too.
func (n *NAS) Certificates(ctx context.Context) ([]poll.Certificate, error) {
	certificates, err := n.api.Certificate.Query(ctx, nil, truenas.CertificateQueryOptions{Select: []any{"name", "until"}})
	if err != nil {
		return nil, err
	}

	info, err := n.api.System.Info(ctx)
	if err != nil {
		return nil, err
	}
	zone := n.zone(info.Timezone)

	out := make([]poll.Certificate, 0, len(certificates.Answer))
	for i := range certificates.Answer {
		c := &certificates.Answer[i]
		out = append(out, poll.Certificate{Name: c.Name, Expires: expiry(c.Until, zone)})
	}

	return out, nil
}

// linkUp is what the NAS calls an interface with a link.
const linkUp = "LINK_STATE_UP"

// Interfaces asks the NAS for its network interfaces and whether each has a link.
func (n *NAS) Interfaces(ctx context.Context) ([]poll.Interface, error) {
	interfaces, err := n.api.Interface.Query(ctx, nil, truenas.InterfaceQueryOptions{Select: []any{"name", "type", "state.link_state", "state.mtu"}})
	if err != nil {
		return nil, err
	}

	out := make([]poll.Interface, 0, len(interfaces.Answer))
	for _, i := range interfaces.Answer {
		state := pointer.From(i.State)
		out = append(out, poll.Interface{Name: i.Name, Type: i.Type, LinkUp: state.LinkState == linkUp, MTU: state.Mtu})
	}

	return out, nil
}
