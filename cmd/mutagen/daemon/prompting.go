package daemon

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	"github.com/mutagen-io/mutagen/cmd"
	"github.com/mutagen-io/mutagen/pkg/grpcutil"
	"github.com/mutagen-io/mutagen/pkg/prompt"
	promptingsvc "github.com/mutagen-io/mutagen/pkg/service/prompt"
)

// HostPrompting starts a background Goroutine that acts as a command line
// prompter. It accepts a context to regulate the lifetime of the prompting, a
// daemon client connection (which this method will not close), and an indicator
// of whether or not prompting should be supported (as opposed to only
// messaging). It returns the associated prompter identifier (which can be used
// with other RPC methods), a completion channel that will be closed once
// hosting has terminated and cleaned up the console, an error channel that will
// be populated with the first error to occur during hosting, and any error that
// occurred during hosting initialization (in which case all other parameters
// will be zero-valued).
func HostPrompting(ctx context.Context, connection *grpc.ClientConn, allowPrompts bool) (string, <-chan struct{}, <-chan error, error) {
	// Create a prompting service client.
	promptingClient := promptingsvc.NewPromptingClient(connection)

	// Create a cancellable subcontext that we can use to cancel prompt hosting
	// in case of failure.
	ctx, cancel := context.WithCancel(ctx)

	// Initiate prompt hosting.
	stream, err := promptingClient.Host(ctx)
	if err != nil {
		cancel()
		return "", nil, nil, fmt.Errorf("unable to initiate prompt hosting: %w", err)
	}

	// Send the initialization request.
	request := &promptingsvc.HostRequest{
		AllowPrompts: allowPrompts,
	}
	if err := stream.Send(request); err != nil {
		cancel()
		return "", nil, nil, fmt.Errorf("unable to send initialization request: %w", err)
	}

	// Receive the initialization response, validate it, and extract the
	// prompter identifier.
	var identifier string
	if response, err := stream.Recv(); err != nil {
		cancel()
		return "", nil, nil, fmt.Errorf("unable to receive initialization response: %w", err)
	} else if err = response.EnsureValid(true, allowPrompts); err != nil {
		cancel()
		return "", nil, nil, fmt.Errorf("invalid initialization response: %w", err)
	} else {
		identifier = response.Identifier
	}

	// Create a channel that we can use to monitor for completion. We have to
	// use a separate channel for this (as opposed to relying solely on the
	// error channel) because we want to be able to signal on this channel after
	// the status line printer has been cleaned up.
	done := make(chan struct{})

	// Create a channel that we can use to monitor for errors.
	hostingErrors := make(chan error, 1)

	// Start hosting in a background Goroutine.
	go func() {
		// Defer closure of the completion channel. Note that we want this to
		// occur after the status line printer is cleaned up.
		defer close(done)

		// Create a status line printer and defer a break.
		statusLinePrinter := &cmd.StatusLinePrinter{}
		defer statusLinePrinter.BreakIfNonEmpty()

		// Defer cancellation of the context to ensure context resource cleanup
		// and server-side cancellation in the event of a client-side error,
		// such as failed command line prompting.
		defer cancel()

		// Loop and handle requests indefinitely.
		for {
			if response, err := stream.Recv(); err != nil {
				hostingErrors <- fmt.Errorf("unable to receive message/prompt response: %w", grpcutil.PeelAwayRPCErrorLayer(err))
				return
			} else if err = response.EnsureValid(false, allowPrompts); err != nil {
				hostingErrors <- fmt.Errorf("invalid message/prompt response received: %w", err)
				return
			} else if response.IsPrompt {
				statusLinePrinter.BreakIfNonEmpty()
				if response, err := prompt.PromptCommandLine(response.Message); err != nil {
					hostingErrors <- fmt.Errorf("unable to perform prompting: %w", err)
					return
				} else if err = stream.Send(&promptingsvc.HostRequest{Response: response}); err != nil {
					hostingErrors <- fmt.Errorf("unable to send prompt response: %w", grpcutil.PeelAwayRPCErrorLayer(err))
					return
				}
			} else {
				statusLinePrinter.Print(response.Message)
				if err := stream.Send(&promptingsvc.HostRequest{}); err != nil {
					hostingErrors <- fmt.Errorf("unable to send message response: %w", grpcutil.PeelAwayRPCErrorLayer(err))
					return
				}
			}
		}
	}()

	// Success.
	return identifier, done, hostingErrors, nil
}
