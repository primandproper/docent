// Package secrets keeps an integration's token in the operating system's
// credential store: the Keychain on macOS, the Secret Service on Linux and the
// Credential Manager on Windows. A token never goes in a config file.
package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"

	"github.com/zalando/go-keyring"
)

const (
	o11yName = "secrets"

	// service is the name every Docent secret is filed under.
	service = "docent"

	// The places a secret can come from, as `docent doctor` names them.
	SourceEnvironment = "environment"
	SourceStore       = "credential store"
)

// ErrNotFound is returned for a secret that was never stored.
var ErrNotFound = errors.New("no such secret")

type (
	// Store is the credential store. A secret is named after the
	// integration it belongs to.
	Store interface {
		// Get returns a secret, or ErrNotFound.
		Get(ctx context.Context, name string) (string, error)
		// Set stores a secret, replacing any that was there.
		Set(ctx context.Context, name, secret string) error
		// Delete removes a secret. Removing one that isn't there is ErrNotFound.
		Delete(ctx context.Context, name string) error
	}

	keyringStore struct {
		_      struct{}
		logger logging.Logger
		tracer tracing.Tracer
	}
)

var _ Store = (*keyringStore)(nil)

// NewStore returns the operating system's credential store.
func NewStore(logger logging.Logger, tracerProvider tracing.Provider) Store {
	return &keyringStore{
		logger: logging.NewNamedLogger(logger, o11yName),
		tracer: tracing.NewNamedTracer(tracerProvider, o11yName),
	}
}

func (s *keyringStore) Get(ctx context.Context, name string) (string, error) {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	secret, err := keyring.Get(service, name)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", fmt.Errorf("%q: %w", name, ErrNotFound)
	}

	if err != nil {
		return "", fmt.Errorf("reading %q from the credential store: %w", name, err)
	}

	return secret, nil
}

func (s *keyringStore) Set(ctx context.Context, name, secret string) error {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	// On macOS this hands the secret to /usr/bin/security on its standard
	// input, so it is never an argument another process could read.
	if err := keyring.Set(service, name, secret); err != nil {
		return fmt.Errorf("storing %q in the credential store: %w", name, err)
	}

	s.logger.WithValue("name", name).Info("stored a secret")

	return nil
}

func (s *keyringStore) Delete(ctx context.Context, name string) error {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	err := keyring.Delete(service, name)
	if errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("%q: %w", name, ErrNotFound)
	}

	if err != nil {
		return fmt.Errorf("removing %q from the credential store: %w", name, err)
	}

	s.logger.WithValue("name", name).Info("removed a secret")

	return nil
}

// Resolve finds a secret: in the environment variable first, which is the
// override and the answer on a machine with no credential store, then in the
// store. source says which, and is empty when there is none. An error means
// the store couldn't be asked, not that the secret is missing.
func Resolve(ctx context.Context, store Store, getenv func(string) string, envVar, name string) (secret, source string, err error) {
	if getenv != nil {
		if secret = getenv(envVar); secret != "" {
			return secret, SourceEnvironment, nil
		}
	}

	if store == nil {
		return "", "", nil
	}

	secret, err = store.Get(ctx, name)

	switch {
	case errors.Is(err, ErrNotFound):
		return "", "", nil
	case err != nil:
		return "", "", err
	case secret == "":
		return "", "", nil
	}

	return secret, SourceStore, nil
}
