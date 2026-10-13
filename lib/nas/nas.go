// Package nas asks a TrueNAS the poller's questions over its API and turns what it says into the poller's own words.
//
// Each question is one or a few of the API's methods, all of them reads, and each answer is decoded into only the fields the page reports: what a
// newer TrueNAS adds is ignored, and what an older one lacks is left empty. Nothing here dials or logs in; the calls go through a Caller that does.
package nas

import (
	"context"
	"fmt"
	"sync"

	"github.com/katbyte/go-kt/clog"
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
	c Caller
	o Options

	// missing is the methods the NAS has been found not to have, so each is logged once and not at every round.
	missing sync.Map
}

// NAS answers every question the poller has.
var _ poll.NAS = (*NAS)(nil)

// New is the NAS at the other end of c.
func New(c Caller, o Options) *NAS {
	return &NAS{c: c, o: o}
}

// ask makes one call and gives its answer as a T. What went wrong is said with the method's name in front, since a question can be several methods.
func ask[T any](ctx context.Context, n *NAS, method string, params ...any) (T, error) {
	var answer T

	err := n.c.Call(ctx, method, params, &answer)
	if err == nil {
		return answer, nil
	}

	if n.o.NoSuchMethod != nil && n.o.NoSuchMethod(err) {
		if _, told := n.missing.LoadOrStore(method, true); !told {
			clog.Log.Warnf("this NAS has no %s: what it would have said is left off the page", method)
		}

		var nothing T

		return nothing, nil
	}

	return answer, fmt.Errorf("%s: %w", method, err)
}
