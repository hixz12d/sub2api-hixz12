//go:build linux

package repository

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Linux's SSL_CERT_FILE is process-local. A fresh process prevents x509's
// cached system roots from making certificate trust depend on test order.
func TestHTTP2OutcomeRealUTLSProxyFallback(t *testing.T) {
	const childMode = "SUB2API_TEST_UTLS_PROXY"
	if scheme := os.Getenv(childMode); scheme != "" {
		require.Contains(t, []string{"http", "socks5", "socks5h"}, scheme)
		testHTTP2OutcomeRealDo(t, scheme, true)
		return
	}
	for _, scheme := range []string{"http", "socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHTTP2OutcomeRealUTLSProxyFallback$", "-test.timeout=45s", "-test.v")
			cmd.Env = append(os.Environ(), childMode+"="+scheme)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
		})
	}
}
