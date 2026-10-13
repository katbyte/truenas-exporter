package nas

import (
	"context"

	"github.com/katbyte/go-kt/pointer"
	"github.com/katbyte/truenas-metrics/lib/poll"
)

// Disks asks the NAS for every disk it knows of, and for the temperature of each.
func (n *NAS) Disks(ctx context.Context) ([]poll.Disk, error) {
	disks, err := ask[[]struct {
		Name      string  `json:"name"`
		Serial    string  `json:"serial"`
		WWN       *string `json:"lunid"`
		Model     *string `json:"model"`
		Type      *string `json:"type"`
		Bus       string  `json:"bus"`
		Size      *int64  `json:"size"`
		Enclosure *struct {
			Number int `json:"number"`
			Slot   int `json:"slot"`
		} `json:"enclosure"`
	}](ctx, n, "disk.query")
	if err != nil {
		return nil, err
	}

	// a disk that gives no temperature is in the answer with nothing against its name
	temperatures, err := ask[map[string]*float64](ctx, n, "disk.temperatures")
	if err != nil {
		return nil, err
	}

	out := make([]poll.Disk, 0, len(disks))
	for _, d := range disks {
		disk := poll.Disk{
			Name:        d.Name,
			Serial:      d.Serial,
			WWN:         pointer.From(d.WWN),
			Model:       pointer.From(d.Model),
			Type:        pointer.From(d.Type),
			Bus:         d.Bus,
			Size:        pointer.From(d.Size),
			Temperature: temperatures[d.Name],
		}

		if d.Enclosure != nil {
			disk.Slot = &poll.Slot{Enclosure: d.Enclosure.Number, Slot: d.Enclosure.Slot}
		}

		out = append(out, disk)
	}

	return out, nil
}

// datasetProperties is the properties of a dataset the NAS is asked for: named, so that it does not read out the dozens there are for each.
var datasetProperties = []string{"used", "available", "usedbydataset", "usedbysnapshots", "quota", "refquota", "compressratio"}

// Datasets asks the NAS for every dataset, however deep, as one flat list with how many snapshots each has.
func (n *NAS) Datasets(ctx context.Context) ([]poll.Dataset, error) {
	options := map[string]any{"extra": map[string]any{"flat": true, "retrieve_children": true, "properties": datasetProperties, "snapshots_count": true}}

	datasets, err := ask[[]struct {
		Name            string   `json:"name"`
		Pool            string   `json:"pool"`
		Type            string   `json:"type"`
		Used            property `json:"used"`
		Available       property `json:"available"`
		UsedByDataset   property `json:"usedbydataset"`
		UsedBySnapshots property `json:"usedbysnapshots"`
		Quota           property `json:"quota"`
		RefQuota        property `json:"refquota"`
		Compression     property `json:"compressratio"`
		Snapshots       *int     `json:"snapshot_count"`
	}](ctx, n, "pool.dataset.query", []any{}, options)
	if err != nil {
		return nil, err
	}

	out := make([]poll.Dataset, 0, len(datasets))
	for _, d := range datasets {
		out = append(out, poll.Dataset{
			Name:            d.Name,
			Pool:            d.Pool,
			Type:            d.Type,
			Used:            d.Used.bytes(),
			Available:       d.Available.bytes(),
			UsedByDataset:   d.UsedByDataset.bytes(),
			UsedBySnapshots: d.UsedBySnapshots.bytes(),
			Quota:           d.Quota.limit(),
			RefQuota:        d.RefQuota.limit(),
			Compression:     d.Compression.Parsed.Value,
			Snapshots:       d.Snapshots,
		})
	}

	return out, nil
}

// Shares asks the NAS for its SMB and NFS shares and its iSCSI targets. Nothing of a share is kept but how it is offered and whether it is on.
func (n *NAS) Shares(ctx context.Context) ([]poll.Share, error) {
	type share struct {
		Enabled *bool `json:"enabled"`
		Locked  *bool `json:"locked"`
	}

	var out []poll.Share
	for _, kind := range []struct{ protocol, method string }{{"smb", "sharing.smb.query"}, {"nfs", "sharing.nfs.query"}, {"iscsi", "iscsi.target.query"}} {
		shares, err := ask[[]share](ctx, n, kind.method)
		if err != nil {
			return nil, err
		}

		for _, s := range shares {
			// an iSCSI target has no switch of its own: it is on while the service is
			out = append(out, poll.Share{Protocol: kind.protocol, Enabled: s.Enabled == nil || *s.Enabled, Locked: pointer.From(s.Locked)})
		}
	}

	return out, nil
}

// Certificates asks the NAS for the certificates it holds and when each runs out.
func (n *NAS) Certificates(ctx context.Context) ([]poll.Certificate, error) {
	certificates, err := ask[[]struct {
		Name  string `json:"name"`
		Until moment `json:"until"`
	}](ctx, n, "certificate.query")
	if err != nil {
		return nil, err
	}

	out := make([]poll.Certificate, 0, len(certificates))
	for _, c := range certificates {
		out = append(out, poll.Certificate{Name: c.Name, Expires: c.Until.Time})
	}

	return out, nil
}

// linkUp is what the NAS calls an interface with a link.
const linkUp = "LINK_STATE_UP"

// Interfaces asks the NAS for its network interfaces and whether each has a link.
func (n *NAS) Interfaces(ctx context.Context) ([]poll.Interface, error) {
	interfaces, err := ask[[]struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		State struct {
			Link string `json:"link_state"`
			MTU  int    `json:"mtu"`
		} `json:"state"`
	}](ctx, n, "interface.query")
	if err != nil {
		return nil, err
	}

	out := make([]poll.Interface, 0, len(interfaces))
	for _, i := range interfaces {
		out = append(out, poll.Interface{Name: i.Name, Type: i.Type, LinkUp: i.State.Link == linkUp, MTU: i.State.MTU})
	}

	return out, nil
}
