# Development

```bash
make            # format and build
make test       # the tests, under the race detector
make test-live  # the live tests, against a TrueNAS you name
make testacc    # the live tests, against a throwaway TrueNAS in a virtual machine
make check-all  # build, tests, every linter and the dependency check
```

Dependencies are vendored. The dev tools are pinned in `.tools/go.mod` and the makefile, and `make tools` installs them into `.tools/bin`.

## How it is put together

- [`lib/poll`](../lib/poll) asks the questions on two schedules and keeps the last good answer to each.
- [`lib/nas`](../lib/nas) is the only part that knows the TrueNAS API: which methods answer each question, and how to read what they say.
- [`lib/readings`](../lib/readings) turns what the poller knows into the page.
- The connection to the NAS is [truenas-mcp](https://github.com/katbyte/truenas-mcp)'s.

## The tests

`make test` needs no TrueNAS.

- **The page.** It is built from a NAS the test makes up and kept whole in [`lib/readings/testdata/page.txt`](../lib/readings/testdata/page.txt). A renamed reading shows up as a difference there. `go test ./lib/readings -update` rewrites the file once the change is meant.
- **The NAS's answers.** What a real NAS answers is kept in [`lib/nas/testdata`](../lib/nas/testdata), one file per method, with every name and serial number made up.
- **Read only.** The list of methods the program may call is in `lib/nas`'s tests. Calling one that is not on the list fails them.
- **The whole program.** One test runs it against a stand-in that speaks the API's protocol and answers from those files.

## The live tests

`make test-live` runs the program against a real TrueNAS. It holds the page to what that NAS says when asked directly: every pool and its status, every disk by serial number, every dataset and service, and as many alerts, tasks and shares as it lists.

It only reads and knows nothing of the NAS beforehand, so any TrueNAS will do. Name the NAS first, logging in as the read-only user the program is meant to run as:

```bash
export TRUENAS_METRICS_TEST_SERVER=192.168.1.50
export TRUENAS_METRICS_TEST_USERNAME=metrics
export TRUENAS_METRICS_TEST_API_KEY=3-xxxxxxxx
export TRUENAS_METRICS_TEST_CERTIFICATE_FILE=./nas.pem
make test-live
```

A `.testenv.sh` holding those lines is ignored by git, as is any `.pem`.

## A throwaway TrueNAS

`make testacc` needs no NAS of yours. It:

1. fetches [truenas-mcp](https://github.com/katbyte/truenas-mcp)'s test environment into `.testnas`, at the version `go.mod` names
2. has it install TrueNAS in a virtual machine, with a pool and a read-only user
3. runs the live tests against it as that user
4. removes the machine

It needs qemu and an x86-64 Linux machine with `/dev/kvm`. TrueNAS has no build for a Mac's processor. The first run installs TrueNAS, which takes about ten minutes. Later runs start from the installed disk.

The `acceptance` workflow runs this on every push.
