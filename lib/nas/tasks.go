package nas

import (
	"context"

	"github.com/katbyte/truenas-mcp/sdk/truenas"
	"github.com/katbyte/truenas-metrics/lib/poll"
)

// jobTask is a task of a kind the NAS runs as a job, cloud sync, cloud backup or rsync, said by how its last job went. A job that was stopped by hand
// is over and is neither a success nor a failure; one still waiting to start says nothing of the task yet.
func jobTask(kind poll.TaskKind, id int, name string, enabled bool, j *truenas.CoreGetJobsItemQueryResultItem) poll.Task {
	t := poll.Task{Kind: kind, ID: id, Name: name, Enabled: enabled}
	if j == nil {
		return t
	}

	switch j.State {
	case running:
		t.Running = true
		if j.Progress != nil {
			t.Progress = ratio(float64(j.Progress.Percent))
		}
	case "SUCCESS", "FAILED", "ABORTED":
		t.LastRun = &poll.Run{OK: j.State == "SUCCESS", Failed: j.State == "FAILED", Ended: j.TimeFinished.Time}
		if !j.TimeStarted.IsZero() && j.TimeFinished.After(j.TimeStarted.Time) {
			t.LastRun.Took = j.TimeFinished.Sub(j.TimeStarted.Time)
		}
	}

	return t
}

// stateTask is a snapshot or a replication task, said by the state the NAS keeps of how it last went. A task on hold cannot run until somebody puts
// right what is wrong with it, a locked dataset say, so it is one that failed; one that has never run, or is waiting its turn, has no last run.
func stateTask(kind poll.TaskKind, id int, name string, enabled bool, s *truenas.TaskState) poll.Task {
	t := poll.Task{Kind: kind, ID: id, Name: name, Enabled: enabled}
	if s == nil {
		return t
	}

	switch s.State {
	case running:
		t.Running = true
	case "FINISHED":
		t.LastRun = &poll.Run{OK: true, Ended: s.Datetime.Time}
	case "ERROR", "HOLD":
		t.LastRun = &poll.Run{Failed: true, Ended: s.Datetime.Time}
	}

	return t
}

// Tasks asks the NAS for its scheduled tasks of every kind and how each last went.
func (n *NAS) Tasks(ctx context.Context) ([]poll.Task, error) {
	snapshots, err := n.api.PoolSnapshottask.Query(ctx, nil, truenas.PoolSnapshottaskQueryOptions{})
	if err != nil {
		return nil, err
	}

	replications, err := n.api.Replication.Query(ctx, nil, truenas.ReplicationQueryOptions{})
	if err != nil {
		return nil, err
	}

	cloudSyncs, err := n.api.Cloudsync.Query(ctx, nil, truenas.CloudsyncQueryOptions{})
	if err != nil {
		return nil, err
	}

	cloudBackups, err := n.api.CloudBackup.Query(ctx, nil, truenas.CloudBackupQueryOptions{})
	if err != nil {
		return nil, err
	}

	rsyncs, err := n.api.Rsynctask.Query(ctx, nil, truenas.RsynctaskQueryOptions{})
	if err != nil {
		return nil, err
	}

	scrubs, err := n.api.PoolScrub.Query(ctx, nil, truenas.PoolScrubQueryOptions{})
	if err != nil {
		return nil, err
	}

	var out []poll.Task
	for _, s := range snapshots.Answer {
		out = append(out, stateTask(poll.SnapshotTask, s.Id, s.Dataset, s.Enabled, s.State))
	}
	for i := range replications.Answer {
		r := &replications.Answer[i]
		out = append(out, stateTask(poll.ReplicationTask, r.Id, r.Name, r.Enabled, r.State))
	}
	for _, c := range cloudSyncs.Answer {
		out = append(out, jobTask(poll.CloudSyncTask, c.Id, c.Description, c.Enabled, c.Job))
	}
	for _, c := range cloudBackups.Answer {
		out = append(out, jobTask(poll.CloudBackupTask, c.Id, c.Description, c.Enabled, c.Job))
	}
	for _, r := range rsyncs.Answer {
		out = append(out, jobTask(poll.RsyncTask, r.Id, r.Desc, r.Enabled, r.Job))
	}

	// a scrub's schedule has no run of its own: the pool's scan says how the last one went
	for _, s := range scrubs.Answer {
		out = append(out, poll.Task{Kind: poll.ScrubTask, ID: s.Id, Name: s.PoolName, Enabled: s.Enabled})
	}

	return out, nil
}
