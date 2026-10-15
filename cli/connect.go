package cli

import (
	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/go-kt/version"
	"github.com/katbyte/truenas-exporter/lib/nas"
	"github.com/katbyte/truenas-exporter/lib/poll"
	"github.com/katbyte/truenas-mcp/sdk/truenasrpc"
)

// connect makes the connection the NAS is asked over: truenas-mcp's, which dials on the first question, logs in with the key, and dials again
// whenever the NAS has gone away and come back.
func connect(c connection) (poll.NAS, func(), error) {
	// the NAS writes who called it in its audit log
	opts := []truenasrpc.Option{truenasrpc.WithLog(clog.Log), truenasrpc.WithUserAgent(toolName + "/" + version.Version)}

	if c.certFile != "" {
		opts = append(opts, truenasrpc.WithCertificateFile(c.certFile))
	}

	if c.caFile != "" {
		opts = append(opts, truenasrpc.WithCAFile(c.caFile))
	}

	if c.skipCheck {
		opts = append(opts, truenasrpc.WithoutCertificateCheck())
	}

	var login []truenasrpc.LoginOption
	if c.sendUnchecked {
		login = append(login, truenasrpc.EvenUnchecked())
	}

	conn, err := truenasrpc.New(c.server, truenasrpc.APIKey(c.username, c.key, login...), opts...)
	if err != nil {
		return nil, nil, err
	}

	return nas.New(conn, nas.Options{NoSuchMethod: truenasrpc.IsNoSuchMethod, ClockCorrection: c.clockCorrection}), func() { _ = conn.Close() }, nil
}
