package nas

import (
	"context"

	"github.com/katbyte/go-kt/pointer"
	"github.com/katbyte/truenas-metrics/lib/poll"
)

// job is the job a task's last run was, for the kinds of task the NAS runs as jobs: cloud sync, cloud backup and rsync.
type job struct {
	State    string `json:"state"`
	Progress struct {
		Percent *float64 `json:"percent"`
	} `json:"progress"`
	Started  moment `json:"time_started"`
	Finished moment `json:"time_finished"`
}

// into says of a task how the job went. A job that was stopped by hand is over and is neither a success nor a failure; one still waiting to
// start says nothing of the task yet.
func (j *job) into(t *poll.Task) {
	if j == nil {
		return
	}

	switch j.State {
	case running:
		t.Running = true
		t.Progress = ratio(pointer.From(j.Progress.Percent))
	case "SUCCESS", "FAILED", "ABORTED":
		t.LastRun = &poll.Run{OK: j.State == "SUCCESS", Failed: j.State == "FAILED", Ended: j.Finished.Time}
		if !j.Started.IsZero() && j.Finished.After(j.Started.Time) {
			t.LastRun.Took = j.Finished.Sub(j.Started.Time)
		}
	}
}

// taskState is how a snapshot or a replication task last went, which the NAS keeps as a state and the moment it was reached.
type taskState struct {
	State string `json:"state"`
	At    moment `json:"datetime"`
}

// into says of a task how it last went. A task on hold cannot run until somebody puts right what is wrong with it, a locked dataset say, so it is
// one that failed; one that has never run, or is waiting its turn, has no last run.
func (s taskState) into(t *poll.Task) {
	switch s.State {
	case running:
		t.Running = true
	case "FINISHED":
		t.LastRun = &poll.Run{OK: true, Ended: s.At.Time}
	case "ERROR", "HOLD":
		t.LastRun = &poll.Run{Failed: true, Ended: s.At.Time}
	}
}

// Tasks asks the NAS for its scheduled tasks of every kind and how each last went.
func (n *NAS) Tasks(ctx context.Context) ([]poll.Task, error) {
	var out []poll.Task

	snapshots, err := ask[[]struct {
		ID      int       `json:"id"`
		Dataset string    `json:"dataset"`
		Enabled bool      `json:"enabled"`
		State   taskState `json:"state"`
	}](ctx, n, "pool.snapshottask.query")
	if err != nil {
		return nil, err
	}

	for _, s := range snapshots {
		task := poll.Task{Kind: poll.SnapshotTask, ID: s.ID, Name: s.Dataset, Enabled: s.Enabled}
		s.State.into(&task)
		out = append(out, task)
	}

	replications, err := ask[[]struct {
		ID      int       `json:"id"`
		Name    string    `json:"name"`
		Enabled bool      `json:"enabled"`
		State   taskState `json:"state"`
	}](ctx, n, "replication.query")
	if err != nil {
		return nil, err
	}

	for _, r := range replications {
		task := poll.Task{Kind: poll.ReplicationTask, ID: r.ID, Name: r.Name, Enabled: r.Enabled}
		r.State.into(&task)
		out = append(out, task)
	}

	// the three kinds the NAS runs as jobs differ only in what they call the task's name
	type jobTask struct {
		ID          int    `json:"id"`
		Description string `json:"description"`
		Desc        string `json:"desc"`
		Enabled     bool   `json:"enabled"`
		Job         *job   `json:"job"`
	}

	for _, kind := range []struct {
		kind   poll.TaskKind
		method string
	}{{poll.CloudSyncTask, "cloudsync.query"}, {poll.CloudBackupTask, "cloud_backup.query"}, {poll.RsyncTask, "rsynctask.query"}} {
		var tasks []jobTask
		if tasks, err = ask[[]jobTask](ctx, n, kind.method); err != nil {
			return nil, err
		}

		for _, t := range tasks {
			task := poll.Task{Kind: kind.kind, ID: t.ID, Name: t.Description + t.Desc, Enabled: t.Enabled}
			t.Job.into(&task)
			out = append(out, task)
		}
	}

	scrubs, err := ask[[]struct {
		ID      int    `json:"id"`
		Pool    string `json:"pool_name"`
		Enabled bool   `json:"enabled"`
	}](ctx, n, "pool.scrub.query")
	if err != nil {
		return nil, err
	}

	for _, s := range scrubs {
		out = append(out, poll.Task{Kind: poll.ScrubTask, ID: s.ID, Name: s.Pool, Enabled: s.Enabled})
	}

	return out, nil
}
