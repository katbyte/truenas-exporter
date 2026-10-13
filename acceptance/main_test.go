//go:build acceptance

// Package acceptance_test runs truenas-metrics against a real TrueNAS and checks that the page it writes says what that NAS says of itself: the
// pools it has and their status, every disk by serial number, how many datasets, which services are running. The unit tests answer in a NAS's
// place with what the code expects of one; these are where a real one gets to answer otherwise.
//
// The suite only ever reads, as the program does, and knows nothing of what the NAS holds beforehand: whatever is on the page is compared with
// what the NAS answers when asked directly. So it can be pointed at any TrueNAS, a throwaway one in a virtual machine for choice, where a pool can
// be broken on purpose to see it reported. The user it logs in as should be the one the program is meant to run as, a read-only administrator:
// a question that user may not ask fails the suite, which is the point.
//
//	export TRUENAS_METRICS_TEST_SERVER=192.168.1.50
//	export TRUENAS_METRICS_TEST_USERNAME=metrics
//	export TRUENAS_METRICS_TEST_API_KEY=...                  # or keep the four in a .testenv.sh, which git ignores, and source it
//	export TRUENAS_METRICS_TEST_CERTIFICATE_FILE=./nas.pem   # the NAS's own certificate; left out, it has to be one the system trusts
//	make test-live
//
// With no NAS named every test skips.
package acceptance_test

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) { os.Exit(m.Run()) }
