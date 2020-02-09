package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"

	"github.com/mutagen-io/mutagen/cmd"
	"github.com/mutagen-io/mutagen/pkg/daemon"
	"github.com/mutagen-io/mutagen/pkg/grpcutil"
	"github.com/mutagen-io/mutagen/pkg/ipc"
	"github.com/mutagen-io/mutagen/pkg/mutagen"
	daemonsvc "github.com/mutagen-io/mutagen/pkg/service/daemon"
)

const (
	// dialTimeout is the timeout to use when attempting to connect to the
	// daemon IPC endpoint.
	dialTimeout = 500 * time.Millisecond
	// autostartWaitInterval is the wait period between reconnect attempts after
	// autostarting the daemon.
	autostartWaitInterval = 100 * time.Millisecond
	// autostartRetryCount is the number of times to try reconnecting after
	// autostarting the daemon.
	autostartRetryCount = 10
)

// Connect establishes a daemon connection with the specified autostart and
// version matching behavior. Note that the autostart parameter can still be
// overridden by the MUTAGEN_DISABLE_AUTOSTART environment variable.
func Connect(autostart, requireVersionMatch bool) (*grpc.ClientConn, error) {
	// Compute the path to the daemon IPC endpoint.
	endpoint, err := daemon.EndpointPath()
	if err != nil {
		return nil, fmt.Errorf("unable to compute endpoint path: %w", err)
	}

	// Check if autostart has been globally disabled.
	if daemon.AutostartDisabled {
		autostart = false
	}

	// Create a status line printer and defer a clear.
	statusLinePrinter := &cmd.StatusLinePrinter{UseStandardError: true}
	defer statusLinePrinter.BreakIfNonEmpty()

	// Perform dialing in a loop until failure or success.
	remainingPostAutostatAttempts := autostartRetryCount
	invokedStart := false
	var connection *grpc.ClientConn
	for {
		// Create a context to timeout the dial.
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)

		// Attempt to dial.
		connection, err = grpc.DialContext(
			ctx, endpoint,
			grpc.WithInsecure(),
			grpc.WithContextDialer(ipc.DialContext),
			grpc.WithBlock(),
			grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(grpcutil.MaximumMessageSize)),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(grpcutil.MaximumMessageSize)),
		)

		// Cancel the dialing context. If the dialing operation has already
		// succeeded, this has no effect, but it is necessary avoid leaking
		// context resources.
		cancel()

		// Check for errors.
		if err != nil {
			// Handle failure due to timeouts.
			if err == context.DeadlineExceeded {
				// If autostart is enabled, and we have attempts remaining, then
				// try autostarting, waiting, and retrying.
				if autostart && remainingPostAutostatAttempts > 0 {
					if !invokedStart {
						statusLinePrinter.Print("Attempting to start Mutagen daemon...")
						startMain(nil, nil)
						invokedStart = true
					}
					time.Sleep(autostartWaitInterval)
					remainingPostAutostatAttempts--
					continue
				}

				// Otherwise just fail due to the timeout.
				return nil, errors.New("connection timed out (is the daemon running?)")
			}

			// If we failed for any other reason, then bail.
			return nil, err
		}

		// Print a notice if we started the daemon.
		if invokedStart {
			statusLinePrinter.Clear()
			statusLinePrinter.Print("Started Mutagen daemon in background (terminate with \"mutagen daemon stop\")")
		}

		// We've successfully dialed, so break out of the dialing loop.
		break
	}

	// If requested, verify that the daemon version matches the current process'
	// version.
	if requireVersionMatch {
		daemonService := daemonsvc.NewDaemonClient(connection)
		version, err := daemonService.Version(context.Background(), &daemonsvc.VersionRequest{})
		if err != nil {
			connection.Close()
			return nil, fmt.Errorf("unable to query daemon version: %w", err)
		}
		versionMatch := version.Major == mutagen.VersionMajor &&
			version.Minor == mutagen.VersionMinor &&
			version.Patch == mutagen.VersionPatch &&
			version.Tag == mutagen.VersionTag
		if !versionMatch {
			connection.Close()
			return nil, errors.New("client/daemon version mismatch (daemon restart recommended)")
		}
	}

	// Success.
	return connection, nil
}
