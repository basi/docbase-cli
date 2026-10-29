package broker

import (
	"fmt"
	"os"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/basi/docbase-cli/internal/credential"
)

// TestMain swaps in an in-memory keyring so that no test can reach the
// developer's real credential store. The in-memory keyring needs no D-Bus
// session bus, which Linux CI runners lack, so the session bus check is bypassed.
//
// 'broker start' runs its own executable again as 'broker serve'. In the tests that
// executable is the test binary, which then has to act as the docbase binary.
func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == "broker" && os.Args[2] == "serve" {
		if err := root.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	keyring.MockInit()
	credential.SetBusCheck(func() bool { return true })
	os.Exit(m.Run())
}
