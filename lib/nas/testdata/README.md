# testdata

One file for each method of the TrueNAS API the exporter calls, named for the method and holding what a NAS answers.

Most were taken from a real TrueNAS 25.10.7 and then scrubbed: the shapes, states and sizes are as it gave them, as are the makes of its processor and disks, and every name, serial number, path and piece of text is made up. Lists were cut down to a few rows, and the parts of an answer that hold other people's words (an alert's text, a task's log, a share's settings) were left out.

These were written from the API's own description of what it answers, because the NAS they came from had none of the thing: `app.query`, `vm.query`, `replication.query`, `cloud_backup.query`, `sharing.nfs.query`, `iscsi.target.query` and `api_key.my_keys`. Nobody has yet seen a real answer to them here, so they are the least certain.

Never put a real answer in this folder as it came. A NAS's answers name its disks by serial number and its datasets and shares by what their owner called them.
