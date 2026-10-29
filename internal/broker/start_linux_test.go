//go:build linux

package broker

import (
	"testing"
	"time"
)

func TestServeCommandStartsANewSession(t *testing.T) {
	cmd := serveCommand("/usr/local/bin/docbase", "example", "/tmp/x/broker.sock", time.Hour)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Errorf("SysProcAttr = %+v, want Setsid", cmd.SysProcAttr)
	}
}
