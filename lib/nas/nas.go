// Package nas asks a TrueNAS the poller's questions over its API and turns what it says into the poller's own words.
//
// Each question is one or a few of the API's methods, all of them reads, asked through truenas-mcp's typed client. A query names the fields it
// reads and the NAS sends only those, so a field the page has no use for cannot fail it. The types are made from the NAS's own description of
// each answer: a field that is read and does not fit them fails that method, and is put right in truenas-mcp rather than here. Nothing here
// dials or logs in; the calls go through a Caller that does.
package nas

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/truenas-mcp/sdk/truenas/client"
	"github.com/katbyte/truenas-metrics/lib/poll"
)

// Caller makes one call of the TrueNAS API over a connection that is already logged in, and decodes the answer into into.
type Caller interface {
	Call(ctx context.Context, method string, params []any, into any) error
}

// Options says how to take what a NAS answers.
type Options struct {
	// NoSuchMethod says whether an error is the NAS having no method of the name called, which a NAS older or newer than this was written for
	// may not. Such a method is answered as if the NAS had nothing to say, so that one missing method does not cost the rest of the page. Left
	// unset, it is a failure like any other.
	NoSuchMethod func(error) bool
}

// NAS is one TrueNAS, asked through a Caller.
type NAS struct {
	c   Caller
	o   Options
	api *client.Client

	// missing is the methods the NAS has been found not to have, so each is logged once and not at every round.
	missing sync.Map

	// unknownZones is the time zones the NAS has named that this machine does not know, so each is logged once.
	unknownZones sync.Map
}

// NAS answers every question the poller has.
var _ poll.NAS = (*NAS)(nil)

// New is the NAS at the other end of c.
func New(c Caller, o Options) *NAS {
	n := &NAS{c: c, o: o}

	// the typed client turns down only a missing connection, and calls is always one
	n.api, _ = client.New(calls{n})

	return n
}

// calls is the connection as the typed client uses it: a failure says which method it was, since a question can be several, and a method the NAS
// does not have is answered with nothing when Options says how to tell.
type calls struct{ n *NAS }

func (c calls) Call(ctx context.Context, method string, params []any, into any) error {
	err := c.n.c.Call(ctx, method, params, into)
	if err == nil {
		return nil
	}

	if c.n.o.NoSuchMethod != nil && c.n.o.NoSuchMethod(err) {
		if _, told := c.n.missing.LoadOrStore(method, true); !told {
			clog.Log.Warnf("this NAS has no %s: what it would have said is left off the page", method)
		}

		return nil
	}

	return fmt.Errorf("%s: %w", method, err)
}

// zone is the time zone the NAS names, or UTC for one this machine does not know, which is said once.
func (n *NAS) zone(name string) *time.Location {
	zone, err := time.LoadLocation(name)
	if err == nil {
		return zone
	}

	if _, told := n.unknownZones.LoadOrStore(name, true); !told {
		clog.Log.Warnf("this machine does not know the NAS's time zone %q, so when its certificates run out is read as if it were UTC: %v", name, err)
	}

	return time.UTC
}
